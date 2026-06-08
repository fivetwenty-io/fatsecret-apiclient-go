package client_test

// coverage_test.go adds targeted tests for branches not exercised by
// client_test.go, raising pkg/client coverage to ≥ 82%.

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/client"
)

// ---------------------------------------------------------------------------
// slog adapter
// ---------------------------------------------------------------------------

func TestNewSlogLogger_Debug(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := client.NewSlogLogger(slog.New(h))
	l.Debug("test-debug", map[string]any{"k": "v"})
	if !strings.Contains(buf.String(), "test-debug") {
		t.Fatalf("Debug: expected message in output, got %q", buf.String())
	}
}

func TestNewSlogLogger_Info(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := client.NewSlogLogger(slog.New(h))
	l.Info("test-info", map[string]any{"foo": "bar"})
	if !strings.Contains(buf.String(), "test-info") {
		t.Fatalf("Info: expected message in output, got %q", buf.String())
	}
}

func TestNewSlogLogger_Warn(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := client.NewSlogLogger(slog.New(h))
	l.Warn("test-warn", nil)
	if !strings.Contains(buf.String(), "test-warn") {
		t.Fatalf("Warn: expected message in output, got %q", buf.String())
	}
}

func TestNewSlogLogger_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := client.NewSlogLogger(slog.New(h))
	l.Error("test-error", map[string]any{"err": "boom"})
	if !strings.Contains(buf.String(), "test-error") {
		t.Fatalf("Error: expected message in output, got %q", buf.String())
	}
}

func TestNewSlogLogger_EmptyFields(t *testing.T) {
	t.Parallel()
	// nil and empty maps must not panic (attrsFromFields early-return path).
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := client.NewSlogLogger(slog.New(h))
	l.Debug("no-fields", nil)
	l.Debug("empty-fields", map[string]any{})
}

// ---------------------------------------------------------------------------
// NewHook
// ---------------------------------------------------------------------------

func TestNewHook_WrapsFunction(t *testing.T) {
	t.Parallel()
	var fired int
	h := client.NewHook(func(_ client.HookEvent) {
		fired++
	})
	h(client.HookEvent{Method: "GET", StatusCode: 200})
	if fired != 1 {
		t.Fatalf("hook fired %d times; want 1", fired)
	}
}

// ---------------------------------------------------------------------------
// buildRequestParts — POST with explicit Body
// ---------------------------------------------------------------------------

func TestDo_POST_WithExplicitBody_ParamsInQueryString(t *testing.T) {
	t.Parallel()
	var gotQuery string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	rawBody := []byte(`{"custom":"payload"}`)
	req := &client.Request{
		Method: http.MethodPost,
		Path:   "/rest/server.api",
		Params: map[string][]string{"method": {"foods.add"}},
		Body:   rawBody,
	}

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: expected 200, got %d", resp.StatusCode)
	}
	// format=json and method param must appear in query string (not body).
	if !strings.Contains(gotQuery, "format=json") {
		t.Errorf("format=json not in query string: %q", gotQuery)
	}
	if !strings.Contains(gotQuery, "method=foods.add") {
		t.Errorf("method not in query string: %q", gotQuery)
	}
	// Body must be the raw bytes, not form-encoded.
	if gotBody != string(rawBody) {
		t.Errorf("body: got %q; want %q", gotBody, rawBody)
	}
}

// ---------------------------------------------------------------------------
// DoJSON — error paths
// ---------------------------------------------------------------------------

func TestDoJSON_UnmarshalError_ReturnsError(t *testing.T) {
	t.Parallel()
	// Server returns malformed JSON — unmarshal into struct must fail.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `not valid json {{{{`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	var out struct{ Name string }
	_, err = client.DoJSON(context.Background(), c, &client.Request{
		Method: http.MethodGet,
		Path:   "/",
	}, &out)
	if err == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("error should mention unmarshal, got: %v", err)
	}
}

func TestDoJSON_NilOut_SkipsUnmarshal(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"food":"data"}`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// nil out — must not panic, must return the response.
	resp, err := client.DoJSON(context.Background(), c, &client.Request{
		Method: http.MethodGet,
		Path:   "/",
	}, nil)
	if err != nil {
		t.Fatalf("DoJSON(nil out): %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestDoJSON_DoError_PropagatesError(t *testing.T) {
	t.Parallel()
	// FatSecret error envelope causes Do to return an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"error":{"code":"11","message":"rate limited"}}`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	var out struct{}
	_, err = client.DoJSON(context.Background(), c, &client.Request{
		Method: http.MethodGet,
		Path:   "/",
	}, &out)
	if err == nil {
		t.Fatal("expected error from FatSecret envelope, got nil")
	}
}

// ---------------------------------------------------------------------------
// validate — InsecureSkipVerify + Logger warn path
// ---------------------------------------------------------------------------

func TestNewClient_InsecureSkipVerify_LogsWarn(t *testing.T) {
	t.Parallel()
	// Use a slog-backed logger writing to a buffer so we can assert the warning.
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := client.NewSlogLogger(slog.New(h))

	c, err := client.NewClient(client.Options{
		Authenticator:      validAuth(),
		InsecureSkipVerify: true,
		Logger:             logger,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil Client")
	}
	_ = c.Close()
	// The warn message must appear in the slog buffer.
	if !strings.Contains(buf.String(), "InsecureSkipVerify") {
		t.Errorf("expected InsecureSkipVerify warning in log output, got: %q", buf.String())
	}
}

func TestNewClient_InsecureSkipVerify_TLSModeNoneApplied(t *testing.T) {
	t.Parallel()
	// Builds a real httptest TLS server and verifies InsecureSkipVerify lets
	// the client connect to a self-signed cert (TLSModeNone path).
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.NewClient(client.Options{
		Authenticator:      validAuth(),
		BaseURL:            srv.URL,
		InsecureSkipVerify: true,
		MaxRetries:         0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	resp, err := c.Do(context.Background(), &client.Request{
		Method: http.MethodGet,
		Path:   "/",
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: expected 200, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// buildNetTransport — all non-zero tuning branches
// ---------------------------------------------------------------------------

func TestNewClient_AllTransportKnobs_NonZero(t *testing.T) {
	t.Parallel()
	c, err := client.NewClient(client.Options{
		Authenticator:       validAuth(),
		DialTimeout:         2 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     60 * time.Second,
		KeepAlive:           30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient with all non-zero knobs: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil Client")
	}
	_ = c.Close()
}

// ---------------------------------------------------------------------------
// validate — TLSModeFull without fingerprints
// ---------------------------------------------------------------------------

func TestNewClient_TLSModeFull_WithoutFingerprints_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := client.NewClient(client.Options{
		Authenticator:   validAuth(),
		TLSMode:         client.TLSModeFull,
		TLSFingerprints: nil,
	})
	if err == nil {
		t.Fatal("expected error for TLSModeFull without TLSFingerprints, got nil")
	}
	if !strings.Contains(err.Error(), "TLSFingerprints") {
		t.Errorf("error should mention TLSFingerprints, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NewClient — with Cache (non-noop) to exercise CacheMiddleware branch
// ---------------------------------------------------------------------------

type inMemCache struct {
	data map[string][]byte
}

func newInMemCache() *inMemCache { return &inMemCache{data: make(map[string][]byte)} }

func (c *inMemCache) Get(key string) ([]byte, bool) {
	v, ok := c.data[key]
	return v, ok
}

func (c *inMemCache) Set(key string, value []byte, _ time.Duration) {
	c.data[key] = value
}

func (c *inMemCache) Delete(key string) { delete(c.data, key) }
func (c *inMemCache) Flush()            { c.data = make(map[string][]byte) }

func TestNewClient_WithCache_CachesGET(t *testing.T) {
	t.Parallel()
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"food_id":"1"}`)
	}))
	t.Cleanup(srv.Close)

	cache := newInMemCache()
	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0,
		Cache:         cache,
		CacheTTL:      time.Minute,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	for i := 0; i < 3; i++ {
		resp, err := c.Do(context.Background(), &client.Request{
			Method: http.MethodGet,
			Path:   "/food",
		})
		if err != nil {
			t.Fatalf("Do[%d]: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Do[%d]: status %d", i, resp.StatusCode)
		}
	}
	if hits != 1 {
		t.Errorf("origin hit %d times; want 1 (subsequent calls should be cached)", hits)
	}
}

// ---------------------------------------------------------------------------
// NewClient — with Logger and Hooks plumbing (exercises logging middleware)
// ---------------------------------------------------------------------------

func TestNewClient_WithLoggerAndHook_HookFired(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)

	var hooked int
	hook := client.NewHook(func(_ client.HookEvent) { hooked++ })

	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := client.NewSlogLogger(slog.New(h))

	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0,
		Logger:        logger,
		LogConfig:     client.LogConfig{Enabled: true, BodySample: true, MaxBytes: 512},
		Hooks:         []client.Hook{hook},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.Do(context.Background(), &client.Request{
		Method: http.MethodGet,
		Path:   "/",
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if hooked == 0 {
		t.Fatal("hook was never fired")
	}
}

// ---------------------------------------------------------------------------
// Close is idempotent
// ---------------------------------------------------------------------------

func TestClient_Close_Idempotent(t *testing.T) {
	t.Parallel()
	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
