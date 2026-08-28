package credda

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// statusServer replies with each status in turn, repeating the last one, and
// counts requests.
func statusServer(t *testing.T, retryAfter string, statuses ...int) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(atomic.AddInt32(&calls, 1)) - 1
		if n >= len(statuses) {
			n = len(statuses) - 1
		}
		code := statuses[n]
		if code >= 400 && retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if code >= 400 {
			_, _ = w.Write([]byte(`{"error":{"code":"UNAVAILABLE","message":"not now"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"total":0}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func fastRetryClient(base string, n int) *Client {
	return NewClient(
		WithBaseURL(base),
		WithAPIKey("k"),
		WithRetries(n),
		WithRetryBackoff(time.Millisecond, 5*time.Millisecond),
	)
}

func TestRetriesAreOffByDefault(t *testing.T) {
	srv, calls := statusServer(t, "", 503, 200)
	c := NewClient(WithBaseURL(srv.URL), WithAPIKey("k"))
	if _, err := c.ListInvestigations(context.Background(), nil); err == nil {
		t.Fatal("want the 503 to surface with retries off")
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Errorf("calls = %d, want 1; retries must be opt-in", got)
	}
}

func TestRetryRecoversFromATransient503(t *testing.T) {
	srv, calls := statusServer(t, "", 503, 503, 200)
	if _, err := fastRetryClient(srv.URL, 3).ListInvestigations(context.Background(), nil); err != nil {
		t.Fatalf("ListInvestigations: %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

// The transient set: 429 and the gateway statuses a proxy in front of the
// engine sends, plus 503, which is what the engine itself answers for
// UNAVAILABLE and TOO_MANY_STREAMS.
func TestTransientStatusesAreRetried(t *testing.T) {
	for _, status := range []int{429, 502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, calls := statusServer(t, "", status, 200)
			if _, err := fastRetryClient(srv.URL, 2).ListInvestigations(context.Background(), nil); err != nil {
				t.Fatalf("status %d: %v", status, err)
			}
			if got := atomic.LoadInt32(calls); got != 2 {
				t.Errorf("status %d: calls = %d, want 2", status, got)
			}
		})
	}
}

// A 4xx that is not 429 is the caller's request being wrong. Repeating it
// identically produces the identical refusal and only burns time.
func TestClientErrorsAreNotRetried(t *testing.T) {
	for _, status := range []int{400, 401, 404, 413} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, calls := statusServer(t, "", status)
			if _, err := fastRetryClient(srv.URL, 3).ListInvestigations(context.Background(), nil); err == nil {
				t.Fatalf("status %d: want an error", status)
			}
			if got := atomic.LoadInt32(calls); got != 1 {
				t.Errorf("status %d: calls = %d, want 1", status, got)
			}
		})
	}
}

// THE ONE THAT MATTERS. CreateInvestigation is the only write on this API and
// it carries no idempotency key, because the API accepts none. A retried POST
// would enqueue a SECOND investigation against the same report, and two runs on
// one issue is not something this client will cause on its own.
func TestCreateInvestigationIsNeverRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":{"code":"UNAVAILABLE","message":"not now"}}`))
	}))
	defer srv.Close()

	c := fastRetryClient(srv.URL, 5)
	_, err := c.CreateInvestigation(context.Background(), CreateInvestigationInput{
		RepositoryID: "repo_1", IssueTitle: "Checkout 500s", IssueBody: "on submit",
	})
	if err == nil {
		t.Fatal("want the 503 to surface")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("CreateInvestigation was sent %d times; a retried POST enqueues a duplicate investigation", got)
	}
}

// A transport failure means nothing answered, so there is nothing to conclude
// from it except to try again.
func TestTransportFailuresAreRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			// Close without a response.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Skip("no hijacker on this server")
			}
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":0}`))
	}))
	defer srv.Close()

	if _, err := fastRetryClient(srv.URL, 4).ListRepositories(context.Background(), nil); err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestRetriesAreBounded(t *testing.T) {
	srv, calls := statusServer(t, "", 503)
	if _, err := fastRetryClient(srv.URL, 2).ListInvestigations(context.Background(), nil); err == nil {
		t.Fatal("want the error after the attempts are spent")
	}
	if got := atomic.LoadInt32(calls); got != 3 {
		t.Errorf("calls = %d, want 3 (1 + 2 re-attempts)", got)
	}
}

// Retry-After wins over the backoff schedule when one is sent, but the cap
// applies either way: without a ceiling, a Retry-After of an hour from a proxy
// would hang the call for an hour.
func TestRetryAfterIsHonouredAndCapped(t *testing.T) {
	srv, _ := statusServer(t, "3600", 503, 200)
	c := fastRetryClient(srv.URL, 2)

	start := time.Now()
	if _, err := c.ListInvestigations(context.Background(), nil); err != nil {
		t.Fatalf("ListInvestigations: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %v; a Retry-After of an hour was not capped", elapsed)
	}
}

func TestRetryDelayUsesRetryAfterOverBackoff(t *testing.T) {
	c := NewClient(WithRetryBackoff(time.Millisecond, time.Minute))
	err := &APIError{StatusCode: 429, RetryAfter: 7 * time.Second}
	if got := c.retryDelay(1, err); got != 7*time.Second {
		t.Errorf("retryDelay = %v, want 7s from Retry-After", got)
	}
}

func TestRetryDelayBacksOffExponentiallyAndCaps(t *testing.T) {
	c := NewClient(WithRetryBackoff(100*time.Millisecond, 400*time.Millisecond))
	want := []time.Duration{100, 200, 400, 400, 400}
	for i, w := range want {
		if got := c.retryDelay(i+1, nil); got != w*time.Millisecond {
			t.Errorf("retryDelay(%d) = %v, want %v", i+1, got, w*time.Millisecond)
		}
	}
	// A huge attempt number must not shift past the width of the duration and
	// come back as a negative or tiny wait.
	if got := c.retryDelay(9999, nil); got != 400*time.Millisecond {
		t.Errorf("retryDelay(9999) = %v, want the cap", got)
	}
}

// A cancelled context must interrupt the WAIT, not only the request. A caller
// that gave up should not be held for the remaining backoff.
func TestContextCancellationInterruptsTheBackoff(t *testing.T) {
	srv, _ := statusServer(t, "", 503)
	c := NewClient(WithBaseURL(srv.URL), WithAPIKey("k"),
		WithRetries(5), WithRetryBackoff(2*time.Second, 10*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := c.ListInvestigations(ctx, nil); err == nil {
		t.Fatal("want an error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %v after the context was done", elapsed)
	}
}
