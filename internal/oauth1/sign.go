package oauth1

import (
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- HMAC-SHA1 is mandated by the OAuth 1.0a signature method (RFC 5849)
	"encoding/base64"
	"strings"
)

// BaseString constructs the OAuth 1.0a signature base string as defined by
// RFC 5849 §3.4.1. The base string is the concatenation of three components,
// each percent-encoded and separated by '&':
//
//  1. The uppercase HTTP method (e.g. "GET" or "POST").
//  2. The base URL — the scheme, host, and path with no query string or fragment.
//  3. The normalized parameter string produced by encoding and sorting all
//     request parameters (including oauth_* parameters, excluding oauth_signature).
//
// The caller must include all OAuth protocol parameters (oauth_consumer_key,
// oauth_nonce, oauth_signature_method, oauth_timestamp, oauth_version, and
// oauth_token when applicable) in params before calling BaseString. The
// oauth_signature parameter must NOT be present in params.
func BaseString(method, baseURL string, params map[string]string) string {
	return strings.ToUpper(method) + "&" +
		percentEncode(baseURL) + "&" +
		percentEncode(normalizeParams(params))
}

// Sign computes the OAuth 1.0a HMAC-SHA1 signature for the described request
// and returns it as a standard base64-encoded string (RFC 4648 §4).
//
// The signing key is constructed per RFC 5849 §3.4.2:
//
//	percentEncode(consumerSecret) + "&" + percentEncode(tokenSecret)
//
// When no access token is in use (e.g. two-legged OAuth1 signed requests),
// tokenSecret must be the empty string ""; the '&' separator is still included
// in the signing key per the specification.
//
// method must be the HTTP method string (case-insensitive; uppercased internally).
// baseURL must be the base URI (scheme + host + path, no query or fragment).
// params must contain all request and OAuth protocol parameters except
// oauth_signature.
func Sign(method, baseURL string, params map[string]string, consumerSecret, tokenSecret string) string {
	baseString := BaseString(method, baseURL, params)
	signingKey := percentEncode(consumerSecret) + "&" + percentEncode(tokenSecret)

	mac := hmac.New(sha1.New, []byte(signingKey)) // #nosec G505 -- HMAC-SHA1 is mandated by the OAuth 1.0a signature method (RFC 5849)
	mac.Write([]byte(baseString))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
