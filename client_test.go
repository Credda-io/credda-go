package credda

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// capture is what a test server saw.
type capture struct {
	Method string
	Path   string
	Query  string
	Body   string
	// RawURI is the request-target exactly as it arrived on the wire, before
	// Go's server decoded percent-escapes into URL.Path. It is the only place
	// an escaped path separator is still visible.
	RawURI string
	Auth   string
	Accept string
	// Idempotency is the Idempotency-Key header, which only the create route
	// sends and only under an IdempotentCreate.
	Idempotency string
	Calls       int
}

// newTestServer stands up a server that answers every request with status and
// body, and records the last request it saw.
func newTestServer(t *testing.T, status int, body string) (*Client, *capture) {
	t.Helper()
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.Method = r.Method
		got.Path = r.URL.Path
		got.RawURI = r.RequestURI
		got.Query = r.URL.RawQuery
		got.Body = string(raw)
		got.Auth = r.Header.Get("Authorization")
		got.Accept = r.Header.Get("Accept")
		got.Idempotency = r.Header.Get(IdempotencyHeader)
		got.Calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(WithAPIKey("test-key"), WithBaseURL(srv.URL)), got
}

// serveJSON is newTestServer for tests that only care about the decoded body.
func serveJSON(t *testing.T, body string) *Client {
	t.Helper()
	c, _ := newTestServer(t, 200, body)
	return c
}

// ── the defining property ───────────────────────────────────────────────────

// TestModuleHasNoThirdPartyDependencies is the test that guards the one claim
// this repository makes about itself.
//
// The README says: standard library only, no third-party modules. That is not a
// stylistic preference. This package is imported into a customer's build, and
// every dependency it takes is a dependency they take, in a product whose whole
// argument is that Credda's output can be audited. A `require` block appearing
// here is a change to what the README promises, and it should fail a test
// rather than pass a review.
//
// go.mod is read from disk deliberately: `go list -m all` would report the
// resolved graph, and this asserts the source of truth a reader sees.
func TestModuleHasNoThirdPartyDependencies(t *testing.T) {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	for i, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "require") || strings.HasPrefix(trimmed, "replace") {
			t.Errorf("go.mod:%d declares a dependency: %q\n"+
				"This module is standard library only. Adding a dependency changes what "+
				"the README promises and what every importing build takes on.", i+1, trimmed)
		}
	}
}

// ── construction ────────────────────────────────────────────────────────────

func TestNewClientDefaults(t *testing.T) {
	c := NewClient()
	if c.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, DefaultBaseURL)
	}
	if c.apiKey != "" {
		t.Errorf("apiKey = %q, want empty", c.apiKey)
	}
	if c.retries != 0 {
		t.Errorf("retries = %d, want 0 (retries are opt-in)", c.retries)
	}
	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", c.httpClient.Timeout)
	}
}

func TestWithBaseURLTrimsTrailingSlash(t *testing.T) {
	c := NewClient(WithBaseURL("https://credda.internal:3001///"))
	if c.baseURL != "https://credda.internal:3001" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
}

func TestNewClientIgnoresEmptyOverrides(t *testing.T) {
	c := NewClient(WithBaseURL(""), WithHTTPClient(nil), WithRetries(0), WithRetryBackoff(0, 0), nil)
	if c.baseURL != DefaultBaseURL || c.httpClient == nil {
		t.Errorf("an empty override replaced a working default: %q %v", c.baseURL, c.httpClient)
	}
}

// ── the bearer token ────────────────────────────────────────────────────────

// The gate is deployment-wide (CREDDA_AUTH), not per-route, so the key goes on
// every request whenever one is configured. An enforced deployment needs it on
// every /api route including /api/health and /api/metrics; a disabled one
// ignores it.
func TestAPIKeyIsSentOnEveryRoute(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*Client) error
	}{
		{"ListInvestigations", func(c *Client) error { _, err := c.ListInvestigations(context.Background(), nil); return err }},
		{"GetHealth", func(c *Client) error { _, err := c.GetHealth(context.Background()); return err }},
		{"Metrics", func(c *Client) error { _, err := c.Metrics(context.Background()); return err }},
		{"GetOrganization", func(c *Client) error { _, err := c.GetOrganization(context.Background()); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, got := newTestServer(t, 200, `{}`)
			if err := tc.call(c); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got.Auth != "Bearer test-key" {
				t.Errorf("Authorization = %q, want %q", got.Auth, "Bearer test-key")
			}
		})
	}
}

// A client with no key sends no header. That is a valid configuration: a
// deployment running CREDDA_AUTH=disabled expects none, and Livez needs none in
// either mode. The client must not manufacture a refusal the server would not
// have made.
func TestNoAPIKeySendsNoAuthorizationHeader(t *testing.T) {
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	if err := c.Livez(context.Background()); err != nil {
		t.Fatalf("Livez: %v", err)
	}
	if got.Auth != "" {
		t.Errorf("Authorization = %q, want no header", got.Auth)
	}
}

// ── path and prefix ─────────────────────────────────────────────────────────

// /livez is mounted at the root, ahead of the auth gate, and NOT under /api.
// Prefixing it would ask a route that does not exist and get the 404 the engine
// serves for an unknown route — which a caller would read as "the process is
// down" when it is serving perfectly.
func TestLivezIsNotUnderTheAPIPrefix(t *testing.T) {
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Path = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := NewClient(WithBaseURL(srv.URL)).Livez(context.Background()); err != nil {
		t.Fatalf("Livez: %v", err)
	}
	if got.Path != "/livez" {
		t.Errorf("path = %q, want /livez", got.Path)
	}
}

// An id with a slash or a question mark in it must not be able to reach a
// different route or smuggle a query parameter. Assert on the raw request
// target: Go's own server decodes %2F back into a slash before filling
// URL.Path, so URL.Path cannot tell an escaped separator from a real one.
func TestPathIDsAreEscaped(t *testing.T) {
	c, got := newTestServer(t, 200, `{}`)
	_, _ = c.GetInvestigation(context.Background(), "inv/../organization?x=1")
	if !strings.HasPrefix(got.RawURI, "/api/investigations/inv%2F..%2Forganization%3Fx=1") {
		t.Errorf("an id was not escaped into a single path segment: %q", got.RawURI)
	}
	if strings.Contains(got.RawURI, "/organization") {
		t.Errorf("an id reached another route: %q", got.RawURI)
	}
	if got.Query != "" {
		t.Errorf("an id smuggled a query string: %q", got.Query)
	}
}

// ── responses ───────────────────────────────────────────────────────────────

// A 204 with no body is a success, not a decode failure. Livez is exactly this.
func TestEmptyBodyLeavesTheZeroValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	out, err := NewClient(WithBaseURL(srv.URL)).ListInvestigations(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListInvestigations: %v", err)
	}
	if out == nil || len(out.Investigations) != 0 || out.Total != 0 {
		t.Errorf("empty body did not decode to a zero value: %+v", out)
	}
}

func TestMalformedJSONIsAnError(t *testing.T) {
	c := serveJSON(t, `{"investigations":`)
	if _, err := c.ListInvestigations(context.Background(), nil); err == nil {
		t.Fatal("want a decode error for a truncated body, got nil")
	}
}

// Metrics is Prometheus exposition, not JSON. It must come back verbatim.
func TestMetricsReturnsTextVerbatim(t *testing.T) {
	body := "# HELP credda_http_requests_total Total requests\n" +
		"# TYPE credda_http_requests_total counter\n" +
		`credda_http_requests_total{route="/api/investigations"} 41` + "\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	out, err := NewClient(WithBaseURL(srv.URL), WithAPIKey("k")).Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if out != body {
		t.Errorf("Metrics reshaped the exposition:\ngot  %q\nwant %q", out, body)
	}
}

// ── context ─────────────────────────────────────────────────────────────────

func TestCancelledContextStopsTheRequest(t *testing.T) {
	c := serveJSON(t, `{}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.ListInvestigations(ctx, nil); err == nil {
		t.Fatal("want an error for a cancelled context, got nil")
	}
}
