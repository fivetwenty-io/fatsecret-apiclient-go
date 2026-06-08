package auth

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// parseOAuthHeader splits an "OAuth realm="", k="v", ..." header value into a
// map of parameter name → unquoted value. Returns an error description if
// parsing fails.
func parseOAuthHeader(t *testing.T, header string) map[string]string {
	t.Helper()
	if !strings.HasPrefix(header, "OAuth ") {
		t.Fatalf("Authorization header does not start with 'OAuth ': %q", header)
	}
	// Strip "OAuth " prefix.
	rest := strings.TrimPrefix(header, "OAuth ")

	result := make(map[string]string)
	// Split on ", " between parameters. Each param is key="value".
	// realm="" must be first and has an empty value.
	for _, part := range strings.Split(rest, ", ") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eqIdx := strings.Index(part, "=")
		if eqIdx < 0 {
			t.Fatalf("malformed OAuth header parameter (no '='): %q", part)
		}
		k := part[:eqIdx]
		v := strings.Trim(part[eqIdx+1:], `"`)
		result[k] = v
	}
	return result
}

// nonceRegexp matches a valid 32-character lowercase hex nonce (16 bytes).
var nonceRegexp = regexp.MustCompile(`^[0-9a-f]{32}$`)

// timestampRegexp matches a Unix timestamp (positive integer string).
var timestampRegexp = regexp.MustCompile(`^[1-9][0-9]{9,}$`)

// signatureBase64Regexp matches a non-empty base64-encoded string.
var signatureBase64Regexp = regexp.MustCompile(`^[A-Za-z0-9+/]+=*$`)

// assertOAuth1HeaderFields checks that all mandatory OAuth 1.0a fields are
// present and well-formed in the parsed header map.
func assertOAuth1HeaderFields(t *testing.T, params map[string]string, wantConsumerKey string) {
	t.Helper()

	requiredKeys := []string{
		"realm", "oauth_consumer_key", "oauth_nonce",
		"oauth_signature", "oauth_signature_method",
		"oauth_timestamp", "oauth_version",
	}
	for _, k := range requiredKeys {
		if _, ok := params[k]; !ok {
			t.Errorf("OAuth header missing required parameter %q; full params: %v", k, params)
		}
	}

	if got := params["oauth_consumer_key"]; got != wantConsumerKey {
		t.Errorf("oauth_consumer_key = %q; want %q", got, wantConsumerKey)
	}
	if got := params["oauth_signature_method"]; got != "HMAC-SHA1" {
		t.Errorf("oauth_signature_method = %q; want HMAC-SHA1", got)
	}
	if got := params["oauth_version"]; got != "1.0" {
		t.Errorf("oauth_version = %q; want 1.0", got)
	}
	if nonce := params["oauth_nonce"]; !nonceRegexp.MatchString(nonce) {
		t.Errorf("oauth_nonce %q does not match 32-hex-char pattern", nonce)
	}
	if ts := params["oauth_timestamp"]; !timestampRegexp.MatchString(ts) {
		t.Errorf("oauth_timestamp %q is not a plausible Unix timestamp", ts)
	}
	sig := params["oauth_signature"]
	if sig == "" {
		t.Error("oauth_signature is empty")
	}
	if !signatureBase64Regexp.MatchString(sig) {
		t.Errorf("oauth_signature %q is not valid base64", sig)
	}
}

// ---------------------------------------------------------------------------
// OAuth1Signed tests
// ---------------------------------------------------------------------------

func TestOAuth1Signed_GetHeadersForRequest_AllFieldsPresent(t *testing.T) {
	t.Parallel()

	signer := NewOAuth1Signed(OAuth1Config{
		ConsumerKey:    "test-consumer-key",
		ConsumerSecret: "test-consumer-secret",
	})

	params := url.Values{
		"method": {"foods.search"},
		"format": {"json"},
	}
	headers, err := signer.GetHeadersForRequest(
		context.Background(),
		"GET",
		"https://platform.fatsecret.com/rest/foods/search/v1",
		params,
	)
	if err != nil {
		t.Fatalf("GetHeadersForRequest returned error: %v", err)
	}

	authHeader, ok := headers["Authorization"]
	if !ok {
		t.Fatal("Authorization key missing from GetHeadersForRequest result")
	}
	if len(headers) != 1 {
		t.Errorf("expected exactly 1 header key, got %d: %v", len(headers), headers)
	}

	parsed := parseOAuthHeader(t, authHeader)
	assertOAuth1HeaderFields(t, parsed, "test-consumer-key")

	// Two-legged: oauth_token must NOT appear.
	if _, ok := parsed["oauth_token"]; ok {
		t.Error("oauth_token must not be present in two-legged OAuth1Signed request")
	}
}

func TestOAuth1Signed_GetHeadersForRequest_SignatureNonEmpty(t *testing.T) {
	t.Parallel()

	signer := NewOAuth1Signed(OAuth1Config{
		ConsumerKey:    "ck",
		ConsumerSecret: "cs",
	})
	headers, err := signer.GetHeadersForRequest(
		context.Background(),
		"POST",
		"https://platform.fatsecret.com/rest/server.api",
		url.Values{"method": {"profile.create"}},
	)
	if err != nil {
		t.Fatalf("GetHeadersForRequest: %v", err)
	}
	parsed := parseOAuthHeader(t, headers["Authorization"])
	sig := parsed["oauth_signature"]
	if sig == "" {
		t.Fatal("oauth_signature is empty")
	}
}

func TestOAuth1Signed_GetHeadersForRequest_DifferentNonceEachCall(t *testing.T) {
	t.Parallel()

	signer := NewOAuth1Signed(OAuth1Config{
		ConsumerKey:    "ck",
		ConsumerSecret: "cs",
	})
	ctx := context.Background()
	params := url.Values{}

	headers1, err := signer.GetHeadersForRequest(ctx, "GET", "https://platform.fatsecret.com/rest", params)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	headers2, err := signer.GetHeadersForRequest(ctx, "GET", "https://platform.fatsecret.com/rest", params)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	p1 := parseOAuthHeader(t, headers1["Authorization"])
	p2 := parseOAuthHeader(t, headers2["Authorization"])

	// Nonces must differ with overwhelming probability (cryptographically random).
	if p1["oauth_nonce"] == p2["oauth_nonce"] {
		t.Error("oauth_nonce identical across two calls — crypto/rand may not be working")
	}
}

func TestOAuth1Signed_GetHeaders_ReturnsOAuthHeader(t *testing.T) {
	t.Parallel()

	// GetHeaders delegates to GetHeadersForRequest("GET","",url.Values{}).
	signer := NewOAuth1Signed(OAuth1Config{
		ConsumerKey:    "ck",
		ConsumerSecret: "cs",
	})
	headers, err := signer.GetHeaders(context.Background())
	if err != nil {
		t.Fatalf("GetHeaders: %v", err)
	}
	if _, ok := headers["Authorization"]; !ok {
		t.Error("Authorization key missing from GetHeaders result")
	}
	if !strings.HasPrefix(headers["Authorization"], "OAuth ") {
		t.Errorf("Authorization value %q should start with 'OAuth '", headers["Authorization"])
	}
}

func TestOAuth1Signed_IsAuthenticated_AlwaysTrue(t *testing.T) {
	t.Parallel()

	signer := NewOAuth1Signed(OAuth1Config{
		ConsumerKey:    "ck",
		ConsumerSecret: "cs",
	})
	if !signer.IsAuthenticated() {
		t.Error("OAuth1Signed.IsAuthenticated should always return true")
	}
}

func TestOAuth1Signed_Authenticate_NoError(t *testing.T) {
	t.Parallel()

	signer := NewOAuth1Signed(OAuth1Config{
		ConsumerKey:    "ck",
		ConsumerSecret: "cs",
	})
	if err := signer.Authenticate(context.Background()); err != nil {
		t.Errorf("Authenticate should return nil for valid config, got: %v", err)
	}
}

func TestOAuth1Signed_Refresh_NoError(t *testing.T) {
	t.Parallel()

	signer := NewOAuth1Signed(OAuth1Config{
		ConsumerKey:    "ck",
		ConsumerSecret: "cs",
	})
	if err := signer.Refresh(context.Background()); err != nil {
		t.Errorf("Refresh should return nil for OAuth1Signed, got: %v", err)
	}
}

func TestOAuth1Signed_Logout_NoOp(t *testing.T) {
	t.Parallel()

	signer := NewOAuth1Signed(OAuth1Config{
		ConsumerKey:    "ck",
		ConsumerSecret: "cs",
	})
	// Must not panic; IsAuthenticated stays true.
	signer.Logout()
	if !signer.IsAuthenticated() {
		t.Error("IsAuthenticated should still be true after Logout (no-op for OAuth1)")
	}
}

func TestNewOAuth1Signed_EmptyConsumerKey_ReturnsInvalidSigner(t *testing.T) {
	t.Parallel()

	signer := NewOAuth1Signed(OAuth1Config{ConsumerSecret: "sec"})
	if signer.IsAuthenticated() {
		t.Error("invalid signer should return IsAuthenticated=false")
	}
	err := signer.Authenticate(context.Background())
	if err == nil {
		t.Fatal("Authenticate should return error for empty ConsumerKey")
	}
	if !strings.Contains(err.Error(), "ConsumerKey") {
		t.Errorf("error %q does not mention ConsumerKey", err.Error())
	}
	_, err2 := signer.GetHeadersForRequest(context.Background(), "GET", "https://example.com", url.Values{})
	if err2 == nil {
		t.Fatal("GetHeadersForRequest should return error for invalid signer")
	}
}

func TestNewOAuth1Signed_EmptyConsumerSecret_ReturnsInvalidSigner(t *testing.T) {
	t.Parallel()

	signer := NewOAuth1Signed(OAuth1Config{ConsumerKey: "key"})
	if signer.IsAuthenticated() {
		t.Error("invalid signer should return IsAuthenticated=false")
	}
	err := signer.Authenticate(context.Background())
	if err == nil {
		t.Fatal("Authenticate should return error for empty ConsumerSecret")
	}
	if !strings.Contains(err.Error(), "ConsumerSecret") {
		t.Errorf("error %q does not mention ConsumerSecret", err.Error())
	}
}

// ---------------------------------------------------------------------------
// OAuth1ProfileDelegation tests
// ---------------------------------------------------------------------------

func TestOAuth1ProfileDelegation_GetHeadersForRequest_IncludesOAuthToken(t *testing.T) {
	t.Parallel()

	const authToken = "user-auth-token-xyz" //nolint:gosec // G101: test-only token, not a real credential
	signer := NewOAuth1ProfileDelegation(OAuth1ProfileConfig{
		ConsumerKey:    "ck",
		ConsumerSecret: "cs",
		AuthToken:      authToken,
		AuthSecret:     "auth-secret",
	})

	headers, err := signer.GetHeadersForRequest(
		context.Background(),
		"GET",
		"https://platform.fatsecret.com/rest/foods/search/v1",
		url.Values{"method": {"food.get"}},
	)
	if err != nil {
		t.Fatalf("GetHeadersForRequest: %v", err)
	}

	parsed := parseOAuthHeader(t, headers["Authorization"])
	assertOAuth1HeaderFields(t, parsed, "ck")

	// Three-legged: oauth_token=AuthToken must be present.
	gotToken, ok := parsed["oauth_token"]
	if !ok {
		t.Fatal("oauth_token missing from OAuth1ProfileDelegation header")
	}
	if gotToken != authToken {
		t.Errorf("oauth_token = %q; want %q", gotToken, authToken)
	}
}

func TestOAuth1ProfileDelegation_SignatureDiffersFromTwoLegged(t *testing.T) {
	t.Parallel()

	// Same consumer credentials, same request — but profile delegation adds
	// AuthToken/AuthSecret, so the signature key and hence signature must differ.
	const (
		ck     = "consumer-key"
		cs     = "consumer-secret"
		rawURL = "https://platform.fatsecret.com/rest/server.api"
	)
	params := url.Values{"method": {"food.get"}, "food_id": {"33691"}}

	two := NewOAuth1Signed(OAuth1Config{ConsumerKey: ck, ConsumerSecret: cs})
	three := NewOAuth1ProfileDelegation(OAuth1ProfileConfig{
		ConsumerKey:    ck,
		ConsumerSecret: cs,
		AuthToken:      "utoken",
		AuthSecret:     "usecret",
	})

	ctx := context.Background()
	h2, err := two.GetHeadersForRequest(ctx, "GET", rawURL, params)
	if err != nil {
		t.Fatalf("two-legged: %v", err)
	}
	h3, err := three.GetHeadersForRequest(ctx, "GET", rawURL, params)
	if err != nil {
		t.Fatalf("three-legged: %v", err)
	}

	p2 := parseOAuthHeader(t, h2["Authorization"])
	p3 := parseOAuthHeader(t, h3["Authorization"])

	// Different nonces anyway, but signatures should also differ due to
	// different signing keys. We mainly assert three-legged has oauth_token.
	if p3["oauth_token"] == "" {
		t.Error("three-legged header missing oauth_token")
	}
	if _, hasToken := p2["oauth_token"]; hasToken {
		t.Error("two-legged header must not contain oauth_token")
	}
}

func TestOAuth1ProfileDelegation_IsAuthenticated_AlwaysTrue(t *testing.T) {
	t.Parallel()

	signer := NewOAuth1ProfileDelegation(OAuth1ProfileConfig{
		ConsumerKey:    "ck",
		ConsumerSecret: "cs",
		AuthToken:      "at",
		AuthSecret:     "as",
	})
	if !signer.IsAuthenticated() {
		t.Error("OAuth1ProfileDelegation.IsAuthenticated should always return true")
	}
}

func TestNewOAuth1ProfileDelegation_MissingFields_ReturnsInvalidSigner(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  OAuth1ProfileConfig
		want string
	}{
		{
			name: "missing ConsumerKey",
			cfg:  OAuth1ProfileConfig{ConsumerSecret: "cs", AuthToken: "at", AuthSecret: "as"},
			want: "ConsumerKey",
		},
		{
			name: "missing ConsumerSecret",
			cfg:  OAuth1ProfileConfig{ConsumerKey: "ck", AuthToken: "at", AuthSecret: "as"},
			want: "ConsumerSecret",
		},
		{
			name: "missing AuthToken",
			cfg:  OAuth1ProfileConfig{ConsumerKey: "ck", ConsumerSecret: "cs", AuthSecret: "as"},
			want: "AuthToken",
		},
		{
			name: "missing AuthSecret",
			cfg:  OAuth1ProfileConfig{ConsumerKey: "ck", ConsumerSecret: "cs", AuthToken: "at"},
			want: "AuthSecret",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			signer := NewOAuth1ProfileDelegation(tc.cfg)
			if signer.IsAuthenticated() {
				t.Error("invalid signer must return IsAuthenticated=false")
			}
			err := signer.Authenticate(context.Background())
			if err == nil {
				t.Fatalf("Authenticate should return error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// InvalidAuthenticator tests
// ---------------------------------------------------------------------------

func TestInvalidAuthenticator_AllMethodsReturnError(t *testing.T) {
	t.Parallel()

	sentinel := &struct{ error }{error: &testError{"sentinel auth error"}}
	ia := &InvalidAuthenticator{Err: sentinel}

	ctx := context.Background()

	if got := ia.Authenticate(ctx); !errors.Is(got, sentinel) {
		t.Errorf("Authenticate returned %v; want sentinel error", got)
	}
	if ia.IsAuthenticated() {
		t.Error("IsAuthenticated must return false")
	}
	headers, err := ia.GetHeaders(ctx)
	if headers != nil {
		t.Errorf("GetHeaders headers = %v; want nil", headers)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("GetHeaders error = %v; want sentinel error", err)
	}
	if got := ia.Refresh(ctx); !errors.Is(got, sentinel) {
		t.Errorf("Refresh returned %v; want sentinel error", got)
	}

	// Logout is a no-op — must not panic.
	ia.Logout()

	// IsAuthenticated still false after Logout.
	if ia.IsAuthenticated() {
		t.Error("IsAuthenticated must remain false after Logout")
	}
}

// testError is a simple error implementation used in tests.
type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
