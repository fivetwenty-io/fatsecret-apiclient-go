package auth

import (
	"context"
	"net/url"
)

// Authenticator abstracts all authentication strategies behind a single interface.
// Every method must be safe for concurrent use by multiple goroutines.
type Authenticator interface {
	// Authenticate performs initial credential acquisition. For OAuth 2.0 strategies
	// this fetches the first access token. For OAuth 1.0a strategies this validates
	// that required configuration fields are non-empty. Calling Authenticate on an
	// already-authenticated instance is a no-op for OAuth 1.0a and re-fetches the
	// token for OAuth 2.0 (equivalent to Refresh).
	Authenticate(ctx context.Context) error

	// IsAuthenticated reports whether valid, non-expired credentials are currently
	// held. For OAuth 2.0 this returns false when no token has been fetched or when
	// the token is within the proactive-refresh window. For OAuth 1.0a this returns
	// true whenever the required configuration fields are non-empty.
	IsAuthenticated() bool

	// GetHeaders returns a map of HTTP header name → value that must be injected
	// into a single outbound request. For OAuth 2.0 the map contains a single
	// "Authorization: Bearer <token>" entry. For OAuth 1.0a the map also contains
	// "Authorization: OAuth …" but requires per-request context; callers that
	// hold an [OAuth1RequestSigner] should call [OAuth1RequestSigner.GetHeadersForRequest]
	// instead so that the signature covers the actual request parameters.
	// GetHeaders triggers a proactive token refresh for OAuth 2.0 when needed.
	GetHeaders(ctx context.Context) (map[string]string, error)

	// Refresh unconditionally re-acquires credentials. For OAuth 2.0 this posts a
	// new client-credentials grant. For OAuth 1.0a this is a no-op. Concurrent
	// calls collapse into a single in-flight request; all waiters receive the same
	// result.
	Refresh(ctx context.Context) error

	// Logout discards all held credentials. After Logout, IsAuthenticated returns
	// false and GetHeaders returns an error until Authenticate or Refresh is called.
	Logout()
}

// OAuth1RequestSigner extends [Authenticator] for strategies that compute a
// per-request OAuth 1.0a signature. The signature covers the HTTP method, the
// base URL, and all merged request parameters (query string plus form body for
// POST), so the full request context is required at signing time.
//
// The auth middleware calls GetHeadersForRequest instead of GetHeaders whenever
// the strategy implements this interface.
type OAuth1RequestSigner interface {
	Authenticator

	// GetHeadersForRequest builds and returns the OAuth 1.0a Authorization header
	// for a single request. method is the uppercase HTTP method (e.g. "GET",
	// "POST"). rawURL is the fully-qualified request URL including scheme and host
	// but without query string or fragment. params contains all query/form
	// parameters that will be sent with the request; they are merged with the
	// oauth_* parameters before signature computation.
	//
	// The returned map contains exactly one key, "Authorization", whose value is
	// the OAuth Authorization header value (e.g. `OAuth realm="", oauth_*=…`).
	GetHeadersForRequest(ctx context.Context, method, rawURL string, params url.Values) (map[string]string, error)
}
