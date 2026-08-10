package credda

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// serverWithHeaders replies with the given status, body and response headers:
// the error path is mostly about headers, which newTestServer doesn't set.
func serverWithHeaders(t *testing.T, status int, body string, headers map[string]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return NewClient(WithBaseURL(srv.URL), WithAPIKey("crd_test_key"))
}

func apiErrorFrom(t *testing.T, err error) *APIError {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	apiErr, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	return apiErr
}

func TestAPIErrorCarriesRequestIDFromHeader(t *testing.T) {
	c := serverWithHeaders(t, http.StatusNotFound,
		`{"error":"Not found","code":"USER_NOT_FOUND","requestId":"rq-1","retryable":false}`,
		map[string]string{"X-Request-Id": "rq-1"})

	_, err := c.GetScore(context.Background(), "u1")
	e := apiErrorFrom(t, err)

	if e.RequestID != "rq-1" {
		t.Errorf("RequestID = %q, want rq-1", e.RequestID)
	}
	if e.Code != "USER_NOT_FOUND" {
		t.Errorf("Code = %q, want USER_NOT_FOUND", e.Code)
	}
	if e.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", e.StatusCode)
	}
	// The id must appear in Error() so it lands in a plain log line.
	if got := e.Error(); !strings.Contains(got, "rq-1") {
		t.Errorf("Error() = %q, want it to mention the request id", got)
	}
}

func TestAPIErrorFallsBackToBodyRequestID(t *testing.T) {
	c := serverWithHeaders(t, http.StatusForbidden,
		`{"error":"nope","code":"SCOPE_INSUFFICIENT","requestId":"rq-3"}`, nil)

	_, err := c.GetScore(context.Background(), "u1")
	if got := apiErrorFrom(t, err).RequestID; got != "rq-3" {
		t.Errorf("RequestID = %q, want rq-3", got)
	}
}

func TestAPIErrorSurvivesANonJSONBody(t *testing.T) {
	c := serverWithHeaders(t, http.StatusBadGateway, `<html>502</html>`,
		map[string]string{"X-Request-Id": "rq-4"})

	_, err := c.GetScore(context.Background(), "u1")
	e := apiErrorFrom(t, err)

	if e.StatusCode != http.StatusBadGateway {
		t.Errorf("StatusCode = %d, want 502", e.StatusCode)
	}
	// The header survives a body we cannot parse, which is exactly when a
	// correlation id is most useful.
	if e.RequestID != "rq-4" {
		t.Errorf("RequestID = %q, want rq-4", e.RequestID)
	}
	if !strings.Contains(e.Message, "502") {
		t.Errorf("Message = %q, want it to mention the status", e.Message)
	}
}

func TestAPIErrorWithNoCorrelationInfoIsStillUsable(t *testing.T) {
	c := serverWithHeaders(t, http.StatusBadRequest, `{"error":"plain"}`, nil)
	_, err := c.GetScore(context.Background(), "u1")
	e := apiErrorFrom(t, err)
	if e.RequestID != "" || e.RetryAfter != 0 || e.Retryable {
		t.Errorf("expected zero values, got %+v", e)
	}
	if e.Message != "plain" {
		t.Errorf("Message = %q, want plain", e.Message)
	}
}

func TestAPIErrorExposesValidationDetails(t *testing.T) {
	c := serverWithHeaders(t, http.StatusBadRequest,
		`{"error":"eventType: Invalid enum value","code":"VALIDATION_ERROR","details":[{"path":"eventType","message":"Invalid enum value","code":"invalid_enum_value"}]}`,
		map[string]string{"X-Request-Id": "rq-5"})

	_, err := c.ReportEvent(context.Background(), ReportEventInput{UserID: "u", EventType: "NOPE"}, "")
	e := apiErrorFrom(t, err)

	if e.Code != "VALIDATION_ERROR" {
		t.Fatalf("Code = %q, want VALIDATION_ERROR", e.Code)
	}
	var details []struct {
		Path    string `json:"path"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(e.Details, &details); err != nil {
		t.Fatalf("details did not decode: %v", err)
	}
	if len(details) != 1 || details[0].Path != "eventType" {
		t.Errorf("details = %+v, want one entry for eventType", details)
	}
}

func TestAPIErrorExposesRetryAfterAndRetryable(t *testing.T) {
	c := serverWithHeaders(t, http.StatusTooManyRequests,
		`{"error":"quota","code":"QUOTA_EXCEEDED","retryable":true}`,
		map[string]string{"Retry-After": "45", "X-Request-Id": "rq-9"})

	_, err := c.GetScore(context.Background(), "u1")
	e := apiErrorFrom(t, err)

	if e.RetryAfter != 45*time.Second {
		t.Errorf("RetryAfter = %v, want 45s", e.RetryAfter)
	}
	if !e.Retryable {
		t.Error("Retryable = false, want true (the API said this one is safe to repeat)")
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{"delta seconds", "30", 30 * time.Second},
		{"padded", " 5 ", 5 * time.Second},
		{"zero", "0", 0},
		{"absent", "", 0},
		{"unparseable", "soon", 0},
		{"negative clamps to zero", "-10", 0},
		{"http-date", "Wed, 22 Jul 2026 12:00:20 GMT", 20 * time.Second},
		{"http-date already past", "Wed, 22 Jul 2015 12:00:00 GMT", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseRetryAfter(tc.raw, now); got != tc.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestGetErrorCatalog(t *testing.T) {
	c, got := newTestServer(t, http.StatusOK,
		`{"envelope":{"error":"string"},"retryGuidance":"g","tracing":"t","codes":[{"code":"RATE_LIMIT_EXCEEDED","httpStatus":429,"title":"Rate limited","description":"d","whatToDo":"w","retryable":true}]}`)

	catalog, err := c.GetErrorCatalog(context.Background())
	if err != nil {
		t.Fatalf("GetErrorCatalog: %v", err)
	}
	if got.Method != http.MethodGet || got.Path != "/api/v1/errors" {
		t.Errorf("request = %s %s, want GET /api/v1/errors", got.Method, got.Path)
	}
	// Public endpoint: no key should be sent.
	if got.Auth != "" {
		t.Errorf("Authorization = %q, want empty (public endpoint)", got.Auth)
	}
	if len(catalog.Codes) != 1 || !catalog.Codes[0].Retryable || catalog.Codes[0].HTTPStatus != 429 {
		t.Errorf("codes = %+v", catalog.Codes)
	}
}

func TestGetEnums(t *testing.T) {
	c, got := newTestServer(t, http.StatusOK,
		`{"note":"derived","enums":[{"name":"stakeLevel","description":"d","usedIn":["POST /api/v1/events"],"values":[{"value":"HIGH","description":"d","weight":1.5}]}]}`)

	catalog, err := c.GetEnums(context.Background())
	if err != nil {
		t.Fatalf("GetEnums: %v", err)
	}
	if got.Path != "/api/v1/enums" {
		t.Errorf("path = %s, want /api/v1/enums", got.Path)
	}
	if got.Auth != "" {
		t.Errorf("Authorization = %q, want empty (public endpoint)", got.Auth)
	}
	if len(catalog.Enums) != 1 || catalog.Enums[0].Name != "stakeLevel" {
		t.Fatalf("enums = %+v", catalog.Enums)
	}
	v := catalog.Enums[0].Values[0]
	if v.Value != "HIGH" {
		t.Errorf("value = %q, want HIGH", v.Value)
	}
	// The per-enum extras must survive decoding: they are the useful part.
	if w, ok := v.Extra["weight"].(float64); !ok || w != 1.5 {
		t.Errorf("Extra[weight] = %v, want 1.5", v.Extra["weight"])
	}
	if _, leaked := v.Extra["value"]; leaked {
		t.Error("Extra must not duplicate the known fields")
	}
}
