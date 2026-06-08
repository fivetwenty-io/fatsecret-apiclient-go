package auth

import (
	"context"
	"fmt"
	"net/url"
)

// OAuth1ProfileConfig holds the consumer credentials plus the per-user access
// credentials returned by the FatSecret profile.create endpoint.
type OAuth1ProfileConfig struct {
	// ConsumerKey is the OAuth 1.0a consumer key issued by FatSecret.
	// Required; NewOAuth1ProfileDelegation returns an [InvalidAuthenticator] when empty.
	ConsumerKey string

	// ConsumerSecret is the OAuth 1.0a consumer secret issued by FatSecret.
	// Required; NewOAuth1ProfileDelegation returns an [InvalidAuthenticator] when empty.
	ConsumerSecret string

	// AuthToken is the per-user OAuth 1.0a access token returned by profile.create.
	// Required; NewOAuth1ProfileDelegation returns an [InvalidAuthenticator] when empty.
	AuthToken string

	// AuthSecret is the per-user OAuth 1.0a access token secret returned by profile.create.
	// Required; NewOAuth1ProfileDelegation returns an [InvalidAuthenticator] when empty.
	AuthSecret string

	// Realm is the optional realm value included in the Authorization header.
	// Defaults to the empty string, which is acceptable for the FatSecret API.
	Realm string
}

// OAuth1ProfileDelegation implements OAuth 1.0a per-user request signing using
// HMAC-SHA1. It signs requests on behalf of a specific FatSecret user profile by
// including the user's access token (oauth_token) and using the user's access
// token secret in the signing key.
//
// Create instances with [NewOAuth1ProfileDelegation]; the zero value is not valid.
type OAuth1ProfileDelegation struct {
	cfg OAuth1ProfileConfig
}

// NewOAuth1ProfileDelegation constructs and validates an [OAuth1ProfileDelegation]
// authenticator. It returns an [InvalidAuthenticator] wrapping a descriptive error
// when any required field in cfg is empty.
func NewOAuth1ProfileDelegation(cfg OAuth1ProfileConfig) OAuth1RequestSigner {
	switch {
	case cfg.ConsumerKey == "":
		return &invalidOAuth1Signer{inner: &InvalidAuthenticator{
			Err: fmt.Errorf("auth: OAuth1ProfileConfig.ConsumerKey must not be empty"),
		}}
	case cfg.ConsumerSecret == "":
		return &invalidOAuth1Signer{inner: &InvalidAuthenticator{
			Err: fmt.Errorf("auth: OAuth1ProfileConfig.ConsumerSecret must not be empty"),
		}}
	case cfg.AuthToken == "":
		return &invalidOAuth1Signer{inner: &InvalidAuthenticator{
			Err: fmt.Errorf("auth: OAuth1ProfileConfig.AuthToken must not be empty"),
		}}
	case cfg.AuthSecret == "":
		return &invalidOAuth1Signer{inner: &InvalidAuthenticator{
			Err: fmt.Errorf("auth: OAuth1ProfileConfig.AuthSecret must not be empty"),
		}}
	}
	return &OAuth1ProfileDelegation{cfg: cfg}
}

// Authenticate validates that all required credentials are present. It performs
// no network call and returns nil when configuration is valid.
func (a *OAuth1ProfileDelegation) Authenticate(_ context.Context) error { return nil }

// IsAuthenticated always returns true because the per-user access credentials
// are held in memory and do not expire on a time basis (FatSecret does not issue
// short-lived OAuth 1.0a access tokens).
func (a *OAuth1ProfileDelegation) IsAuthenticated() bool { return true }

// GetHeaders returns an OAuth 1.0a Authorization header computed without a
// request URL or parameters. The signature covers only the oauth_* parameters
// with an empty base URL, which is not a valid FatSecret request signature.
// Callers that hold the concrete type or the [OAuth1RequestSigner] interface
// should call [GetHeadersForRequest] so the signature covers the actual request.
func (a *OAuth1ProfileDelegation) GetHeaders(ctx context.Context) (map[string]string, error) {
	return a.GetHeadersForRequest(ctx, "GET", "", url.Values{})
}

// Refresh is a no-op for OAuth 1.0a delegation. The per-user access credentials
// do not expire and require no server-side refresh. Returns nil unconditionally.
func (a *OAuth1ProfileDelegation) Refresh(_ context.Context) error { return nil }

// Logout is a no-op for OAuth 1.0a delegation. Callers that need to revoke access
// must call the FatSecret profile API directly; this client holds no server-side
// session state to invalidate.
func (a *OAuth1ProfileDelegation) Logout() {}

// GetHeadersForRequest builds the OAuth 1.0a Authorization header for a single
// request, including the per-user access token (oauth_token=AuthToken) and using
// the per-user access token secret (AuthSecret) in the HMAC-SHA1 signing key.
//
// method is the uppercase HTTP method. rawURL is the base URL without query
// string. params contains all query/form parameters to be sent; they are merged
// with the oauth_* parameters before signature computation.
//
// The returned map contains exactly one entry: {"Authorization": "OAuth …"}.
func (a *OAuth1ProfileDelegation) GetHeadersForRequest(_ context.Context, method, rawURL string, params url.Values) (map[string]string, error) {
	return buildOAuth1Header(
		method, rawURL, params,
		a.cfg.ConsumerKey, a.cfg.ConsumerSecret,
		a.cfg.AuthToken, a.cfg.AuthSecret,
		a.cfg.Realm,
	)
}
