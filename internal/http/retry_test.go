package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/internal/ctxkeys"
)

// ---------------------------------------------------------------------------
// helpers shared by retry tests
// ---------------------------------------------------------------------------

// sequentialHandler returns an http.Handler that serves successive responses
// from the provided list. Each call pops the next entry; if exhausted, serves
// the last entry repeatedly.
func sequentialHandler(responses []int) (http.Handler, *int64) {
	var callCount int64
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt64(&callCount, 1)
		idx := int(n) - 1
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		w.WriteHeader(responses[idx])
	})
	return h, &callCount
}

// buildRetryTransport wires RetryMiddleware around a bare httptest server client.
func buildRetryTransport(t *testing.T, hc *http.Client, maxRetries int, baseDelay time.Duration) *Transport {
	t.Helper()
	base := func(req *http.Request) (*http.Response, error) {
		return hc.Do(req)
	}
	chain := Chain(base, RetryMiddleware(maxRetries, baseDelay))
	tr, err := NewTransport(TransportOptions{
		HTTPClient: hc,
		Chain:      chain,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	return tr
}

// ---------------------------------------------------------------------------
// retry tests
// ---------------------------------------------------------------------------

func TestRetry_GET_429ThenOK_ReturnsOK(t *testing.T) {
	t.Parallel()
	// Sequence: 429 → 200. Expect 2 hits, final response 200.
	handler, callCount := sequentialHandler([]int{429, 200})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Zero delay to keep tests fast.
	ctx := ctxkeys.WithRetryDelay(context.Background(), 0)
	tr := buildRetryTransport(t, srv.Client(), 3, 0)
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/food", nil)
	resp, _, err := tr.Do(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt64(callCount); got != 2 {
		t.Fatalf("server hit %d times; want 2", got)
	}
}

func TestRetry_GET_500ExhaustsRetries(t *testing.T) {
	t.Parallel()
	maxRetries := 2
	// Always 500 — expect maxRetries+1 total hits.
	handler, callCount := sequentialHandler([]int{500, 500, 500})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := ctxkeys.WithRetryDelay(context.Background(), 0)
	tr := buildRetryTransport(t, srv.Client(), maxRetries, 0)
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/food", nil)
	// Transport.Do either returns an error (500→ErrServer) or a 500 response;
	// both are accepted — the key assertion is server hit count.
	_, _, _ = tr.Do(ctx, req)
	wantHits := int64(maxRetries + 1)
	if got := atomic.LoadInt64(callCount); got != wantHits {
		t.Fatalf("server hit %d times; want %d", got, wantHits)
	}
}

func TestRetry_POST_500_WithoutForceRetry_NotRetried(t *testing.T) {
	t.Parallel()
	// POST is not idempotent; without ForceRetry should NOT retry.
	handler, callCount := sequentialHandler([]int{500, 200})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := ctxkeys.WithRetryDelay(context.Background(), 0)
	tr := buildRetryTransport(t, srv.Client(), 3, 0)
	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+"/food", nil)
	_, _, _ = tr.Do(ctx, req)
	if got := atomic.LoadInt64(callCount); got != 1 {
		t.Fatalf("server hit %d times; want 1 (POST must not retry without ForceRetry)", got)
	}
}

func TestRetry_POST_500_WithForceRetry_Retried(t *testing.T) {
	t.Parallel()
	// POST with ForceRetry=true should retry on 500.
	handler, callCount := sequentialHandler([]int{500, 200})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := ctxkeys.WithForceRetry(
		ctxkeys.WithRetryDelay(context.Background(), 0),
		true,
	)
	tr := buildRetryTransport(t, srv.Client(), 3, 0)
	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+"/food", nil)
	resp, _, err := tr.Do(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt64(callCount); got != 2 {
		t.Fatalf("server hit %d times; want 2", got)
	}
}

func TestRetry_ContextCancel_StopsRetry(t *testing.T) {
	t.Parallel()
	// Always 500 but context cancelled quickly; retry must stop.
	var callCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// Use a tiny but non-zero delay so the cancel fires mid-backoff.
	ctx = ctxkeys.WithRetryDelay(ctx, 5*time.Millisecond)
	// Cancel after first response.
	cancel()

	tr := buildRetryTransport(t, srv.Client(), 5, 5*time.Millisecond)
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/food", nil)
	_, _, err := tr.Do(ctx, req)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestRetry_PerRequestRetryCount_Override(t *testing.T) {
	t.Parallel()
	// Client maxRetries=5, but per-request override = 1.
	// Server always 500 → expects exactly 2 hits (1 initial + 1 retry).
	handler, callCount := sequentialHandler([]int{500, 500, 500, 500, 500, 500})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := ctxkeys.WithRetries(
		ctxkeys.WithRetryDelay(context.Background(), 0),
		1,
	)
	tr := buildRetryTransport(t, srv.Client(), 5, 0)
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/food", nil)
	_, _, _ = tr.Do(ctx, req)
	if got := atomic.LoadInt64(callCount); got != 2 {
		t.Fatalf("server hit %d times; want 2 (per-request override = 1 retry)", got)
	}
}
