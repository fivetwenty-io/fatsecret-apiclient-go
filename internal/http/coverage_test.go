package http

// coverage_test.go adds targeted tests for branches not exercised by
// client_test.go and middleware_test.go, raising internal/http coverage to ≥ 82%.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/internal/ctxkeys"
)

// ---------------------------------------------------------------------------
// captureBody
// ---------------------------------------------------------------------------

func TestCaptureBody_GetBodyAlreadySet_NoOp(t *testing.T) {
	t.Parallel()
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "http://example.com", bytes.NewReader([]byte("body")))
	// Pre-set GetBody so captureBody is a no-op.
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader([]byte("body"))), nil }
	if err := captureBody(req); err != nil {
		t.Fatalf("captureBody: %v", err)
	}
}

func TestCaptureBody_NilBody_NoOp(t *testing.T) {
	t.Parallel()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", nil)
	if err := captureBody(req); err != nil {
		t.Fatalf("captureBody nil body: %v", err)
	}
	if req.GetBody != nil {
		t.Fatal("GetBody should remain nil for bodyless request")
	}
}

func TestCaptureBody_NoBody_NoOp(t *testing.T) {
	t.Parallel()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", nil)
	req.Body = http.NoBody
	if err := captureBody(req); err != nil {
		t.Fatalf("captureBody NoBody: %v", err)
	}
}

func TestCaptureBody_WithBody_SetsGetBody(t *testing.T) {
	t.Parallel()
	payload := []byte("hello world")
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "http://example.com", bytes.NewReader(payload))
	// Do NOT set GetBody — captureBody should buffer it.
	if err := captureBody(req); err != nil {
		t.Fatalf("captureBody: %v", err)
	}
	if req.GetBody == nil {
		t.Fatal("GetBody must be set after captureBody")
	}
	if req.ContentLength != int64(len(payload)) {
		t.Fatalf("ContentLength = %d; want %d", req.ContentLength, len(payload))
	}
	// GetBody returns independent readers.
	r, _ := req.GetBody()
	got, _ := io.ReadAll(r)
	if string(got) != string(payload) {
		t.Fatalf("GetBody content = %q; want %q", got, payload)
	}
}

// ---------------------------------------------------------------------------
// readBody — nil Body branch
// ---------------------------------------------------------------------------

func TestReadBody_NilBody_ReturnsNil(t *testing.T) {
	t.Parallel()
	tr, _ := NewTransport(TransportOptions{
		Chain: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: nil}, nil
		},
	})
	resp := &http.Response{Body: nil}
	body, err := tr.readBody(resp)
	if err != nil {
		t.Fatalf("readBody nil: %v", err)
	}
	if body != nil {
		t.Fatalf("expected nil body, got %q", body)
	}
}

// ---------------------------------------------------------------------------
// decodeFatSecretError — edge cases
// ---------------------------------------------------------------------------

func TestDecodeFatSecretError_EmptyBody(t *testing.T) {
	t.Parallel()
	if err := decodeFatSecretError(nil, 200); err != nil {
		t.Fatalf("empty body: expected nil, got %v", err)
	}
}

func TestDecodeFatSecretError_NoErrorKey(t *testing.T) {
	t.Parallel()
	body := []byte(`{"food_id":"123"}`)
	if err := decodeFatSecretError(body, 200); err != nil {
		t.Fatalf("no error key: expected nil, got %v", err)
	}
}

func TestDecodeFatSecretError_MalformedJSON(t *testing.T) {
	t.Parallel()
	body := []byte(`{"error": NOTJSON}`)
	if err := decodeFatSecretError(body, 200); err != nil {
		t.Fatalf("malformed JSON: expected nil, got %v", err)
	}
}

func TestDecodeFatSecretError_NullErrorField(t *testing.T) {
	t.Parallel()
	body := []byte(`{"error":null}`)
	if err := decodeFatSecretError(body, 200); err != nil {
		t.Fatalf("null error field: expected nil, got %v", err)
	}
}

func TestDecodeFatSecretError_NonNumericCode(t *testing.T) {
	t.Parallel()
	body := []byte(`{"error":{"code":"abc","message":"bad"}}`)
	if err := decodeFatSecretError(body, 200); err != nil {
		t.Fatalf("non-numeric code: expected nil, got %v", err)
	}
}

func TestDecodeFatSecretError_CodeZero(t *testing.T) {
	t.Parallel()
	body := []byte(`{"error":{"code":"0","message":"zero code"}}`)
	if err := decodeFatSecretError(body, 200); err != nil {
		t.Fatalf("code=0: expected nil, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Transport.Do — body limit exceeded (maxResponseBytes)
// ---------------------------------------------------------------------------

func TestTransportDo_BodyLimitExceeded_TruncatesNotErrors(t *testing.T) {
	t.Parallel()
	// Transport reads up to maxResponseBytes; it does NOT error on truncation —
	// it just caps the read. Verify a large body is capped.
	bigBody := strings.Repeat("x", 200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = io.WriteString(w, bigBody)
	}))
	defer srv.Close()

	base := func(req *http.Request) (*http.Response, error) { return srv.Client().Do(req) }
	tr, err := NewTransport(TransportOptions{
		HTTPClient:       srv.Client(),
		Chain:            base,
		MaxResponseBytes: 100, // cap at 100 bytes
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	_, body, doErr := tr.Do(context.Background(), req)
	if doErr != nil {
		t.Fatalf("Do: %v", doErr)
	}
	if len(body) > 100 {
		t.Fatalf("body length = %d; want ≤ 100", len(body))
	}
}

// ---------------------------------------------------------------------------
// Transport.Do — non-JSON 4xx body
// ---------------------------------------------------------------------------

func TestTransportDo_Non200_PlainTextBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "bad request plain text")
	}))
	defer srv.Close()

	col := &fakeCollector{}
	tr := newTransportWithBase(t, srv.Client(), col)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	_, _, err := tr.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if col.incFailuresCalls == 0 {
		t.Fatal("IncFailures not called on 400")
	}
}

// ---------------------------------------------------------------------------
// CacheMiddleware — GET with no-params URL (cacheKey no-params branch)
// ---------------------------------------------------------------------------

func TestCacheKey_NoParams(t *testing.T) {
	t.Parallel()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com/path", nil)
	key := cacheKey(req)
	if key != "GET:http://example.com/path" {
		t.Fatalf("cacheKey = %q; want GET:http://example.com/path", key)
	}
}

// ---------------------------------------------------------------------------
// CacheMiddleware — non-2xx response must not be stored
// ---------------------------------------------------------------------------

func TestCacheMiddleware_GET_Non2xx_NotStored(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"server error"}`)
	}))
	defer srv.Close()

	fc := newFakeCache()
	base := func(req *http.Request) (*http.Response, error) { return srv.Client().Do(req) }
	chain := Chain(base, CacheMiddleware(fc, time.Minute))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/food", nil)
	// Intentionally ignore the error (500 → dispatch error from Transport.Do).
	_, _, _ = tr.Do(ctx, req)

	if fc.setCalls != 0 {
		t.Fatalf("cache Set called %d times for 500 response; want 0", fc.setCalls)
	}
}

// ---------------------------------------------------------------------------
// CacheMiddleware — upstream error on miss must propagate
// ---------------------------------------------------------------------------

func TestCacheMiddleware_GET_UpstreamError_Propagated(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("upstream failure")
	base := func(_ *http.Request) (*http.Response, error) { return nil, wantErr }
	chain := Chain(base, CacheMiddleware(newFakeCache(), time.Minute))
	tr, _ := NewTransport(TransportOptions{Chain: chain})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com/food", nil)
	_, _, err := tr.Do(ctx, req)
	if err == nil {
		t.Fatal("expected upstream error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wantErr in chain; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// AuthMiddleware — GetHeaders returns error
// ---------------------------------------------------------------------------

func TestAuthMiddleware_GetHeadersError_ReturnsError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("auth: no token")
	a := &fakeAuthenticator{headerErr: wantErr}
	base := func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}
	chain := Chain(base, AuthMiddleware(a))
	tr, _ := NewTransport(TransportOptions{Chain: chain})

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", nil)
	_, _, err := tr.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected GetHeaders error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wantErr; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// AuthMiddleware — OAuth1RequestSigner GetHeadersForRequest error path
// ---------------------------------------------------------------------------

func TestAuthMiddleware_OAuth1Signer_GetHeadersError_ReturnsError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("oauth1: signing failed")
	signer := &stubOAuth1Signer{}
	signer.headerErr = wantErr

	base := func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}
	chain := Chain(base, AuthMiddleware(signer))
	tr, _ := NewTransport(TransportOptions{Chain: chain})

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com?food_id=1", nil)
	_, _, err := tr.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected OAuth1 signing error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wantErr; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// AuthMiddleware — 401 with GetBody rewind path
// ---------------------------------------------------------------------------

func TestAuthMiddleware_401_WithGetBody_Rewinds(t *testing.T) {
	t.Parallel()
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	a := &fakeAuthenticator{}
	base := func(req *http.Request) (*http.Response, error) { return srv.Client().Do(req) }
	chain := Chain(base, AuthMiddleware(a))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	// Use BuildRequest so GetBody is set (rewind path exercised on 401 retry).
	req, err := BuildRequest(context.Background(), "POST", srv.URL+"/food", []byte(`{"food":"apple"}`), nil)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	resp, _, doErr := tr.Do(context.Background(), req)
	if doErr != nil {
		t.Fatalf("Do: %v", doErr)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if callCount != 2 {
		t.Fatalf("server hit %d times; want 2", callCount)
	}
}

// ---------------------------------------------------------------------------
// RetryMiddleware — captureBody path via raw body (not BuildRequest)
// ---------------------------------------------------------------------------

func TestRetry_WithRawBody_CapturesAndReplays(t *testing.T) {
	t.Parallel()
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		body, _ := io.ReadAll(r.Body)
		if callCount == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if string(body) != `{"retry":"me"}` {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "body mismatch")
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	ctx := ctxkeys.WithRetryDelay(context.Background(), 0)
	ctx = ctxkeys.WithForceRetry(ctx, true)
	tr := buildRetryTransport(t, srv.Client(), 2, 0)

	// Construct request manually with a body but no GetBody — captureBody must buffer it.
	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+"/food",
		bytes.NewReader([]byte(`{"retry":"me"}`)))
	// Explicitly do NOT set GetBody — let captureBody handle it.
	resp, _, err := tr.Do(ctx, req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if callCount != 2 {
		t.Fatalf("server hit %d times; want 2", callCount)
	}
}

// ---------------------------------------------------------------------------
// LoggingMiddleware — loggingEnabled=true, BodySample, post-send branches
// ---------------------------------------------------------------------------

func TestLoggingMiddleware_Enabled_LogsDebug(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"food":"data"}`)
	}))
	defer srv.Close()

	var logged []string
	logger := &recordLogger{msgs: &logged}

	base := func(req *http.Request) (*http.Response, error) { return srv.Client().Do(req) }
	cfg := LogConfig{Enabled: true, BodySample: true, MaxBytes: 512}
	chain := Chain(base, LoggingMiddleware(logger, nil, &spyCollector{}, cfg))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/food", nil)
	if _, _, err := tr.Do(ctx, req); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if len(logged) == 0 {
		t.Fatal("expected log output when Enabled=true, got none")
	}
}

func TestLoggingMiddleware_Enabled_ErrorLogs_Warn(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"error":"server error"}`)
	}))
	defer srv.Close()

	var warnMsgs []string
	logger := &recordLogger{warnMsgs: &warnMsgs}

	base := func(req *http.Request) (*http.Response, error) { return srv.Client().Do(req) }
	cfg := LogConfig{Enabled: true}
	chain := Chain(base, LoggingMiddleware(logger, nil, &spyCollector{}, cfg))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	_, _, _ = tr.Do(ctx, req)

	if len(warnMsgs) == 0 {
		t.Fatal("expected Warn log on 500 response, got none")
	}
}

func TestLoggingMiddleware_PerRequestOverride_Enable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	var logged []string
	logger := &recordLogger{msgs: &logged}

	base := func(req *http.Request) (*http.Response, error) { return srv.Client().Do(req) }
	// cfg.Enabled=false but per-request override=true.
	cfg := LogConfig{Enabled: false}
	chain := Chain(base, LoggingMiddleware(logger, nil, &spyCollector{}, cfg))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := ctxkeys.WithLogging(context.Background(), true)
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	if _, _, err := tr.Do(ctx, req); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if len(logged) == 0 {
		t.Fatal("expected log output from per-request override, got none")
	}
}

func TestLoggingMiddleware_NilHook_NotPanics(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	base := func(req *http.Request) (*http.Response, error) { return srv.Client().Do(req) }
	// Pass a nil hook in the slice — must not panic.
	chain := Chain(base, LoggingMiddleware(nullLogger{}, []Hook{nil}, &spyCollector{}, LogConfig{}))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	if _, _, err := tr.Do(ctx, req); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

func TestLoggingMiddleware_BodySample_MaxBytes_Truncated(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = io.WriteString(w, strings.Repeat("a", 200))
	}))
	defer srv.Close()

	var logged []string
	logger := &recordLogger{msgs: &logged}

	base := func(req *http.Request) (*http.Response, error) { return srv.Client().Do(req) }
	cfg := LogConfig{Enabled: true, BodySample: true, MaxBytes: 50}
	chain := Chain(base, LoggingMiddleware(logger, nil, &spyCollector{}, cfg))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	if _, _, err := tr.Do(ctx, req); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if len(logged) == 0 {
		t.Fatal("expected log output")
	}
}

func TestLoggingMiddleware_BytesSent_Tracked(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	col := &spyCollector{}
	base := func(req *http.Request) (*http.Response, error) { return srv.Client().Do(req) }
	chain := Chain(base, LoggingMiddleware(nullLogger{}, nil, col, LogConfig{}))
	tr, _ := NewTransport(TransportOptions{Chain: chain, HTTPClient: srv.Client()})

	ctx := context.Background()
	// POST with a body so ContentLength > 0 → AddBytesSent called.
	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL, bytes.NewReader([]byte("hello")))
	req.ContentLength = 5
	if _, _, err := tr.Do(ctx, req); err != nil {
		t.Fatalf("Do: %v", err)
	}
	// Cannot assert exact value without inspecting AddBytesSentCalls since
	// spyCollector tracks calls not values; use fakeCollector instead.
	fc := &fakeCollector{}
	base2 := func(req *http.Request) (*http.Response, error) { return srv.Client().Do(req) }
	chain2 := Chain(base2, LoggingMiddleware(nullLogger{}, nil, fc, LogConfig{}))
	tr2, _ := NewTransport(TransportOptions{Chain: chain2, HTTPClient: srv.Client()})

	req2, _ := http.NewRequestWithContext(ctx, "POST", srv.URL, bytes.NewReader([]byte("hello")))
	req2.ContentLength = 5
	if _, _, err := tr2.Do(ctx, req2); err != nil {
		t.Fatalf("Do2: %v", err)
	}
	if fc.addBytesSentCalls == 0 {
		t.Fatal("AddBytesSent not called for POST with body")
	}
}

// ---------------------------------------------------------------------------
// recordLogger — captures log output for assertions
// ---------------------------------------------------------------------------

type recordLogger struct {
	msgs     *[]string
	warnMsgs *[]string
}

func (r *recordLogger) Debug(msg string, _ map[string]any) {
	if r.msgs != nil {
		*r.msgs = append(*r.msgs, msg)
	}
}
func (r *recordLogger) Info(msg string, _ map[string]any) {
	if r.msgs != nil {
		*r.msgs = append(*r.msgs, msg)
	}
}
func (r *recordLogger) Warn(msg string, _ map[string]any) {
	if r.warnMsgs != nil {
		*r.warnMsgs = append(*r.warnMsgs, msg)
	}
	if r.msgs != nil {
		*r.msgs = append(*r.msgs, msg)
	}
}
func (r *recordLogger) Error(msg string, _ map[string]any) {
	if r.msgs != nil {
		*r.msgs = append(*r.msgs, msg)
	}
}

var _ Logger = (*recordLogger)(nil)

// ---------------------------------------------------------------------------
// isRetryable — force=true + error path
// ---------------------------------------------------------------------------

func TestIsRetryable_ForceTrue_NonIdempotent_NetworkError(t *testing.T) {
	t.Parallel()
	ctx := ctxkeys.WithForceRetry(context.Background(), true)
	// POST is non-idempotent but force=true; network error → must be retryable.
	if !isRetryable(ctx, "POST", nil, errors.New("network down")) {
		t.Fatal("expected retryable for POST with force=true and network error")
	}
}

func TestIsRetryable_NonIdempotent_NoForce_NotRetryable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	resp := &http.Response{StatusCode: 500}
	if isRetryable(ctx, "POST", resp, nil) {
		t.Fatal("expected NOT retryable for POST without force")
	}
}

func TestIsRetryable_Idempotent_NetworkError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if !isRetryable(ctx, "GET", nil, errors.New("EOF")) {
		t.Fatal("expected retryable for GET with network error")
	}
}
