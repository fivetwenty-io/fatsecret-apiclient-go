package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tokenServerHandler writes a well-formed token response, incrementing counter
// for each request received. If delay > 0 it sleeps before responding so
// concurrent callers pile up.
func tokenServerHandler(t *testing.T, token string, expiresIn int, delay time.Duration, counter *atomic.Int32) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if delay > 0 {
			time.Sleep(delay)
		}
		counter.Add(1)

		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/x-www-form-urlencoded") {
			t.Errorf("unexpected Content-Type: %s", ct)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parsing form: %v", err)
		}
		if r.FormValue("grant_type") != "client_credentials" {
			t.Errorf("expected grant_type=client_credentials, got %q", r.FormValue("grant_type"))
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"access_token": token,
			"expires_in":   expiresIn,
			"token_type":   "Bearer",
		}); err != nil {
			t.Errorf("encoding token response: %v", err)
		}
	}
}

func TestOAuth2ClientCredentials_GetHeaders_HappyPath(t *testing.T) {
	t.Parallel()

	const wantToken = "test-access-token-abc123"
	var counter atomic.Int32
	srv := httptest.NewServer(tokenServerHandler(t, wantToken, 3600, 0, &counter))
	t.Cleanup(srv.Close)

	auth := NewOAuth2ClientCredentials(OAuth2Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		TokenURL:     srv.URL,
	})

	ctx := context.Background()
	headers, err := auth.GetHeaders(ctx)
	if err != nil {
		t.Fatalf("GetHeaders returned error: %v", err)
	}

	authHeader, ok := headers["Authorization"]
	if !ok {
		t.Fatal("Authorization header missing from GetHeaders result")
	}
	want := "Bearer " + wantToken
	if authHeader != want {
		t.Errorf("Authorization header = %q; want %q", authHeader, want)
	}
	if n := counter.Load(); n != 1 {
		t.Errorf("token endpoint hit %d times; want 1", n)
	}
}

func TestOAuth2ClientCredentials_IsAuthenticated_TrueAfterRefresh(t *testing.T) {
	t.Parallel()

	var counter atomic.Int32
	srv := httptest.NewServer(tokenServerHandler(t, "tok", 3600, 0, &counter))
	t.Cleanup(srv.Close)

	auth := NewOAuth2ClientCredentials(OAuth2Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		TokenURL:     srv.URL,
	})

	ctx := context.Background()
	if auth.IsAuthenticated() {
		t.Fatal("IsAuthenticated should be false before first Authenticate")
	}

	if err := auth.Authenticate(ctx); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !auth.IsAuthenticated() {
		t.Fatal("IsAuthenticated should be true after Authenticate with long-lived token")
	}
}

func TestOAuth2ClientCredentials_ProactiveRefresh(t *testing.T) {
	t.Parallel()

	// Token expires in 1s; RefreshBefore=2s → window immediately covers the token.
	// IsAuthenticated must return false. GetHeaders must trigger a new fetch.
	var counter atomic.Int32
	srv := httptest.NewServer(tokenServerHandler(t, "refreshed-token", 1, 0, &counter))
	t.Cleanup(srv.Close)

	a := &OAuth2ClientCredentials{
		cfg: OAuth2Config{
			ClientID:      "cid",
			ClientSecret:  "csec",
			TokenURL:      srv.URL,
			RefreshBefore: 2 * time.Second,
			HTTPClient:    http.DefaultClient,
		},
	}

	// Prime with a short-lived token directly (bypass singleflight for setup).
	if err := a.Refresh(context.Background()); err != nil {
		t.Fatalf("initial Refresh: %v", err)
	}
	if n := counter.Load(); n != 1 {
		t.Fatalf("expected 1 token fetch after initial Refresh, got %d", n)
	}

	// expires_in=1s, RefreshBefore=2s → 1s < 2s → outside valid window → false.
	if a.IsAuthenticated() {
		t.Fatal("IsAuthenticated should be false when token expiry < RefreshBefore window")
	}

	// GetHeaders must detect needsRefresh and fetch again.
	headers, err := a.GetHeaders(context.Background())
	if err != nil {
		t.Fatalf("GetHeaders after proactive-refresh trigger: %v", err)
	}
	authHeader, ok := headers["Authorization"]
	if !ok {
		t.Fatal("Authorization header missing after proactive refresh")
	}
	if !strings.HasPrefix(authHeader, "Bearer ") {
		t.Errorf("Authorization header %q does not start with 'Bearer '", authHeader)
	}
	if n := counter.Load(); n < 2 {
		t.Errorf("expected ≥2 token fetches (initial + proactive refresh), got %d", n)
	}
}

func TestOAuth2ClientCredentials_Logout(t *testing.T) {
	t.Parallel()

	var counter atomic.Int32
	srv := httptest.NewServer(tokenServerHandler(t, "tok", 3600, 0, &counter))
	t.Cleanup(srv.Close)

	auth := NewOAuth2ClientCredentials(OAuth2Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		TokenURL:     srv.URL,
	})
	ctx := context.Background()

	if err := auth.Authenticate(ctx); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !auth.IsAuthenticated() {
		t.Fatal("should be authenticated after Authenticate")
	}

	auth.Logout()

	if auth.IsAuthenticated() {
		t.Fatal("IsAuthenticated should return false after Logout")
	}
}

// TestOAuth2ClientCredentials_ConcurrentRefresh_SingleFlight asserts that N
// concurrent Refresh calls result in exactly ONE token endpoint hit. This test
// must NOT run in parallel because it relies on a shared atomic counter.
func TestOAuth2ClientCredentials_ConcurrentRefresh_SingleFlight(t *testing.T) {
	const goroutines = 20

	var counter atomic.Int32
	// 50ms delay gives all goroutines time to stack up before the first completes.
	srv := httptest.NewServer(tokenServerHandler(t, "sf-token", 3600, 50*time.Millisecond, &counter))
	t.Cleanup(srv.Close)

	a := &OAuth2ClientCredentials{
		cfg: OAuth2Config{
			ClientID:     "cid",
			ClientSecret: "csec",
			TokenURL:     srv.URL,
			HTTPClient:   http.DefaultClient,
		},
	}
	// Apply defaults manually (RefreshBefore unused here, but must be nonzero
	// so IsAuthenticated works after the fetch).
	a.cfg.RefreshBefore = defaultRefreshBefore

	// Gate: all goroutines wait for the same start signal.
	var (
		ready sync.WaitGroup
		start = make(chan struct{})
		done  sync.WaitGroup
		errs  = make([]error, goroutines)
	)
	ready.Add(goroutines)
	done.Add(goroutines)

	ctx := context.Background()
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			ready.Done()
			<-start // wait for gate
			errs[i] = a.Refresh(ctx)
			done.Done()
		}()
	}

	ready.Wait() // all goroutines are staged
	close(start) // release all at once
	done.Wait()  // wait for all to finish

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Refresh returned error: %v", i, err)
		}
	}

	if n := counter.Load(); n != 1 {
		t.Errorf("token endpoint hit %d times; singleflight must deduplicate to exactly 1", n)
	}
}

func TestOAuth2ClientCredentials_Non200Response_ReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad credentials"}`))
	}))
	t.Cleanup(srv.Close)

	auth := NewOAuth2ClientCredentials(OAuth2Config{
		ClientID:     "bad-id",
		ClientSecret: "bad-secret",
		TokenURL:     srv.URL,
	})

	err := auth.Authenticate(context.Background())
	if err == nil {
		t.Fatal("expected error from non-200 token response, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q does not mention status 401", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid_client") {
		t.Errorf("error %q does not contain JSON error field 'invalid_client'", err.Error())
	}
}

func TestOAuth2ClientCredentials_Non200_PlainBody_ReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	t.Cleanup(srv.Close)

	auth := NewOAuth2ClientCredentials(OAuth2Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		TokenURL:     srv.URL,
	})

	err := auth.Authenticate(context.Background())
	if err == nil {
		t.Fatal("expected error from 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not mention status 500", err.Error())
	}
}

func TestOAuth2ClientCredentials_EmptyAccessToken_ReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"","expires_in":3600,"token_type":"Bearer"}`))
	}))
	t.Cleanup(srv.Close)

	auth := NewOAuth2ClientCredentials(OAuth2Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		TokenURL:     srv.URL,
	})

	err := auth.Authenticate(context.Background())
	if err == nil {
		t.Fatal("expected error from empty access_token, got nil")
	}
	if !strings.Contains(err.Error(), "empty access_token") {
		t.Errorf("error %q does not mention empty access_token", err.Error())
	}
}

func TestOAuth2ClientCredentials_InvalidExpiresIn_ReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":0,"token_type":"Bearer"}`))
	}))
	t.Cleanup(srv.Close)

	auth := NewOAuth2ClientCredentials(OAuth2Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		TokenURL:     srv.URL,
	})

	err := auth.Authenticate(context.Background())
	if err == nil {
		t.Fatal("expected error from expires_in=0, got nil")
	}
	if !strings.Contains(err.Error(), "invalid expires_in") {
		t.Errorf("error %q does not mention invalid expires_in", err.Error())
	}
}

func TestNewOAuth2ClientCredentials_MissingClientID_ReturnsInvalidAuthenticator(t *testing.T) {
	t.Parallel()

	a := NewOAuth2ClientCredentials(OAuth2Config{ClientSecret: "secret"})
	ia, ok := a.(*InvalidAuthenticator)
	if !ok {
		t.Fatalf("expected *InvalidAuthenticator, got %T", a)
	}
	if ia.Err == nil {
		t.Fatal("InvalidAuthenticator.Err must not be nil")
	}
	if !strings.Contains(ia.Err.Error(), "ClientID") {
		t.Errorf("error %q does not mention ClientID", ia.Err.Error())
	}
	if a.IsAuthenticated() {
		t.Error("IsAuthenticated must be false for InvalidAuthenticator")
	}
}

func TestNewOAuth2ClientCredentials_MissingClientSecret_ReturnsInvalidAuthenticator(t *testing.T) {
	t.Parallel()

	a := NewOAuth2ClientCredentials(OAuth2Config{ClientID: "id"})
	ia, ok := a.(*InvalidAuthenticator)
	if !ok {
		t.Fatalf("expected *InvalidAuthenticator, got %T", a)
	}
	if ia.Err == nil {
		t.Fatal("InvalidAuthenticator.Err must not be nil")
	}
	if !strings.Contains(ia.Err.Error(), "ClientSecret") {
		t.Errorf("error %q does not mention ClientSecret", ia.Err.Error())
	}
}

func TestOAuth2ClientCredentials_WithScopes(t *testing.T) {
	t.Parallel()

	var capturedScope string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		capturedScope = r.FormValue("scope")
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"access_token": "scoped-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		}); err != nil {
			t.Errorf("encoding response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	auth := NewOAuth2ClientCredentials(OAuth2Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		TokenURL:     srv.URL,
		Scopes:       []string{"basic", "premier"},
	})

	if err := auth.Authenticate(context.Background()); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if capturedScope != "basic premier" {
		t.Errorf("scope sent = %q; want %q", capturedScope, "basic premier")
	}
}

func TestOAuth2ClientCredentials_GetHeaders_AfterLogout_TriggersRefresh(t *testing.T) {
	t.Parallel()

	var counter atomic.Int32
	srv := httptest.NewServer(tokenServerHandler(t, "new-token", 3600, 0, &counter))
	t.Cleanup(srv.Close)

	auth := NewOAuth2ClientCredentials(OAuth2Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		TokenURL:     srv.URL,
	})
	ctx := context.Background()

	if _, err := auth.GetHeaders(ctx); err != nil {
		t.Fatalf("first GetHeaders: %v", err)
	}
	auth.Logout()

	headers, err := auth.GetHeaders(ctx)
	if err != nil {
		t.Fatalf("GetHeaders after Logout: %v", err)
	}
	authHeader := headers["Authorization"]
	if authHeader != "Bearer new-token" {
		t.Errorf("Authorization = %q; want %q", authHeader, "Bearer new-token")
	}
	if n := counter.Load(); n != 2 {
		t.Errorf("expected 2 token fetches (initial + post-logout), got %d", n)
	}
}

func TestOAuth2ClientCredentials_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Server delays longer than the client timeout so the client context
	// expires before the response arrives. The handler uses a select so it
	// unblocks promptly when either the test ends or the server closes.
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(func() {
		close(done)
		srv.Close()
	})

	auth := NewOAuth2ClientCredentials(OAuth2Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		TokenURL:     srv.URL,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := auth.Authenticate(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context") {
		t.Errorf("error %q is not context-related", err.Error())
	}
}
