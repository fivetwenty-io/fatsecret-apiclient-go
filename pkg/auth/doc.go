// Package auth provides authentication strategies for the FatSecret API client.
//
// All strategies implement the [Authenticator] interface, which exposes lifecycle
// methods for credential acquisition, validation, refresh, and teardown. The two
// OAuth 1.0a strategies additionally implement [OAuth1RequestSigner], which
// requires the HTTP method, URL, and merged request parameters at signing time.
//
// Available implementations:
//
//   - [OAuth2ClientCredentials] — OAuth 2.0 client-credentials flow (recommended
//     for server-to-server access without per-user context).
//   - [OAuth1Signed] — two-legged OAuth 1.0a HMAC-SHA1 signing using a consumer
//     key and secret (required for profile.create and profile.get_auth).
//   - [OAuth1ProfileDelegation] — per-user OAuth 1.0a signing using a consumer
//     key/secret plus a user-specific auth token and auth secret returned by
//     profile.create.
//   - [InvalidAuthenticator] — deferred configuration error; surfaces only on
//     first use, enabling dependency-injection patterns where auth config may be
//     absent at construction time.
//
// All implementations are safe for concurrent use by multiple goroutines.
package auth
