// Package credda is the official Go client for the Credda Reliability Score API.
//
// Two access models, matching the API (and the TypeScript SDK, @credda/js):
//
//   - Public: ResolveToken / GetTrustExport / GetDIDDocument / GetTrustRegistry
//     hit public endpoints and need no API key.
//   - Platform: everything else sends a platform API key as a Bearer token.
//     These are for SERVER-SIDE use only; never ship a `crd_live_…` key to an
//     untrusted client.
//
// The zero value of Client is not usable. Construct one with NewClient.
package credda

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production Credda API root.
const DefaultBaseURL = "https://api.credda.io"

// apiPrefix is prepended to every versioned API path. The /.well-known/*
// discovery documents live outside it (see getWellKnown).
const apiPrefix = "/api/v1"

// Client is a Credda API client. It is safe for concurrent use by multiple
// goroutines.
type Client struct {
	baseURL       string
	apiKey        string
	httpClient    *http.Client
	retries       int
	retryBase     time.Duration
	maxRetryDelay time.Duration
}

// Option configures a Client. Pass options to NewClient.
type Option func(*Client)

// WithAPIKey sets the platform API key sent as `Authorization: Bearer …` on
// every request that requires one.
func WithAPIKey(key string) Option {
	return func(c *Client) { c.apiKey = key }
}

// WithBaseURL overrides the API root (default DefaultBaseURL). Trailing
// slashes are trimmed.
func WithBaseURL(base string) Option {
	return func(c *Client) {
		if base != "" {
			c.baseURL = strings.TrimRight(base, "/")
		}
	}
}

// WithHTTPClient supplies a custom *http.Client (timeouts, transport, proxies).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithRetries enables opt-in automatic retries of TRANSIENT failures (network
// errors, 429, 502, 503, 504), matching @credda/js. n is the number of
// RE-attempts; 0 (the default) is off.
//
// Applied to GETs always, and to POSTs ONLY when the call carries an
// Idempotency-Key, so enabling this can never double-report an event. Backoff is
// 300ms doubling per attempt, or the server's own Retry-After when it sent one,
// capped at 5s either way. Tune with WithRetryBackoff.
func WithRetries(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.retries = n
		}
	}
}

// WithRetryBackoff overrides the first backoff wait and the ceiling on any
// single wait (defaults 300ms and 5s). The cap matters: a monthly-quota 429 can
// carry a Retry-After of days, and without it a retry would hang the call.
func WithRetryBackoff(base, max time.Duration) Option {
	return func(c *Client) {
		if base > 0 {
			c.retryBase = base
		}
		if max > 0 {
			c.maxRetryDelay = max
		}
	}
}

// NewClient builds a Client. With no options it targets DefaultBaseURL with a
// 30s-timeout HTTP client, no API key (public endpoints only) and no retries.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:       DefaultBaseURL,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		retryBase:     300 * time.Millisecond,
		maxRetryDelay: 5 * time.Second,
	}
	for _, o := range opts {
		if o != nil {
			o(c)
		}
	}
	return c
}

// APIError is returned for any non-2xx API response. It carries the HTTP
// status, the API's own error message (from the JSON `error` or `message`
// field) and the request path.
//
// It also carries everything needed to debug the failure later:
//
//   - RequestID: the X-Request-Id correlation id. Log it. Quoting it lets
//     Credda find the exact request in our logs; without it, support starts
//     from "describe what happened".
//   - Code: the stable machine code (see GET /api/v1/errors).
//   - Details: structured context, e.g. one entry per failed field on a
//     VALIDATION_ERROR.
//   - RetryAfter: how long the server asked you to wait (every 429 says so);
//     zero when it did not.
type APIError struct {
	StatusCode int
	Message    string
	Path       string
	// Code is the API's stable machine code, e.g. "QUOTA_EXCEEDED".
	Code string
	// RequestID is the X-Request-Id correlation id for this request.
	RequestID string
	// Details is the raw `details` field, left as JSON so a caller can decode
	// it into whatever shape the specific code documents.
	Details json.RawMessage
	// RetryAfter is the server's requested back-off. Zero when not sent.
	RetryAfter time.Duration
	// Retryable is the API's own verdict on whether repeating the identical
	// request can succeed later. Never retry when false.
	Retryable bool
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("credda: %s (status %d, path %s, requestId %s)", e.Message, e.StatusCode, e.Path, e.RequestID)
	}
	return fmt.Sprintf("credda: %s (status %d, path %s)", e.Message, e.StatusCode, e.Path)
}

// parseRetryAfter converts a Retry-After header into a duration. Accepts the
// delta-seconds form the API always sends and the HTTP-date form the spec also
// permits; returns 0 for absent/unparseable values and never returns negative.
func parseRetryAfter(raw string, now time.Time) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if at, err := http.ParseTime(raw); err == nil {
		if d := at.Sub(now); d > 0 {
			return d
		}
		return 0
	}
	return 0
}

// AsAPIError reports whether err is (or wraps) an *APIError, returning it.
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}

// ── transport ───────────────────────────────────────────────────────────────

type requestOptions struct {
	method      string
	path        string // relative to apiPrefix unless absolute is true
	absolute    bool   // path is relative to the API root (for /.well-known/*)
	body        any
	needsAPIKey bool
	headers     map[string]string
}

// safeToRepeat reports whether repeating this request can only ever be
// exactly-once: GETs always, POSTs only when idempotency-keyed. Every other
// write is left alone, so an opt-in retry cannot double-report.
func (ro requestOptions) safeToRepeat() bool {
	switch ro.method {
	case http.MethodGet:
		return true
	case http.MethodPost:
		_, keyed := ro.headers["Idempotency-Key"]
		return keyed
	}
	return false
}

// retryableStatus is the transient set shared with @credda/js: rate limit plus
// upstream and gateway blips.
func retryableStatus(code int) bool {
	return code == 429 || code == 502 || code == 503 || code == 504
}

func (c *Client) do(ctx context.Context, ro requestOptions, out any) error {
	var encoded []byte
	if ro.body != nil {
		b, err := json.Marshal(ro.body)
		if err != nil {
			return fmt.Errorf("credda: encoding request body: %w", err)
		}
		encoded = b
	}

	tries := 1
	if c.retries > 0 && ro.safeToRepeat() {
		tries = c.retries + 1
	}

	var lastErr error
	for i := 0; i < tries; i++ {
		if i > 0 {
			// The server's own Retry-After wins when it sent one: it knows when
			// the window resets, and waiting less just earns another 429.
			delay := c.retryBase << (i - 1)
			var apiErr *APIError
			if errors.As(lastErr, &apiErr) && apiErr.RetryAfter > 0 {
				delay = apiErr.RetryAfter
			}
			if delay > c.maxRetryDelay {
				delay = c.maxRetryDelay
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		err := c.attempt(ctx, ro, encoded, out)
		if err == nil {
			return nil
		}
		lastErr = err
		var apiErr *APIError
		if errors.As(err, &apiErr) && !retryableStatus(apiErr.StatusCode) {
			return err
		}
	}
	return lastErr
}

func (c *Client) attempt(ctx context.Context, ro requestOptions, encoded []byte, out any) error {
	if ro.needsAPIKey && c.apiKey == "" {
		return fmt.Errorf("credda: %s %s requires an API key (construct the client with WithAPIKey)", ro.method, ro.path)
	}

	prefix := apiPrefix
	if ro.absolute {
		prefix = ""
	}
	full := c.baseURL + prefix + ro.path

	var reader io.Reader
	if encoded != nil {
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, ro.method, full, reader)
	if err != nil {
		return fmt.Errorf("credda: building request: %w", err)
	}
	if encoded != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" && ro.needsAPIKey {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	for k, v := range ro.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("credda: %s %s: %w", ro.method, ro.path, err)
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := ""
		var body struct {
			Error     string          `json:"error"`
			Message   string          `json:"message"`
			Code      string          `json:"code"`
			RequestID string          `json:"requestId"`
			Retryable bool            `json:"retryable"`
			Details   json.RawMessage `json:"details"`
		}
		if readErr == nil && json.Unmarshal(raw, &body) == nil {
			msg = body.Error
			if msg == "" {
				msg = body.Message
			}
		}
		if msg == "" {
			msg = fmt.Sprintf("request failed (%d)", resp.StatusCode)
		}
		// The header is authoritative: it survives a non-JSON body; the body
		// echoes the same id.
		requestID := resp.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = body.RequestID
		}
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    msg,
			Path:       ro.path,
			Code:       body.Code,
			RequestID:  requestID,
			Details:    body.Details,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
			Retryable:  body.Retryable,
		}
	}

	if out == nil {
		return nil
	}
	if readErr != nil {
		return fmt.Errorf("credda: reading response body: %w", readErr)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		// 204 / empty body for a caller that expected JSON. Leave zero value.
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("credda: decoding response from %s: %w", ro.path, err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, needsKey bool, out any) error {
	return c.do(ctx, requestOptions{method: http.MethodGet, path: path, needsAPIKey: needsKey}, out)
}

func (c *Client) getWellKnown(ctx context.Context, path string, out any) error {
	return c.do(ctx, requestOptions{method: http.MethodGet, path: path, absolute: true}, out)
}

func (c *Client) post(ctx context.Context, path string, body any, headers map[string]string, out any) error {
	return c.do(ctx, requestOptions{
		method: http.MethodPost, path: path, body: body, needsAPIKey: true, headers: headers,
	}, out)
}

// postPublic POSTs without attaching an API key, for token-gated public
// endpoints (e.g. a counterparty responding to a confirmation request).
func (c *Client) postPublic(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, requestOptions{method: http.MethodPost, path: path, body: body, needsAPIKey: false}, out)
}

func (c *Client) patch(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, requestOptions{method: http.MethodPatch, path: path, body: body, needsAPIKey: true}, out)
}

func (c *Client) delete(ctx context.Context, path string) error {
	return c.do(ctx, requestOptions{method: http.MethodDelete, path: path, needsAPIKey: true}, nil)
}

func esc(s string) string { return url.PathEscape(s) }

func withQuery(path string, qs url.Values) string {
	if len(qs) == 0 {
		return path
	}
	return path + "?" + qs.Encode()
}

func setInt(qs url.Values, key string, v *int) {
	if v != nil {
		qs.Set(key, strconv.Itoa(*v))
	}
}

func setStr(qs url.Values, key, v string) {
	if v != "" {
		qs.Set(key, v)
	}
}

func setBool(qs url.Values, key string, v *bool) {
	if v != nil {
		qs.Set(key, strconv.FormatBool(*v))
	}
}

func setFloat(qs url.Values, key string, v *float64) {
	if v != nil {
		qs.Set(key, strconv.FormatFloat(*v, 'f', -1, 64))
	}
}

// Int is a helper for building optional int query/body fields.
func Int(v int) *int { return &v }

// Bool is a helper for building optional bool body fields.
func Bool(v bool) *bool { return &v }

// Float is a helper for building optional float body fields.
func Float(v float64) *float64 { return &v }

// String is a helper for building optional string body fields.
func String(v string) *string { return &v }
