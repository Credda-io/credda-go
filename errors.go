package credda

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Error codes the engine sends. These are the values of `error.code` in an
// error body, taken from apps/api/src/errors.ts, auth.ts, organization.ts,
// stream.ts and routes/investigations.ts, and cross-checked against ERROR_CODES
// in apps/api/src/openapi.ts, which that repository's own test proves is the
// REACHABLE set rather than the declared one. There is no published catalogue
// route; this is the set the source constructs.
const (
	// CodeInvalidRequest is a 400 for a malformed request.
	CodeInvalidRequest = "INVALID_REQUEST"
	// CodeValidationFailed is a 400 whose message names the offending query
	// parameter or body field, e.g. "limit: Number must be less than or equal
	// to 100".
	CodeValidationFailed = "VALIDATION_FAILED"
	// CodeNotFound is a 404 for a resource that does not exist, or exists in
	// another organisation. The API does not distinguish the two, deliberately.
	CodeNotFound = "NOT_FOUND"
	// CodeNoOrganization is the 404 the /api/organization routes answer when
	// the request names no organisation, which is every request on a deployment
	// running CREDDA_AUTH=disabled. It is not an authentication failure: the
	// deployment did not ask for credentials. What is missing is the resource
	// "the current organisation", which does not exist for such a request.
	CodeNoOrganization = "NO_ORGANIZATION"
	// CodeAlreadyFinished is the 409 CancelInvestigation answers when the run
	// already reached a terminal state: there is nothing to stop, and nothing
	// to undo. Only on the cancel route.
	CodeAlreadyFinished = "ALREADY_FINISHED"
	// CodeNotCancellable is the 409 CancelInvestigation answers when the run is
	// executing outside the job queue — `credda run` runs the engine in its own
	// process against the same database — so this API has no way to reach it
	// and will not pretend otherwise. Only on the cancel route.
	CodeNotCancellable = "NOT_CANCELLABLE"
	// CodePayloadTooLarge is the 413 for a request body over 256KB.
	CodePayloadTooLarge = "PAYLOAD_TOO_LARGE"
	// CodeUnauthenticated is the 401 from the auth gate: no bearer token, or a
	// token that verifies against no live key.
	CodeUnauthenticated = "UNAUTHENTICATED"
	// CodeTooManyStreams is the 503 for exceeding the process-wide budget of 64
	// concurrent event streams. Transient, and retried when WithRetries is on.
	CodeTooManyStreams = "TOO_MANY_STREAMS"
	// CodeUnavailable is a 503 for a dependency that could not answer.
	//
	// No response can actually carry it: it is the default second argument of
	// unavailable() in apps/api/src/errors.ts and every call site passes its
	// own code over it. Kept because removing an exported constant breaks a
	// build for no gain, and said here so it is not read as something to
	// branch on.
	CodeUnavailable = "UNAVAILABLE"
	// CodeInternalError is the opaque 500. The engine logs the real failure
	// server-side and sends nothing back, so stack traces and SQL text never
	// cross the boundary. Quote RequestID when reporting one.
	CodeInternalError = "INTERNAL_ERROR"
)

// APIError is returned for any non-2xx response. It carries the HTTP status,
// the engine's own error code and message, and the request path.
//
// RequestID is the X-Request-Id correlation id. Log it: on a CodeInternalError
// the message is deliberately empty of detail, and the id is the only thing
// that finds the failure in the engine's logs.
type APIError struct {
	StatusCode int
	// Code is the engine's `error.code`, e.g. CodeNotFound. Empty when the
	// response body was not the engine's error envelope, which is what a proxy
	// in front of it sends.
	Code string
	// Message is the engine's `error.message`.
	Message string
	// Path is the request path, relative to the API root.
	Path string
	// RequestID is the X-Request-Id correlation id for this request.
	RequestID string
	// RetryAfter is a requested back-off. The engine never sends one; a proxy
	// or rate limiter in front of it may. Zero when absent.
	RetryAfter time.Duration
	// Body is the raw response body, for the case where it was not the
	// engine's envelope and Code is therefore empty.
	Body []byte
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("credda: %s (status %d, code %s, path %s, requestId %s)",
			e.Message, e.StatusCode, e.codeOrUnknown(), e.Path, e.RequestID)
	}
	return fmt.Sprintf("credda: %s (status %d, code %s, path %s)",
		e.Message, e.StatusCode, e.codeOrUnknown(), e.Path)
}

func (e *APIError) codeOrUnknown() string {
	if e.Code == "" {
		return "-"
	}
	return e.Code
}

// IsNotFound reports whether err is a 404 from the engine. Note that a resource
// belonging to another organisation is reported as not found, so this does not
// distinguish "no such id" from "not yours".
func IsNotFound(err error) bool {
	e, ok := AsAPIError(err)
	return ok && e.StatusCode == http.StatusNotFound
}

// IsUnauthenticated reports whether err is the auth gate's 401.
func IsUnauthenticated(err error) bool {
	e, ok := AsAPIError(err)
	return ok && e.StatusCode == http.StatusUnauthorized
}

// AsAPIError reports whether err is (or wraps) an *APIError, returning it.
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}

// apiErrorFrom builds an APIError from a non-2xx response.
//
// The envelope is `{"error":{"code":…,"message":…}}` on every path through
// apps/api/src/errors.ts and the auth gate. Anything else is something between
// the caller and the engine, and is reported with the status and the raw body
// rather than being forced into a shape it does not have.
func apiErrorFrom(resp *http.Response, path string, raw []byte, readErr error) *APIError {
	out := &APIError{
		StatusCode: resp.StatusCode,
		Path:       path,
		RequestID:  resp.Header.Get("X-Request-Id"),
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		Body:       raw,
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if readErr == nil && json.Unmarshal(raw, &envelope) == nil {
		out.Code = envelope.Error.Code
		out.Message = envelope.Error.Message
	}
	if out.Message == "" {
		out.Message = fmt.Sprintf("request failed (%d)", resp.StatusCode)
	}
	return out
}

// parseRetryAfter converts a Retry-After header into a duration. Accepts both
// the delta-seconds form and the HTTP-date form the spec permits; returns 0 for
// absent or unparseable values, and never returns negative.
func parseRetryAfter(rawHeader string, now time.Time) time.Duration {
	rawHeader = strings.TrimSpace(rawHeader)
	if rawHeader == "" {
		return 0
	}
	if secs, err := strconv.Atoi(rawHeader); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if at, err := http.ParseTime(rawHeader); err == nil {
		if d := at.Sub(now); d > 0 {
			return d
		}
		return 0
	}
	return 0
}
