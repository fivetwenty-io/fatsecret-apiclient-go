package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pkgerrors "github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/errors"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/metrics"
)

// ---------------------------------------------------------------------------
// fakeCollector — records counts for assertion
// ---------------------------------------------------------------------------

type fakeCollector struct {
	incRequestsCalls  int
	incFailuresCalls  int
	observeDurCalls   int
	incActiveCalls    int
	decActiveCalls    int
	addBytesSentCalls int
	addBytesRecvCalls int
}

func (f *fakeCollector) IncRequests(_, _ string)        { f.incRequestsCalls++ }
func (f *fakeCollector) IncFailures(_, _ string, _ int) { f.incFailuresCalls++ }
func (f *fakeCollector) ObserveDuration(_, _ string, _ time.Duration) {
	f.observeDurCalls++
}
func (f *fakeCollector) IncActiveConnections()      { f.incActiveCalls++ }
func (f *fakeCollector) DecActiveConnections()      { f.decActiveCalls++ }
func (f *fakeCollector) AddBytesSent(_ int64)       { f.addBytesSentCalls++ }
func (f *fakeCollector) AddBytesRecv(_ int64)       { f.addBytesRecvCalls++ }
func (f *fakeCollector) Snapshot() metrics.Snapshot { return metrics.Snapshot{} }

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTransportWithBase builds a Transport whose chain is a bare RoundTripFunc
// that delegates to the given *http.Client (no middleware).
func newTransportWithBase(t *testing.T, hc *http.Client, col metrics.Collector) *Transport {
	t.Helper()
	base := func(req *http.Request) (*http.Response, error) {
		return hc.Do(req)
	}
	tr, err := NewTransport(TransportOptions{
		HTTPClient: hc,
		Chain:      base,
		Collector:  col,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	return tr
}

func fatSecretBody(code, msg string) []byte {
	b, _ := json.Marshal(map[string]any{
		"error": map[string]string{"code": code, "message": msg},
	})
	return b
}

// ---------------------------------------------------------------------------
// BuildRequest tests
// ---------------------------------------------------------------------------

func TestBuildRequest_NilContext(t *testing.T) {
	t.Parallel()
	//nolint:staticcheck // SA1012: intentional nil-context rejection test
	_, err := BuildRequest(nil, "GET", "http://example.com", nil, nil) //lint:ignore SA1012 intentional nil-context rejection test
	if err == nil {
		t.Fatal("expected error for nil context")
	}
	want := "http: nil context is not allowed"
	if err.Error() != want {
		t.Fatalf("error = %q; want %q", err.Error(), want)
	}
}

func TestBuildRequest_EmptyMethod(t *testing.T) {
	t.Parallel()
	_, err := BuildRequest(context.Background(), "", "http://example.com", nil, nil)
	if err == nil {
		t.Fatal("expected error for empty method")
	}
}

func TestBuildRequest_EmptyURL(t *testing.T) {
	t.Parallel()
	_, err := BuildRequest(context.Background(), "GET", "", nil, nil)
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestBuildRequest_WithBody_SetsGetBody(t *testing.T) {
	t.Parallel()
	body := []byte(`{"food":"apple"}`)
	req, err := BuildRequest(context.Background(), "POST", "http://example.com", body, nil)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if req.GetBody == nil {
		t.Fatal("GetBody must be set when body provided")
	}
	if req.ContentLength != int64(len(body)) {
		t.Fatalf("ContentLength = %d; want %d", req.ContentLength, len(body))
	}
	// verify GetBody returns independent reader
	r1, _ := req.GetBody()
	b1, _ := io.ReadAll(r1)
	r2, _ := req.GetBody()
	b2, _ := io.ReadAll(r2)
	if string(b1) != string(body) || string(b2) != string(body) {
		t.Fatalf("GetBody mismatch: got %q / %q", b1, b2)
	}
}

func TestBuildRequest_Headers(t *testing.T) {
	t.Parallel()
	req, err := BuildRequest(context.Background(), "GET", "http://example.com", nil, map[string]string{
		"X-Custom": "hello",
	})
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got := req.Header.Get("X-Custom"); got != "hello" {
		t.Fatalf("X-Custom = %q; want %q", got, "hello")
	}
}

// ---------------------------------------------------------------------------
// NewTransport tests
// ---------------------------------------------------------------------------

func TestNewTransport_NilChain(t *testing.T) {
	t.Parallel()
	_, err := NewTransport(TransportOptions{})
	if err == nil {
		t.Fatal("expected error for nil Chain")
	}
}

func TestNewTransport_DefaultMaxResponseBytes(t *testing.T) {
	t.Parallel()
	base := func(_ *http.Request) (*http.Response, error) { return nil, io.EOF }
	tr, err := NewTransport(TransportOptions{Chain: base})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	if tr.maxResponseBytes != DefaultMaxResponseBytes {
		t.Fatalf("maxResponseBytes = %d; want %d", tr.maxResponseBytes, DefaultMaxResponseBytes)
	}
}

// ---------------------------------------------------------------------------
// Transport.Do tests
// ---------------------------------------------------------------------------

func TestTransportDo_NilContext(t *testing.T) {
	t.Parallel()
	base := func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}
	tr, _ := NewTransport(TransportOptions{Chain: base})
	req, _ := http.NewRequest("GET", "http://example.com", nil)

	//nolint:staticcheck // SA1012: intentional nil-context rejection test
	_, _, err := tr.Do(nil, req) //lint:ignore SA1012 intentional nil-context rejection test
	if err == nil {
		t.Fatal("expected error for nil context")
	}
	want := "client: nil context is not allowed"
	if err.Error() != want {
		t.Fatalf("error = %q; want %q", err.Error(), want)
	}
}

func TestTransportDo_200OK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"food_id":"123"}`))
	}))
	defer srv.Close()

	col := &fakeCollector{}
	tr := newTransportWithBase(t, srv.Client(), col)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/food", nil)
	resp, body, err := tr.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if string(body) != `{"food_id":"123"}` {
		t.Fatalf("body = %q; want %q", body, `{"food_id":"123"}`)
	}
}

func TestTransportDo_FatSecretError_Code11_RateLimited(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fatSecretBody("11", "Application request limit reached"))
	}))
	defer srv.Close()

	col := &fakeCollector{}
	tr := newTransportWithBase(t, srv.Client(), col)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	_, _, err := tr.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, pkgerrors.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited; got %v", err)
	}
	var rle *pkgerrors.RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("expected *RateLimitError; got %T", err)
	}
	if col.incFailuresCalls == 0 {
		t.Fatal("IncFailures not called on rate limit error")
	}
}

func TestTransportDo_FatSecretError_Code13_AuthenticationError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fatSecretBody("13", "Invalid access token"))
	}))
	defer srv.Close()

	tr := newTransportWithBase(t, srv.Client(), &fakeCollector{})
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	_, _, err := tr.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, pkgerrors.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized; got %v", err)
	}
	var ae *pkgerrors.AuthenticationError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AuthenticationError; got %T", err)
	}
}

func TestTransportDo_FatSecretError_Code14_PermissionError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fatSecretBody("14", "Missing scope: premier"))
	}))
	defer srv.Close()

	tr := newTransportWithBase(t, srv.Client(), &fakeCollector{})
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	_, _, err := tr.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, pkgerrors.ErrForbidden) {
		t.Fatalf("expected ErrForbidden; got %v", err)
	}
	var pe *pkgerrors.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PermissionError; got %T", err)
	}
}

func TestTransportDo_Non200_NoFatSecretBody_DispatchByStatus_404(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	col := &fakeCollector{}
	tr := newTransportWithBase(t, srv.Client(), col)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/missing", nil)
	_, _, err := tr.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !errors.Is(err, pkgerrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound; got %v", err)
	}
	if col.incFailuresCalls == 0 {
		t.Fatal("IncFailures not called on 404")
	}
}

func TestTransportDo_TransportError_IOEof(t *testing.T) {
	t.Parallel()
	// RoundTripper returns io.EOF to simulate server closing connection.
	errRT := &errorRoundTripper{err: fmt.Errorf("wrap: %w", io.EOF)}
	hc := &http.Client{Transport: errRT}
	col := &fakeCollector{}
	tr := newTransportWithBase(t, hc, col)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://127.0.0.1:1/fail", nil)
	_, _, err := tr.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected transport error")
	}
	if col.incFailuresCalls == 0 {
		t.Fatal("IncFailures not called on transport error")
	}
}

// errorRoundTripper implements http.RoundTripper and always returns an error.
type errorRoundTripper struct {
	err error
}

func (e *errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, e.err
}
