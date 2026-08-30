package credda

import (
	"context"
	"errors"
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

// THE ONE THAT MATTERS. Creating an investigation commits a model budget, so a
// create sent twice is a second bill. Until core 620ea75 this client refused to
// retry it at all, because the route accepted no idempotency key. It accepts one
// now, and the guarantee is the engine's rather than this abstention: under a
// key the second request is answered with the run the first one opened.
//
// So the create IS retried, and opens exactly one run.
func TestCreateInvestigationOnceIsRetriedAndOpensExactlyOneRun(t *testing.T) {
	var calls, opened int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if r.Header.Get("Idempotency-Key") == "" {
			t.Error("the retried create carried no Idempotency-Key; without one a repeat opens a second run")
		}
		// The engine records the run on the first request and the response is
		// lost on the way back. The retry carries the same key, so the engine
		// hands back that same run with a 200 rather than opening a second.
		if n == 1 {
			atomic.AddInt32(&opened, 1)
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
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"investigation":{"id":"inv_1","state":"CREATED"}}`))
	}))
	defer srv.Close()

	req, err := NewIdempotentCreate(CreateInvestigationInput{
		RepositoryID: "repo_1", IssueTitle: "Checkout 500s", IssueBody: "on submit",
	})
	if err != nil {
		t.Fatalf("NewIdempotentCreate: %v", err)
	}
	created, err := fastRetryClient(srv.URL, 5).CreateInvestigationOnce(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateInvestigationOnce: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("the create was sent %d times; want 2 — one lost, one retried", got)
	}
	if got := atomic.LoadInt32(&opened); got != 1 {
		t.Fatalf("the engine opened %d runs; the key exists so that it opens exactly one", got)
	}
	if created.Status != StatusReplayed {
		t.Errorf("Status = %q, want %q: the retry was answered with the run the first attempt opened",
			created.Status, StatusReplayed)
	}
	if created.Opened() {
		t.Error("Opened() is true on a replay; nothing was created by that request and nothing was billed")
	}
	if created.Key != req.Key() {
		t.Errorf("Key = %q, want %q", created.Key, req.Key())
	}
}

// The keyless create is still never retried, and for the reason it always was:
// with no header the route does exactly what it did before the header existed,
// one run per request.
func TestCreateInvestigationWithoutAKeyIsNeverRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Header.Get("Idempotency-Key") != "" {
			t.Error("CreateInvestigation sent an Idempotency-Key; it must not invent one")
		}
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
		t.Fatalf("CreateInvestigation was sent %d times; a retried keyless POST enqueues a duplicate investigation", got)
	}
}

// A reused key is an answer, not a blip: the engine understood the request and
// refused it, so repeating it earns the same 409.
func TestReusedIdempotencyKeyIsNotRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":{"code":"IDEMPOTENCY_KEY_REUSED","message":"already used"}}`))
	}))
	defer srv.Close()

	req, err := NewIdempotentCreate(CreateInvestigationInput{RepositoryID: "repo_1", IssueTitle: "t", IssueBody: "b"})
	if err != nil {
		t.Fatalf("NewIdempotentCreate: %v", err)
	}
	_, err = fastRetryClient(srv.URL, 5).CreateInvestigationOnce(context.Background(), req)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != CodeIdempotencyKeyReused {
		t.Fatalf("err = %v, want a 409 %s", err, CodeIdempotencyKeyReused)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("the refusal was sent %d times; want 1", got)
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
