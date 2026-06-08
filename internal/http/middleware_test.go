package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/auth"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/cache"
	pkgerrors "github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/errors"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/metrics"
)

// ===========================================================================
// FAKE DOUBLES
// ===========================================================================

// ---------------------------------------------------------------------------
// fakeAuthenticator
// ---------------------------------------------------------------------------

type fakeAuthenticator struct {
	mu           sync.Mutex
	refreshCount int
	// headerErr is returned from GetHeaders/GetHeadersForRequest on first call
	// per-call cycle if non-nil.
	headerErr error
	// headers returned by GetHeaders.
	headers map[string]string
}

func (f *fakeAuthenticator) Authenticate(_ context.Context) error { return nil }
func (f *fakeAuthenticator) IsAuthenticated() bool                { return true }
func (f *fakeAuthenticator) Logout()                              {}

func (f *fakeAuthenticator) GetHeaders(_ context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.headerErr != nil {
		return nil, f.headerErr
	}
	if f.headers != nil {
		return f.headers, nil
	}
	return map[string]string{"Authorization": "Bearer fake-token"}, nil
}

func (f *fakeAuthenticator) Refresh(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshCount++
	return nil
}

func (f *fakeAuthenticator) getRefreshCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refreshCount
}

// fakeAuthenticatorWithRefreshErr calls Refresh successfully but marks the
// second GetHeaders call to also return an auth header (simulating refresh
// giving a new valid token).  It tracks whether 401 was retried.
type stubOAuth1Signer struct {
	fakeAuthenticator
}

func (s *stubOAuth1Signer) GetHeadersForRequest(ctx context.Context, _, _ string, _ url.Values) (map[string]string, error) {
	return s.GetHeaders(ctx)
}

var _ auth.OAuth1RequestSigner = (*stubOAuth1Signer)(nil)

// ---------------------------------------------------------------------------
// fakeCache
// ---------------------------------------------------------------------------

type fakeCache struct {
	mu       sync.Mutex
	data     map[string][]byte
	getCalls int
	setCalls int
}

func newFakeCache() *fakeCache {
	return &fakeCache{data: make(map[string][]byte)}
}

func (c *fakeCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++
	v, ok := c.data[key]
	return v, ok
}

func (c *fakeCache) Set(key string, value []byte, _ time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setCalls++
	c.data[key] = value
}

func (c *fakeCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}

func (c *fakeCache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string][]byte)
}

var _ cache.Cache = (*fakeCache)(nil)

// ---------------------------------------------------------------------------
// spyCollector — extends fakeCollector with per-method tracking
// ---------------------------------------------------------------------------

type spyCollector struct {
	mu               sync.Mutex
	incRequestsCalls int
	incFailuresCalls int
	observeDurCalls  int
	incActiveCalls   int
	decActiveCalls   int
}

func (s *spyCollector) IncRequests(_, _ string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incRequestsCalls++
}
func (s *spyCollector) IncFailures(_, _ string, _ int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incFailuresCalls++
}
func (s *spyCollector) ObserveDuration(_, _ string, _ time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observeDurCalls++
}
func (s *spyCollector) IncActiveConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incActiveCalls++
}
func (s *spyCollector) DecActiveConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decActiveCalls++
}
func (s *spyCollector) AddBytesSent(_ int64)       {}
func (s *spyCollector) AddBytesRecv(_ int64)       {}
func (s *spyCollector) Snapshot() metrics.Snapshot { return metrics.Snapshot{} }

var _ metrics.Collector = (*spyCollector)(nil)

// ---------------------------------------------------------------------------
// hookCapture — Hook that stores all received HookEvents
// ---------------------------------------------------------------------------

type hookCapture struct {
	mu     sync.Mutex
	events []HookEvent
}

func (hc *hookCapture) hook() Hook {
	return func(ev HookEvent) {
		hc.mu.Lock()
		defer hc.mu.Unlock()
		hc.events = append(hc.events, ev)
	}
}

func (hc *hookCapture) all() []HookEvent {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	out := make([]HookEvent, len(hc.events))
	copy(out, hc.events)
	return out
}

// ---------------------------------------------------------------------------
// nullLogger — Logger that discards all output
// ---------------------------------------------------------------------------

type nullLogger struct{}

func (n nullLogger) Debug(_ string, _ map[string]any) {}
func (n nullLogger) Info(_ string, _ map[string]any)  {}
func (n nullLogger) Warn(_ string, _ map[string]any)  {}
func (n nullLogger) Error(_ string, _ map[string]any) {}

var _ Logger = nullLogger{}

// ===========================================================================
// RedactHeaders tests
// ===========================================================================

func TestRedactHeaders_Authorization(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Set("Authorization", "Bearer secret-token")
	h.Set("Content-Type", "application/json")

	redacted := RedactHeaders(h)

	if got := redacted.Get("Authorization"); got != "[REDACTED]" {
		t.Fatalf("Authorization = %q; want [REDACTED]", got)
	}
	if got := redacted.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q; want application/json", got)
	}
	// original must be unmodified
	if got := h.Get("Authorization"); got != "Bearer secret-token" {
		t.Fatalf("original Authorization modified: got %q", got)
	}
}

func TestRedactHeaders_TokenAndSecret(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Set("X-Token", "abc")
	h.Set("X-Secret", "xyz")
	h.Set("X-Safe", "visible")

	redacted := RedactHeaders(h)
	if got := redacted.Get("X-Token"); got != "[REDACTED]" {
		t.Fatalf("X-Token = %q; want [REDACTED]", got)
	}
	if got := redacted.Get("X-Secret"); got != "[REDACTED]" {
		t.Fatalf("X-Secret = %q; want [REDACTED]", got)
	}
	if got := redacted.Get("X-Safe"); got != "visible" {
		t.Fatalf("X-Safe = %q; want visible", got)
	}
}

func TestRedactParams_Token(t *testing.T) {
	t.Parallel()
	v := url.Values{}
	v.Set("token", "my-token-value")
	v.Set("secret", "my-secret-value")
	v.Set("food_id", "12345")

	redacted := RedactParams(v)
	if got := redacted.Get("token"); got != "[REDACTED]" {
		t.Fatalf("token = %q; want [REDACTED]", got)
	}
	if got := redacted.Get("secret"); got != "[REDACTED]" {
		t.Fatalf("secret = %q; want [REDACTED]", got)
	}
	if got := redacted.Get("food_id"); got != "12345" {
		t.Fatalf("food_id = %q; want 12345", got)
	}
	// original unmodified
	if got := v.Get("token"); got != "my-token-value" {
		t.Fatalf("original token modified: got %q", got)
	}
}

// ===========================================================================
// AuthMiddleware tests
// ===========================================================================

func TestAuthMiddleware_HeadersInjected(t *testing.T) {
	t.Parallel()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	a := &fakeAuthenticator{}
	base := func(req *http.Request) (*http.Response, error) {
		return srv.Client().Do(req)
	}
	chain := Chain(base, AuthMiddleware(a))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	_, _, err := tr.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer fake-token" {
		t.Fatalf("Authorization = %q; want Bearer fake-token", gotAuth)
	}
}

func TestAuthMiddleware_401_RefreshThenRetry_Returns200(t *testing.T) {
	t.Parallel()
	var callCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt64(&callCount, 1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	a := &fakeAuthenticator{}
	base := func(req *http.Request) (*http.Response, error) {
		return srv.Client().Do(req)
	}
	chain := Chain(base, AuthMiddleware(a))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/food", nil)
	resp, _, err := tr.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if got := a.getRefreshCount(); got != 1 {
		t.Fatalf("Refresh called %d times; want 1", got)
	}
	if got := atomic.LoadInt64(&callCount); got != 2 {
		t.Fatalf("server hit %d times; want 2", got)
	}
}

func TestAuthMiddleware_401Twice_PropagatesAuthError(t *testing.T) {
	t.Parallel()
	// Server always 401 — after refresh the retry also gets 401.
	// Transport.Do should convert final 401 → AuthenticationError / ErrUnauthorized.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	a := &fakeAuthenticator{}
	base := func(req *http.Request) (*http.Response, error) {
		return srv.Client().Do(req)
	}
	chain := Chain(base, AuthMiddleware(a))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/food", nil)
	_, _, err := tr.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for double 401")
	}
	if !errors.Is(err, pkgerrors.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized; got %v", err)
	}
	// Refresh called exactly once
	if got := a.getRefreshCount(); got != 1 {
		t.Fatalf("Refresh called %d times; want 1", got)
	}
}

func TestAuthMiddleware_OAuth1RequestSigner_Used(t *testing.T) {
	t.Parallel()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// stubOAuth1Signer implements OAuth1RequestSigner — middleware should call
	// GetHeadersForRequest, not GetHeaders.
	signer := &stubOAuth1Signer{}
	signer.headers = map[string]string{"Authorization": "OAuth realm=\"\",oauth_token=fake"}
	base := func(req *http.Request) (*http.Response, error) {
		return srv.Client().Do(req)
	}
	chain := Chain(base, AuthMiddleware(signer))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"?food_id=123", nil)
	_, _, err := tr.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != `OAuth realm="",oauth_token=fake` {
		t.Fatalf("Authorization = %q; want OAuth header", gotAuth)
	}
}

// ===========================================================================
// CacheMiddleware tests
// ===========================================================================

func TestCacheMiddleware_GET_SecondCallFromCache(t *testing.T) {
	t.Parallel()
	var originHits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&originHits, 1)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"cached":"yes"}`))
	}))
	defer srv.Close()

	fc := newFakeCache()
	base := func(req *http.Request) (*http.Response, error) {
		return srv.Client().Do(req)
	}
	chain := Chain(base, CacheMiddleware(fc, 5*time.Minute))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	req1, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/food?id=1", nil)
	if _, _, err := tr.Do(ctx, req1); err != nil {
		t.Fatalf("first call error: %v", err)
	}

	req2, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/food?id=1", nil)
	_, body2, err := tr.Do(ctx, req2)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if string(body2) != `{"cached":"yes"}` {
		t.Fatalf("body = %q; want cached body", body2)
	}
	if got := atomic.LoadInt64(&originHits); got != 1 {
		t.Fatalf("origin hit %d times; want 1 (second should be cached)", got)
	}
}

func TestCacheMiddleware_POST_NotCached(t *testing.T) {
	t.Parallel()
	var originHits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&originHits, 1)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	fc := newFakeCache()
	base := func(req *http.Request) (*http.Response, error) {
		return srv.Client().Do(req)
	}
	chain := Chain(base, CacheMiddleware(fc, 5*time.Minute))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+"/food", nil)
		if _, _, err := tr.Do(ctx, req); err != nil {
			t.Fatalf("POST call %d error: %v", i, err)
		}
	}
	if got := atomic.LoadInt64(&originHits); got != 3 {
		t.Fatalf("origin hit %d times; want 3 (POST must not cache)", got)
	}
	if fc.setCalls != 0 {
		t.Fatalf("cache Set called %d times for POST; want 0", fc.setCalls)
	}
}

func TestCacheMiddleware_GET_CacheMiss_ThenHit(t *testing.T) {
	t.Parallel()
	// Two different URLs must each hit the origin once; same URL twice hits once.
	var originHits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&originHits, 1)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	fc := newFakeCache()
	base := func(req *http.Request) (*http.Response, error) {
		return srv.Client().Do(req)
	}
	chain := Chain(base, CacheMiddleware(fc, time.Minute))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	urls := []string{
		srv.URL + "/a",
		srv.URL + "/b",
		srv.URL + "/a", // cache hit
		srv.URL + "/b", // cache hit
	}
	for _, u := range urls {
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		if _, _, err := tr.Do(ctx, req); err != nil {
			t.Fatalf("error for %s: %v", u, err)
		}
	}
	if got := atomic.LoadInt64(&originHits); got != 2 {
		t.Fatalf("origin hit %d times; want 2 (two unique URLs)", got)
	}
}

// ===========================================================================
// LoggingMiddleware tests
// ===========================================================================

func TestLoggingMiddleware_HookFired_FieldsPopulated(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"food":"data"}`))
	}))
	defer srv.Close()

	capture := &hookCapture{}
	col := &spyCollector{}
	base := func(req *http.Request) (*http.Response, error) {
		return srv.Client().Do(req)
	}
	chain := Chain(base, LoggingMiddleware(nullLogger{}, []Hook{capture.hook()}, col, LogConfig{Enabled: false}))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/food?id=42", nil)
	_, _, err := tr.Do(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := capture.all()
	if len(events) != 1 {
		t.Fatalf("hook fired %d times; want 1", len(events))
	}
	ev := events[0]
	if ev.Method != "GET" {
		t.Fatalf("Method = %q; want GET", ev.Method)
	}
	if ev.URL == "" {
		t.Fatal("URL must be populated")
	}
	if ev.StatusCode != 200 {
		t.Fatalf("StatusCode = %d; want 200", ev.StatusCode)
	}
	if ev.Duration <= 0 {
		t.Fatal("Duration must be > 0")
	}
}

func TestLoggingMiddleware_Collector_IncRequests_ObserveDuration(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	col := &spyCollector{}
	base := func(req *http.Request) (*http.Response, error) {
		return srv.Client().Do(req)
	}
	chain := Chain(base, LoggingMiddleware(nullLogger{}, nil, col, LogConfig{}))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	if _, _, err := tr.Do(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	col.mu.Lock()
	ir := col.incRequestsCalls
	od := col.observeDurCalls
	col.mu.Unlock()

	if ir == 0 {
		t.Fatal("IncRequests not called")
	}
	if od == 0 {
		t.Fatal("ObserveDuration not called")
	}
}

func TestLoggingMiddleware_Collector_IncFailures_OnError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	col := &spyCollector{}
	base := func(req *http.Request) (*http.Response, error) {
		return srv.Client().Do(req)
	}
	chain := Chain(base, LoggingMiddleware(nullLogger{}, nil, col, LogConfig{}))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	_, _, _ = tr.Do(ctx, req)

	col.mu.Lock()
	ifail := col.incFailuresCalls
	col.mu.Unlock()

	if ifail == 0 {
		t.Fatal("IncFailures not called on 500 response")
	}
}

func TestLoggingMiddleware_HookFired_OnTransportError(t *testing.T) {
	t.Parallel()
	errRT := &errorRoundTripper{err: errors.New("dial error")}
	hc := &http.Client{Transport: errRT}

	capture := &hookCapture{}
	col := &spyCollector{}
	base := func(req *http.Request) (*http.Response, error) {
		return hc.Do(req)
	}
	chain := Chain(base, LoggingMiddleware(nullLogger{}, []Hook{capture.hook()}, col, LogConfig{}))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: hc})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:1/fail", nil)
	_, _, _ = tr.Do(ctx, req)

	events := capture.all()
	if len(events) != 1 {
		t.Fatalf("hook fired %d times; want 1", len(events))
	}
	if events[0].Error == nil {
		t.Fatal("HookEvent.Error must be non-nil for transport error")
	}
}

// ===========================================================================
// Chain tests
// ===========================================================================

func TestChain_Order(t *testing.T) {
	t.Parallel()
	// Verify outermost middleware runs first.
	var order []string
	makeMiddleware := func(name string) Middleware {
		return func(next RoundTripFunc) RoundTripFunc {
			return func(req *http.Request) (*http.Response, error) {
				order = append(order, name+"-in")
				resp, err := next(req)
				order = append(order, name+"-out")
				return resp, err
			}
		}
	}
	base := func(_ *http.Request) (*http.Response, error) {
		order = append(order, "base")
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}
	chain := Chain(base, makeMiddleware("A"), makeMiddleware("B"))
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", nil)
	_, _ = chain(req)

	wantOrder := []string{"A-in", "B-in", "base", "B-out", "A-out"}
	if len(order) != len(wantOrder) {
		t.Fatalf("order = %v; want %v", order, wantOrder)
	}
	for i, want := range wantOrder {
		if order[i] != want {
			t.Fatalf("order[%d] = %q; want %q (full: %v)", i, order[i], want, order)
		}
	}
}
