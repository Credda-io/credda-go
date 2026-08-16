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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&calls, 1)) - 1
		if n >= len(statuses) {
			n = len(statuses) - 1
		}
		code := statuses[n]
		if code >= 400 && retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(code)
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestRetryPolicy(t *testing.T) {
	fast := func(base string, opts ...Option) *Client {
		return NewClient(append([]Option{
			WithBaseURL(base),
			WithAPIKey("crd_test_key"),
			WithRetryBackoff(time.Millisecond, 20*time.Millisecond),
		}, opts...)...)
	}

	tests := []struct {
		name      string
		statuses  []int
		opts      []Option
		call      func(c *Client) error
		wantCalls int32
		wantErr   bool
	}{
		{
			name:     "default is no retry",
			statuses: []int{429, 200},
			call: func(c *Client) error {
				_, err := c.GetBenchmarks(context.Background())
				return err
			},
			wantCalls: 1,
			wantErr:   true,
		},
		{
			name:     "GET retries a transient 503 and succeeds",
			statuses: []int{503, 200},
			opts:     []Option{WithRetries(2)},
			call: func(c *Client) error {
				_, err := c.GetBenchmarks(context.Background())
				return err
			},
			wantCalls: 2,
		},
		{
			name:     "retry count is bounded",
			statuses: []int{503},
			opts:     []Option{WithRetries(2)},
			call: func(c *Client) error {
				_, err := c.GetBenchmarks(context.Background())
				return err
			},
			wantCalls: 3,
			wantErr:   true,
		},
		{
			name:     "non-transient 404 is never retried",
			statuses: []int{404},
			opts:     []Option{WithRetries(3)},
			call: func(c *Client) error {
				_, err := c.GetBenchmarks(context.Background())
				return err
			},
			wantCalls: 1,
			wantErr:   true,
		},
		{
			name:     "idempotency-keyed POST is retried",
			statuses: []int{429, 200},
			opts:     []Option{WithRetries(2)},
			call: func(c *Client) error {
				_, err := c.CreateConfirmationRequest(context.Background(), CreateConfirmationInput{}, "order-42")
				return err
			},
			wantCalls: 2,
		},
		{
			name:     "bare POST is never retried",
			statuses: []int{429, 200},
			opts:     []Option{WithRetries(3)},
			call: func(c *Client) error {
				_, err := c.CreateConfirmationRequest(context.Background(), CreateConfirmationInput{}, "")
				return err
			},
			wantCalls: 1,
			wantErr:   true,
		},
		{
			name:     "keyless single-use respond is never retried",
			statuses: []int{503, 200},
			opts:     []Option{WithRetries(3)},
			call: func(c *Client) error {
				_, err := c.RespondToConfirmation(context.Background(), "cf_1", "tok", "confirm")
				return err
			},
			wantCalls: 1,
			wantErr:   true,
		},
		{
			name:     "DELETE is never retried",
			statuses: []int{503, 200},
			opts:     []Option{WithRetries(3)},
			call: func(c *Client) error {
				return c.DeletePolicy(context.Background(), "pol_1")
			},
			wantCalls: 1,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, calls := statusServer(t, "", tc.statuses...)
			err := tc.call(fast(srv.URL, tc.opts...))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if got := atomic.LoadInt32(calls); got != tc.wantCalls {
				t.Fatalf("requests = %d, want %d", got, tc.wantCalls)
			}
		})
	}
}

func TestServerVerdictDecidesUncoveredStatuses(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		wantCalls int32
	}{
		{"catalog says retryable", `{"code":"INTERNAL_ERROR","retryable":true}`, 3},
		{"catalog says it is not", `{"code":"PLAN_REQUIRED","retryable":false}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(500)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c := NewClient(WithBaseURL(srv.URL), WithRetries(2),
				WithRetryBackoff(time.Millisecond, 20*time.Millisecond))
			if _, err := c.GetBenchmarks(context.Background()); err == nil {
				t.Fatal("expected an error")
			}
			if got := atomic.LoadInt32(&calls); got != tc.wantCalls {
				t.Fatalf("requests = %d, want %d", got, tc.wantCalls)
			}
		})
	}
}

func TestMissingAPIKeyIsNotRetried(t *testing.T) {
	srv, calls := statusServer(t, "", 200)
	c := NewClient(WithBaseURL(srv.URL), WithRetries(3),
		WithRetryBackoff(time.Second, time.Second))
	start := time.Now()
	if err := c.DeletePolicy(context.Background(), "pol_1"); err == nil {
		t.Fatal("expected an error")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("took %v, a client-side precondition must fail without backoff", elapsed)
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}

func TestRetryAfterIsHonoredAndCapped(t *testing.T) {
	t.Run("honored over exponential backoff", func(t *testing.T) {
		srv, calls := statusServer(t, "1", 429, 200)
		c := NewClient(WithBaseURL(srv.URL), WithRetries(1),
			WithRetryBackoff(50*time.Millisecond, 5*time.Second))
		start := time.Now()
		if _, err := c.GetBenchmarks(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed := time.Since(start); elapsed < time.Second {
			t.Fatalf("waited %v, expected the server's 1s Retry-After to win over the 50ms base", elapsed)
		}
		if got := atomic.LoadInt32(calls); got != 2 {
			t.Fatalf("requests = %d, want 2", got)
		}
	})

	t.Run("capped so a quota reset cannot hang the call", func(t *testing.T) {
		srv, calls := statusServer(t, "86400", 429, 200)
		c := NewClient(WithBaseURL(srv.URL), WithRetries(1),
			WithRetryBackoff(time.Millisecond, 30*time.Millisecond))
		start := time.Now()
		if _, err := c.GetBenchmarks(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("waited %v, expected the 30ms cap to bound a 24h Retry-After", elapsed)
		}
		if got := atomic.LoadInt32(calls); got != 2 {
			t.Fatalf("requests = %d, want 2", got)
		}
	})
}

func TestRetryStopsOnCancelledContext(t *testing.T) {
	srv, calls := statusServer(t, "", 503)
	c := NewClient(WithBaseURL(srv.URL), WithRetries(5),
		WithRetryBackoff(time.Second, 5*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.GetBenchmarks(ctx); err == nil {
		t.Fatal("expected an error")
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}
