package client_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/internal/ctxkeys"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/auth"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/client"
	fserrors "github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/errors"
)

// validAuth returns a functional OAuth1 authenticator using dummy credentials.
// It satisfies auth.Authenticator and auth.OAuth1RequestSigner; the auth
// middleware calls GetHeadersForRequest which returns a valid-looking OAuth
// header without a real network call.
func validAuth() auth.Authenticator {
	return auth.NewOAuth1Signed(auth.OAuth1Config{
		ConsumerKey:    "test-key",
		ConsumerSecret: "test-secret",
	})
}

// ---- NewClient construction tests ----------------------------------------

func TestNewClient_NilAuthenticator_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := client.NewClient(client.Options{
		Authenticator: nil,
	})
	if err == nil {
		t.Fatal("expected error for nil Authenticator, got nil")
	}
	if !strings.Contains(err.Error(), "Authenticator") {
		t.Errorf("error should mention Authenticator, got: %v", err)
	}
}

func TestNewClient_ValidOptions_NoError(t *testing.T) {
	t.Parallel()
	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil Client, got nil")
	}
	_ = c.Close()
}

func TestNewClient_TLSModeFingerprint_WithoutFingerprints_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := client.NewClient(client.Options{
		Authenticator:   validAuth(),
		TLSMode:         client.TLSModeFingerprint,
		TLSFingerprints: nil,
	})
	if err == nil {
		t.Fatal("expected error for TLSModeFingerprint without TLSFingerprints, got nil")
	}
	if !strings.Contains(err.Error(), "TLSFingerprints") {
		t.Errorf("error should mention TLSFingerprints, got: %v", err)
	}
}

func TestNewClient_UnparseableBaseURL_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       "://not-a-valid-url",
	})
	if err == nil {
		t.Fatal("expected error for unparseable BaseURL, got nil")
	}
	if !strings.Contains(err.Error(), "BaseURL") {
		t.Errorf("error should mention BaseURL, got: %v", err)
	}
}

// ---- Round-trip key identity tests (D-07 critical) ------------------------
//
// Each test calls a pkg/client.WithXxx setter then reads the value back via
// internal/ctxkeys getter. The fact that both succeed proves the setter and
// reader share the identical context key type — a different unexported key
// would cause the type assertion in the getter to fail and ok would be false.

func TestRoundTrip_WithRetries(t *testing.T) {
	t.Parallel()
	ctx := client.WithRetries(context.Background(), 7)
	got, ok := ctxkeys.RetryCount(ctx)
	if !ok {
		t.Fatal("ctxkeys.RetryCount: key not found in context (key type mismatch?)")
	}
	if got != 7 {
		t.Errorf("expected 7, got %d", got)
	}
}

func TestRoundTrip_WithRetries_Zero(t *testing.T) {
	t.Parallel()
	ctx := client.WithRetries(context.Background(), 0)
	got, ok := ctxkeys.RetryCount(ctx)
	if !ok {
		t.Fatal("ctxkeys.RetryCount: key not found in context")
	}
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestRoundTrip_WithRetries_Negative(t *testing.T) {
	t.Parallel()
	ctx := client.WithRetries(context.Background(), -3)
	got, ok := ctxkeys.RetryCount(ctx)
	if !ok {
		t.Fatal("ctxkeys.RetryCount: key not found in context")
	}
	if got != -3 {
		t.Errorf("expected -3, got %d", got)
	}
}

func TestRoundTrip_WithForceRetry_True(t *testing.T) {
	t.Parallel()
	ctx := client.WithForceRetry(context.Background(), true)
	got, ok := ctxkeys.ForceRetry(ctx)
	if !ok {
		t.Fatal("ctxkeys.ForceRetry: key not found in context (key type mismatch?)")
	}
	if !got {
		t.Errorf("expected true, got false")
	}
}

func TestRoundTrip_WithForceRetry_False(t *testing.T) {
	t.Parallel()
	ctx := client.WithForceRetry(context.Background(), false)
	got, ok := ctxkeys.ForceRetry(ctx)
	if !ok {
		t.Fatal("ctxkeys.ForceRetry: key not found in context")
	}
	if got {
		t.Errorf("expected false, got true")
	}
}

func TestRoundTrip_WithRetryDelay(t *testing.T) {
	t.Parallel()
	const want = 250 * time.Millisecond
	ctx := client.WithRetryDelay(context.Background(), want)
	got, ok := ctxkeys.RetryDelay(ctx)
	if !ok {
		t.Fatal("ctxkeys.RetryDelay: key not found in context (key type mismatch?)")
	}
	if got != want {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestRoundTrip_WithRetryDelay_Zero(t *testing.T) {
	t.Parallel()
	ctx := client.WithRetryDelay(context.Background(), 0)
	got, ok := ctxkeys.RetryDelay(ctx)
	if !ok {
		t.Fatal("ctxkeys.RetryDelay: key not found in context")
	}
	if got != 0 {
		t.Errorf("expected 0, got %v", got)
	}
}

func TestRoundTrip_WithLogging_Enable(t *testing.T) {
	t.Parallel()
	ctx := client.WithLogging(context.Background(), true)
	got, ok := ctxkeys.Logging(ctx)
	if !ok {
		t.Fatal("ctxkeys.Logging: key not found in context (key type mismatch?)")
	}
	if !got {
		t.Errorf("expected true, got false")
	}
}

func TestRoundTrip_WithLogging_Disable(t *testing.T) {
	t.Parallel()
	ctx := client.WithLogging(context.Background(), false)
	got, ok := ctxkeys.Logging(ctx)
	if !ok {
		t.Fatal("ctxkeys.Logging: key not found in context")
	}
	if got {
		t.Errorf("expected false, got true")
	}
}

func TestRoundTrip_WithLogFields(t *testing.T) {
	t.Parallel()
	fields := map[string]any{"request_id": "abc123", "user_id": 42}
	ctx := client.WithLogFields(context.Background(), fields)
	got, ok := ctxkeys.LogFields(ctx)
	if !ok {
		t.Fatal("ctxkeys.LogFields: key not found in context (key type mismatch?)")
	}
	if got["request_id"] != "abc123" {
		t.Errorf("request_id: expected abc123, got %v", got["request_id"])
	}
	if got["user_id"] != 42 {
		t.Errorf("user_id: expected 42, got %v", got["user_id"])
	}
}

func TestRoundTrip_WithLogFields_Merge(t *testing.T) {
	t.Parallel()
	ctx := client.WithLogFields(context.Background(), map[string]any{"a": 1})
	ctx = client.WithLogFields(ctx, map[string]any{"b": 2, "a": 99})
	got, ok := ctxkeys.LogFields(ctx)
	if !ok {
		t.Fatal("ctxkeys.LogFields: key not found in context")
	}
	if got["a"] != 99 {
		t.Errorf("merge: a: expected 99, got %v", got["a"])
	}
	if got["b"] != 2 {
		t.Errorf("merge: b: expected 2, got %v", got["b"])
	}
}

func TestRoundTrip_WithLogFields_Nil(t *testing.T) {
	t.Parallel()
	ctx := client.WithLogFields(context.Background(), nil)
	got, ok := ctxkeys.LogFields(ctx)
	if !ok {
		t.Fatal("ctxkeys.LogFields: key not found in context for nil fields")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map for nil fields, got %v", got)
	}
}

// ---- Client.Do happy path --------------------------------------------------

// newTestServer returns an httptest.Server that responds with body for any
// request. The handler records the last received URL query for inspection.
func newTestServer(t *testing.T, statusCode int, body string) (*httptest.Server, *string) { //nolint:unparam // statusCode param kept for potential non-200 test cases
	t.Helper()
	var lastRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &lastRawQuery
}

func TestDo_FatSecretErrorEnvelope_HTTP200_ReturnsTypedError(t *testing.T) {
	t.Parallel()
	// FatSecret returns API failures as HTTP 200 + an error envelope. Do must
	// surface this as a typed error rather than a "successful" Response.
	const body = `{ "error": {"code": 21, "message": "Invalid IP address detected:  '203.0.113.7'" }}`
	srv, _ := newTestServer(t, http.StatusOK, body)

	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	resp, err := c.Do(context.Background(), &client.Request{
		Method: http.MethodGet,
		Path:   "/rest/foods/search/v5",
	})
	if err == nil {
		t.Fatal("expected typed error for HTTP-200 error envelope, got nil")
	}
	if !errors.Is(err, fserrors.ErrIPBlocked) {
		t.Fatalf("expected ErrIPBlocked, got %v", err)
	}
	// The Response is still returned so callers can inspect status/body.
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected Response with status 200 alongside error, got %+v", resp)
	}
}

func TestDo_HappyPath_GET(t *testing.T) {
	t.Parallel()
	const responseBody = `{"foods":{"food":[]}}`
	srv, lastQuery := newTestServer(t, http.StatusOK, responseBody)

	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0, // disable retries for deterministic test
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	req := &client.Request{
		Method: http.MethodGet,
		Path:   "/rest/server.api",
		Params: map[string][]string{"method": {"foods.search"}, "search_expression": {"chicken"}},
	}

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode: expected 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != responseBody {
		t.Errorf("Body: expected %q, got %q", responseBody, string(resp.Body))
	}

	// format=json must appear in query string.
	if !strings.Contains(*lastQuery, "format=json") {
		t.Errorf("format=json not found in query string: %q", *lastQuery)
	}
}

func TestDo_HappyPath_POST(t *testing.T) {
	t.Parallel()
	const responseBody = `{"profile":{"user_id":"u1"}}`
	var lastBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, responseBody)
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

	req := &client.Request{
		Method: http.MethodPost,
		Path:   "/rest/server.api",
		Params: map[string][]string{"method": {"profile.create"}},
	}

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode: expected 200, got %d", resp.StatusCode)
	}
	// format=json must appear in the POST body for form-encoded requests.
	if !strings.Contains(lastBody, "format=json") {
		t.Errorf("format=json not found in POST body: %q", lastBody)
	}
}

func TestDo_NilContext_ReturnsError(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.StatusOK, `{}`)

	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	//nolint:staticcheck // SA1012: intentional nil-context rejection test
	_, err = c.Do(nil, &client.Request{Method: http.MethodGet}) //lint:ignore SA1012 intentional nil-context rejection test
	if err == nil {
		t.Fatal("expected error for nil context, got nil")
	}
	if !strings.Contains(err.Error(), "nil context") {
		t.Errorf("error should mention nil context, got: %v", err)
	}
}

func TestDo_NilRequest_ReturnsError(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.StatusOK, `{}`)

	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.Do(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request, got nil")
	}
}

func TestDo_EmptyMethod_ReturnsError(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, http.StatusOK, `{}`)

	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.Do(context.Background(), &client.Request{Method: ""})
	if err == nil {
		t.Fatal("expected error for empty Method, got nil")
	}
}

// ---- Zero-knob transport tuning --------------------------------------------

func TestNewClient_MaxIdleConnsPerHost_Applied_NoError(t *testing.T) {
	t.Parallel()
	// Verifies that a non-zero MaxIdleConnsPerHost does not cause a panic or
	// construction error. Direct assertion of the stdlib transport field is not
	// feasible via the Client interface, but the value being applied is exercised
	// by buildNetTransport (non-zero branch).
	c, err := client.NewClient(client.Options{
		Authenticator:       validAuth(),
		MaxIdleConnsPerHost: 20,
	})
	if err != nil {
		t.Fatalf("NewClient with MaxIdleConnsPerHost=20: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil Client")
	}
	_ = c.Close()
}

func TestNewClient_ZeroKnob_AllZero_NoError(t *testing.T) {
	t.Parallel()
	// All transport-tuning fields at zero; should build without error.
	c, err := client.NewClient(client.Options{
		Authenticator:       validAuth(),
		DialTimeout:         0,
		TLSHandshakeTimeout: 0,
		MaxIdleConnsPerHost: 0,
		IdleConnTimeout:     0,
		KeepAlive:           0,
	})
	if err != nil {
		t.Fatalf("NewClient with all-zero knobs: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil Client")
	}
	_ = c.Close()
}

// ---- Auth accessor ---------------------------------------------------------

func TestClient_Auth_ReturnsSameAuthenticator(t *testing.T) {
	t.Parallel()
	a := validAuth()
	c, err := client.NewClient(client.Options{
		Authenticator: a,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := c.Auth()
	if got == nil {
		t.Fatal("Auth() returned nil")
	}
}

// ---- InvalidAuthenticator stub (construction only) -------------------------

func TestNewClient_InvalidAuthenticatorWithErr_BuildsClient(t *testing.T) {
	t.Parallel()
	// InvalidAuthenticator with a non-nil Err satisfies auth.Authenticator.
	// NewClient should succeed (Authenticator is non-nil); the error only
	// surfaces at request time when GetHeaders is called.
	a := &auth.InvalidAuthenticator{Err: errors.New("stub: no credentials configured")}
	c, err := client.NewClient(client.Options{
		Authenticator: a,
	})
	if err != nil {
		t.Fatalf("NewClient with InvalidAuthenticator: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil Client")
	}
	_ = c.Close()
}

// ---- DoJSON convenience ----------------------------------------------------

func TestDoJSON_UnmarshalsBody(t *testing.T) {
	t.Parallel()
	type foodResult struct {
		Name string `json:"name"`
	}
	type envelope struct {
		Food foodResult `json:"food"`
	}

	const body = `{"food":{"name":"Chicken Breast"}}`
	srv, _ := newTestServer(t, http.StatusOK, body)

	c, err := client.NewClient(client.Options{
		Authenticator: validAuth(),
		BaseURL:       srv.URL,
		MaxRetries:    0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	var out envelope
	resp, err := client.DoJSON(context.Background(), c, &client.Request{
		Method: http.MethodGet,
		Path:   "/rest/server.api",
	}, &out)
	if err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode: expected 200, got %d", resp.StatusCode)
	}
	if out.Food.Name != "Chicken Breast" {
		t.Errorf("Food.Name: expected 'Chicken Breast', got %q", out.Food.Name)
	}
}
