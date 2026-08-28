package credda

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The engine has one error envelope, on every path through
// apps/api/src/errors.ts and the auth gate:
//
//	{"error": {"code": "...", "message": "..."}}
//
// These tests pin that this client reads it, that it survives a body that is
// NOT that envelope (a proxy's 502 page), and that the discriminators a caller
// actually branches on — status, code, request id — arrive intact.

func errorServer(t *testing.T, status int, body string, headers map[string]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(WithBaseURL(srv.URL), WithAPIKey("k"))
}

func TestErrorEnvelopeIsDecoded(t *testing.T) {
	c := errorServer(t, 404,
		`{"error":{"code":"NOT_FOUND","message":"No such investigation: inv_missing"}}`,
		map[string]string{"X-Request-Id": "req_abc123"})

	_, err := c.GetInvestigation(context.Background(), "inv_missing")
	apiErr, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
	if apiErr.Code != CodeNotFound {
		t.Errorf("Code = %q, want %q", apiErr.Code, CodeNotFound)
	}
	if apiErr.Message != "No such investigation: inv_missing" {
		t.Errorf("Message = %q", apiErr.Message)
	}
	if apiErr.Path != "/investigations/inv_missing" {
		t.Errorf("Path = %q", apiErr.Path)
	}
	// The request id is the only handle on a failure whose message the engine
	// deliberately emptied. It must survive.
	if apiErr.RequestID != "req_abc123" {
		t.Errorf("RequestID = %q, want req_abc123", apiErr.RequestID)
	}
	if !strings.Contains(apiErr.Error(), "req_abc123") {
		t.Errorf("Error() drops the request id: %q", apiErr.Error())
	}
}

// A 500 from the engine carries no detail on purpose: stack traces and SQL text
// never cross the boundary. What a caller gets instead is the request id, and
// the client must not paper over the emptiness with an invented explanation.
func TestInternalErrorIsOpaqueAndKeepsItsRequestID(t *testing.T) {
	c := errorServer(t, 500,
		`{"error":{"code":"INTERNAL_ERROR","message":"Internal server error"}}`,
		map[string]string{"X-Request-Id": "req_zzz"})

	_, err := c.ListValidations(context.Background(), nil)
	apiErr, _ := AsAPIError(err)
	if apiErr == nil || apiErr.Code != CodeInternalError {
		t.Fatalf("want %s, got %+v", CodeInternalError, apiErr)
	}
	if apiErr.RequestID != "req_zzz" {
		t.Errorf("RequestID = %q", apiErr.RequestID)
	}
}

// A body that is not the engine's envelope — a proxy's HTML 502 — must not be
// forced into a shape it does not have. Code stays empty, and the raw body is
// kept so the caller can see what actually answered.
func TestNonEnvelopeBodyKeepsStatusAndRawBody(t *testing.T) {
	c := errorServer(t, 502, "<html><body>502 Bad Gateway</body></html>", nil)

	_, err := c.ListRepositories(context.Background(), nil)
	apiErr, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("want *APIError, got %T", err)
	}
	if apiErr.Code != "" {
		t.Errorf("Code = %q, want empty for a non-envelope body", apiErr.Code)
	}
	if apiErr.StatusCode != 502 {
		t.Errorf("StatusCode = %d, want 502", apiErr.StatusCode)
	}
	if !strings.Contains(string(apiErr.Body), "Bad Gateway") {
		t.Errorf("raw body was discarded: %q", apiErr.Body)
	}
	if apiErr.Message == "" {
		t.Error("Message is empty; want a fallback naming the status")
	}
}

// NO_ORGANIZATION is a 404 and not a 401, deliberately: a deployment running
// CREDDA_AUTH=disabled did not ask for credentials, so demanding them says the
// wrong thing. A caller must be able to tell it from a deleted organisation,
// which is what the code is for.
func TestNoOrganizationIsDistinguishableFromAMissingOne(t *testing.T) {
	c := errorServer(t, 404,
		`{"error":{"code":"NO_ORGANIZATION","message":"This request names no organisation. A bearer key is what identifies one."}}`,
		nil)

	_, err := c.GetOrganization(context.Background())
	if !IsNotFound(err) {
		t.Fatalf("want a 404, got %v", err)
	}
	apiErr, _ := AsAPIError(err)
	if apiErr.Code != CodeNoOrganization {
		t.Errorf("Code = %q, want %q", apiErr.Code, CodeNoOrganization)
	}
	if IsUnauthenticated(err) {
		t.Error("NO_ORGANIZATION reported as an authentication failure; it is not one")
	}
}

func TestUnauthenticatedIsRecognised(t *testing.T) {
	c := errorServer(t, 401,
		`{"error":{"code":"UNAUTHENTICATED","message":"Invalid API key"}}`,
		map[string]string{"WWW-Authenticate": `Bearer realm="credda"`})

	_, err := c.ListInvestigations(context.Background(), nil)
	if !IsUnauthenticated(err) {
		t.Fatalf("want an unauthenticated error, got %v", err)
	}
	if IsNotFound(err) {
		t.Error("a 401 reported as not found")
	}
}

// A query parameter outside a vocabulary is a 400 whose message names the
// field. Passing it through is the whole value of the error.
func TestValidationFailedMessageNamesTheField(t *testing.T) {
	c := errorServer(t, 400,
		`{"error":{"code":"VALIDATION_FAILED","message":"limit: Number must be less than or equal to 100"}}`,
		nil)

	_, err := c.ListInvestigations(context.Background(), &InvestigationQuery{Page: Page{Limit: Int(500)}})
	apiErr, _ := AsAPIError(err)
	if apiErr == nil || apiErr.Code != CodeValidationFailed {
		t.Fatalf("want %s, got %+v", CodeValidationFailed, apiErr)
	}
	if !strings.Contains(apiErr.Message, "limit") {
		t.Errorf("Message does not name the field: %q", apiErr.Message)
	}
}

func TestAsAPIErrorUnwraps(t *testing.T) {
	inner := &APIError{StatusCode: 404, Code: CodeNotFound, Message: "gone"}
	wrapped := fmt.Errorf("loading the queue: %w", inner)
	got, ok := AsAPIError(wrapped)
	if !ok || got != inner {
		t.Fatalf("AsAPIError did not unwrap: %v %v", got, ok)
	}
	if !errors.Is(wrapped, error(inner)) {
		t.Error("errors.Is does not see through the wrap")
	}
}

func TestAsAPIErrorOnATransportError(t *testing.T) {
	// A connection refused is not an APIError: nothing answered.
	c := NewClient(WithBaseURL("http://127.0.0.1:1"), WithAPIKey("k"))
	_, err := c.ListInvestigations(context.Background(), nil)
	if err == nil {
		t.Fatal("want a transport error")
	}
	if _, ok := AsAPIError(err); ok {
		t.Error("a transport failure was reported as an APIError; nothing answered")
	}
	if IsNotFound(err) || IsUnauthenticated(err) {
		t.Error("a transport failure classified as an HTTP outcome")
	}
}

// GetHealth is the one route whose error body is the answer. A degraded engine
// says 503 AND names the check that failed, and throwing the body away would
// force a second request to learn what a caller already asked.
func TestGetHealthReturnsTheBodyOnADegraded503(t *testing.T) {
	c := errorServer(t, 503, `{
		"status":"degraded",
		"schemaVersion":41,
		"expectedSchemaVersion":42,
		"checks":[
			{"name":"database","status":"ok","detail":"query answered"},
			{"name":"migrations","status":"failed","detail":"at version 41, this build expects 42"},
			{"name":"artifactStore","status":"unknown","detail":"no artifact root configured"}
		]
	}`, nil)

	out, err := c.GetHealth(context.Background())
	if err == nil {
		t.Fatal("a degraded engine must still be an error")
	}
	if out == nil {
		t.Fatal("the readiness body was discarded")
	}
	if out.Status != "degraded" {
		t.Errorf("Status = %q", out.Status)
	}
	if len(out.Checks) != 3 {
		t.Fatalf("checks = %d, want 3", len(out.Checks))
	}
	if out.Checks[1].Status != "failed" || !strings.Contains(out.Checks[1].Detail, "expects 42") {
		t.Errorf("the failing check lost its detail: %+v", out.Checks[1])
	}
	// "unknown" is not a softer "failed": it means the check could not be run.
	// It must not be flattened into either of the other two.
	if out.Checks[2].Status != "unknown" {
		t.Errorf("artifactStore status = %q, want unknown", out.Checks[2].Status)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"5", 5 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"not-a-number", 0},
		{now.Add(30 * time.Second).Format(http.TimeFormat), 30 * time.Second},
		{now.Add(-30 * time.Second).Format(http.TimeFormat), 0},
	} {
		if got := parseRetryAfter(tc.in, now); got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
