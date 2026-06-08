package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/internal/oauth1"
)

// OAuth1Config holds the consumer credentials for OAuth 1.0a two-legged signing.
type OAuth1Config struct {
	// ConsumerKey is the OAuth 1.0a consumer key issued by FatSecret.
	// Required; NewOAuth1Signed returns an [InvalidAuthenticator] when empty.
	ConsumerKey string

	// ConsumerSecret is the OAuth 1.0a consumer secret issued by FatSecret.
	// Required; NewOAuth1Signed returns an [InvalidAuthenticator] when empty.
	ConsumerSecret string

	// Realm is the optional realm value included in the Authorization header.
	// Defaults to the empty string, which is acceptable for the FatSecret API.
	Realm string
}

// OAuth1Signed implements two-legged OAuth 1.0a request signing using HMAC-SHA1.
// It holds only consumer credentials; no access token is included. This strategy
// is required for endpoints such as profile.create and profile.get_auth that
// accept signed requests without a per-user access token.
//
// Create instances with [NewOAuth1Signed]; the zero value is not valid.
type OAuth1Signed struct {
	cfg OAuth1Config
}

// NewOAuth1Signed constructs and validates an [OAuth1Signed] authenticator.
// It returns an [InvalidAuthenticator] wrapping a descriptive error when
// cfg.ConsumerKey or cfg.ConsumerSecret is empty.
func NewOAuth1Signed(cfg OAuth1Config) OAuth1RequestSigner {
	if cfg.ConsumerKey == "" {
		return &invalidOAuth1Signer{inner: &InvalidAuthenticator{
			Err: fmt.Errorf("auth: OAuth1Config.ConsumerKey must not be empty"),
		}}
	}
	if cfg.ConsumerSecret == "" {
		return &invalidOAuth1Signer{inner: &InvalidAuthenticator{
			Err: fmt.Errorf("auth: OAuth1Config.ConsumerSecret must not be empty"),
		}}
	}
	return &OAuth1Signed{cfg: cfg}
}

// Authenticate validates that the consumer key and secret are non-empty; it does
// not perform any network call. Returns nil when configuration is valid.
func (a *OAuth1Signed) Authenticate(_ context.Context) error {
	// Configuration is validated at construction; Authenticate is a no-op for OAuth1.
	return nil
}

// IsAuthenticated always returns true because OAuth 1.0a signing requires no
// token acquisition; the consumer credentials are sufficient and do not expire.
func (a *OAuth1Signed) IsAuthenticated() bool { return true }

// GetHeaders returns an OAuth 1.0a Authorization header computed without a
// request URL or parameters. The signature covers only the oauth_* parameters
// with an empty base URL, which is not a valid FatSecret request signature.
// Callers that hold the concrete type or the [OAuth1RequestSigner] interface
// should call [GetHeadersForRequest] so the signature covers the actual request.
func (a *OAuth1Signed) GetHeaders(ctx context.Context) (map[string]string, error) {
	return a.GetHeadersForRequest(ctx, "GET", "", url.Values{})
}

// Refresh is a no-op for OAuth 1.0a signing. The consumer credentials do not
// expire and require no server-side refresh. Returns nil unconditionally.
func (a *OAuth1Signed) Refresh(_ context.Context) error { return nil }

// Logout is a no-op for OAuth 1.0a signing. There are no held tokens to discard.
func (a *OAuth1Signed) Logout() {}

// GetHeadersForRequest builds the OAuth 1.0a Authorization header for a single
// request. method is the uppercase HTTP method. rawURL is the base URL without
// query string. params contains all query/form parameters to be sent; they are
// merged with the oauth_* parameters before signature computation.
//
// The returned map contains exactly one entry: {"Authorization": "OAuth …"}.
func (a *OAuth1Signed) GetHeadersForRequest(_ context.Context, method, rawURL string, params url.Values) (map[string]string, error) {
	return buildOAuth1Header(method, rawURL, params, a.cfg.ConsumerKey, a.cfg.ConsumerSecret, "", "", a.cfg.Realm)
}

// buildOAuth1Header is the shared signing logic used by both [OAuth1Signed] and
// [OAuth1ProfileDelegation]. It constructs the oauth_* parameters, computes the
// HMAC-SHA1 signature, and formats the Authorization header value.
//
// tokenKey and tokenSecret are empty strings for two-legged (no access token) flows.
// realm may be empty; the FatSecret API accepts an empty realm.
func buildOAuth1Header(
	method, rawURL string,
	params url.Values,
	consumerKey, consumerSecret,
	tokenKey, tokenSecret,
	realm string,
) (map[string]string, error) {
	nonce, err := generateNonce()
	if err != nil {
		return nil, fmt.Errorf("auth: generating OAuth1 nonce: %w", err)
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)

	// Build the full parameter map: merge request params + oauth_* params.
	// oauth_signature is excluded from normalization per RFC 5849 §3.4.1.
	allParams := make(map[string]string)
	for k, vals := range params {
		if len(vals) > 0 {
			allParams[k] = vals[0]
		}
	}
	allParams["oauth_consumer_key"] = consumerKey
	allParams["oauth_nonce"] = nonce
	allParams["oauth_signature_method"] = "HMAC-SHA1"
	allParams["oauth_timestamp"] = ts
	allParams["oauth_version"] = "1.0"
	if tokenKey != "" {
		allParams["oauth_token"] = tokenKey
	}

	sig := oauth1.Sign(method, rawURL, allParams, consumerSecret, tokenSecret)

	// Build the Authorization header value.
	// Quoted parameter values use the oauth_* params only (not request params).
	var b strings.Builder
	b.WriteString("OAuth ")
	b.WriteString(`realm="`)
	b.WriteString(realm)
	b.WriteString(`"`)

	writeOAuthParam := func(k, v string) {
		b.WriteString(", ")
		b.WriteString(k)
		b.WriteString(`="`)
		b.WriteString(v)
		b.WriteByte('"')
	}

	writeOAuthParam("oauth_consumer_key", consumerKey)
	writeOAuthParam("oauth_nonce", nonce)
	writeOAuthParam("oauth_signature", sig)
	writeOAuthParam("oauth_signature_method", "HMAC-SHA1")
	writeOAuthParam("oauth_timestamp", ts)
	if tokenKey != "" {
		writeOAuthParam("oauth_token", tokenKey)
	}
	writeOAuthParam("oauth_version", "1.0")

	return map[string]string{"Authorization": b.String()}, nil
}

// generateNonce produces a 16-byte cryptographically random hex nonce using
// crypto/rand. Returns an error only if the system entropy source fails.
func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// invalidOAuth1Signer wraps an [InvalidAuthenticator] and satisfies the
// [OAuth1RequestSigner] interface so that NewOAuth1Signed can return a typed
// value even when configuration is invalid.
type invalidOAuth1Signer struct {
	inner *InvalidAuthenticator
}

func (s *invalidOAuth1Signer) Authenticate(_ context.Context) error { return s.inner.Err }
func (s *invalidOAuth1Signer) IsAuthenticated() bool                { return false }
func (s *invalidOAuth1Signer) GetHeaders(_ context.Context) (map[string]string, error) {
	return nil, s.inner.Err
}
func (s *invalidOAuth1Signer) Refresh(_ context.Context) error { return s.inner.Err }
func (s *invalidOAuth1Signer) Logout()                         {}
func (s *invalidOAuth1Signer) GetHeadersForRequest(_ context.Context, _, _ string, _ url.Values) (map[string]string, error) {
	return nil, s.inner.Err
}
