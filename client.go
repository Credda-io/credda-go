// Package credda is the official Go client for the Credda engine API.
//
// Credda finds defects and security vulnerabilities in a company's production
// and QA environments, reproduces the failure, diagnoses the cause, writes the
// patch, proves it with a test that fails before and passes after, and opens a
// pull request. It proposes; it never merges.
//
// This package is a typed reader over that engine's HTTP API. Every method here
// corresponds to a route mounted in the engine (apps/api/src/app.ts) and every
// field to a value written by its serializers (apps/api/src/serialize.ts).
// Nothing else is here: if the engine does not serve it, this client does not
// name it.
//
// # What this client can do
//
// The surface is read-mostly, because the API is. There is exactly one write:
// CreateInvestigation, which enqueues an investigation in state CREATED.
// Execution is driven by the engine's worker, not by the API, so there is no
// method here that starts, cancels, or advances a run, and none that creates a
// validation, a patch, or a pull request. Those are the worker's, and the API
// exposes no route for them.
//
// # Authentication
//
// A deployment sets CREDDA_AUTH to "enforced" or "disabled" (apps/api/src/auth.ts).
// When enforced, every /api route requires an organisation's API key as an
// RFC 6750 bearer token; pass it with WithAPIKey. When disabled, requests carry
// no organisation, and the /api/organization routes answer 404 NO_ORGANIZATION
// for that reason rather than guessing one. GET /livez needs no credential in
// either mode and discloses nothing.
//
// # Stability
//
// See the versioning section of the README. This module is pre-1.0 and the tags
// before v0.4.0 describe a different, retired product.
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

// DefaultBaseURL is the engine's default local address. There is no hosted
// default: Credda runs against a customer's own deployment, so a base URL is
// something a caller supplies with WithBaseURL rather than something this
// package knows.
const DefaultBaseURL = "http://localhost:3001"

// apiPrefix is prepended to every /api route. /livez is mounted outside it,
// ahead of the auth gate (apps/api/src/app.ts).
const apiPrefix = "/api"

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

// WithAPIKey sets the organisation API key sent as `Authorization: Bearer …`.
//
// The key names an organisation, never a person: api_keys has an org_id and no
// user_id, which is why every scoped read is scoped by organisation and why
// OrganizationMember.RoleEnforced is false. Keys are minted out of band by the
// operator; the API has no route that creates or revokes one.
func WithAPIKey(key string) Option {
	return func(c *Client) { c.apiKey = key }
}

// WithBaseURL sets the engine's API root. Trailing slashes are trimmed.
func WithBaseURL(base string) Option {
	return func(c *Client) {
		if base != "" {
			c.baseURL = strings.TrimRight(base, "/")
		}
	}
}

// WithHTTPClient supplies a custom *http.Client (timeouts, transport, proxies).
//
// Use it for Stream: the default client's 30s timeout applies to the whole
// response, and an event stream is meant to stay open for longer than that.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithRetries enables opt-in automatic retries of transient failures: network
// errors, and 429/502/503/504. n is the number of re-attempts; 0 (the default)
// is off.
//
// Applied to GETs only. Every GET on this API is a read and repeating one
// changes nothing. CreateInvestigation is never retried: the API accepts no
// idempotency key, so a repeated POST would enqueue a second investigation for
// the same report, and two runs against one issue is not a thing this client
// will cause on its own.
//
// Backoff is 300ms doubling per attempt, or the server's own Retry-After when
// one was sent, capped at 5s either way. Tune with WithRetryBackoff.
func WithRetries(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.retries = n
		}
	}
}

// WithRetryBackoff overrides the first backoff wait and the ceiling on any
// single wait (defaults 300ms and 5s). The cap matters: a Retry-After from a
// proxy in front of the engine can run to minutes, and without a ceiling a
// retry would hang the call for as long as that header says.
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
// 30s-timeout HTTP client, no API key and no retries.
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

// ── transport ───────────────────────────────────────────────────────────────

type requestOptions struct {
	method   string
	path     string // relative to apiPrefix unless absolute is true
	absolute bool   // path is relative to the API root (for /livez)
	body     any
	headers  map[string]string
	accept   string
	// raw suppresses JSON decoding; the caller is handed the response instead.
	raw bool
	// noRetry opts a GET out of the retry policy even when one is configured.
	// Exactly one route sets it; see GetHealth.
	noRetry bool
}

// safeToRepeat reports whether repeating this request can only ever be
// exactly-once. GETs, and nothing else: see WithRetries.
//
// noRetry is a separate question from safety. A route may set it because
// repeating the request is pointless rather than because it is unsafe.
func (ro requestOptions) safeToRepeat() bool {
	return ro.method == http.MethodGet && !ro.noRetry
}

// retryable decides whether err is worth repeating. A non-APIError is a
// transport failure, which is. An APIError is when the status is transient:
// 429 and 5xx gateway statuses from a proxy, and 503, which is what the engine
// itself answers for UNAVAILABLE and TOO_MANY_STREAMS.
func retryable(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return true
	}
	switch apiErr.StatusCode {
	case 429, 502, 503, 504:
		return true
	}
	return false
}

// retryDelay is the wait before attempt i (1-based). A Retry-After wins when
// one was sent: whatever put it there knows when the window reopens, and
// waiting less just earns the same answer again.
func (c *Client) retryDelay(i int, lastErr error) time.Duration {
	delay := c.maxRetryDelay
	if i-1 < 62 {
		delay = c.retryBase << (i - 1)
	}
	var apiErr *APIError
	if errors.As(lastErr, &apiErr) && apiErr.RetryAfter > 0 {
		delay = apiErr.RetryAfter
	}
	if delay > c.maxRetryDelay || delay < 0 {
		delay = c.maxRetryDelay
	}
	return delay
}

func (c *Client) do(ctx context.Context, ro requestOptions, out any) error {
	_, err := c.doRaw(ctx, ro, out)
	return err
}

// doRaw runs the request with retries. When ro.raw is set the *http.Response is
// returned with its body unread and unclosed, which is what Stream needs.
func (c *Client) doRaw(ctx context.Context, ro requestOptions, out any) (*http.Response, error) {
	var encoded []byte
	if ro.body != nil {
		b, err := json.Marshal(ro.body)
		if err != nil {
			return nil, fmt.Errorf("credda: encoding request body: %w", err)
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
			timer := time.NewTimer(c.retryDelay(i, lastErr))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		resp, err := c.attempt(ctx, ro, encoded, out)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) attempt(ctx context.Context, ro requestOptions, encoded []byte, out any) (*http.Response, error) {
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
		return nil, fmt.Errorf("credda: building request: %w", err)
	}
	if encoded != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Sent whenever a key is configured. There is no per-route "public" flag,
	// because CREDDA_AUTH is a deployment-wide switch: a disabled deployment
	// ignores the header, an enforced one requires it on every /api route.
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if ro.accept != "" {
		req.Header.Set("Accept", ro.accept)
	}
	for k, v := range ro.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("credda: %s %s: %w", ro.method, ro.path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		raw, readErr := io.ReadAll(resp.Body)
		return nil, apiErrorFrom(resp, ro.path, raw, readErr)
	}

	if ro.raw {
		return resp, nil
	}
	defer resp.Body.Close()

	if out == nil {
		return resp, nil
	}
	raw, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("credda: reading response body: %w", readErr)
	}
	if s, ok := out.(*string); ok {
		*s = string(raw)
		return resp, nil
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		// 204 or an empty body for a caller that expected JSON. Leave the zero
		// value rather than reporting a decode failure for a body that is
		// legitimately absent.
		return resp, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("credda: decoding response from %s: %w", ro.path, err)
	}
	return resp, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, requestOptions{method: http.MethodGet, path: path}, out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, requestOptions{method: http.MethodPost, path: path, body: body}, out)
}

// ── query building ──────────────────────────────────────────────────────────

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

// Int is a helper for building optional int query fields.
func Int(v int) *int { return &v }

// Bool is a helper for building optional bool query fields.
func Bool(v bool) *bool { return &v }

// String is a helper for building optional string fields.
func String(v string) *string { return &v }

// Page is the limit/offset pair every paged route on this API accepts.
//
// Nil means "let the server decide", which is limit=50, offset=0 everywhere.
// The server clamps limit to 100 on every route and refuses anything larger
// with a 400 rather than quietly reducing it.
type Page struct {
	Limit  *int
	Offset *int
}

func (p *Page) apply(qs url.Values) {
	if p == nil {
		return
	}
	setInt(qs, "limit", p.Limit)
	setInt(qs, "offset", p.Offset)
}
