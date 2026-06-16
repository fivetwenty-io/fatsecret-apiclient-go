package errors

import (
	"encoding/json"
	"strconv"
	"strings"
)

// fatSecretErrorEnvelope mirrors the FatSecret error body shape:
//
//	{"error":{"code":N,"message":"..."}}
//
// FatSecret returns these envelopes with HTTP 200, so the body — not the HTTP
// status — is the authoritative failure signal for API-level errors.
type fatSecretErrorEnvelope struct {
	Error *struct {
		// Code is decoded as RawMessage because FatSecret has been observed to
		// encode it both as a JSON number (21) and as a JSON string ("21").
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	} `json:"error"`
}

// FromResponse inspects a FatSecret HTTP response and returns the appropriate
// typed error, or nil when the response indicates success.
//
// Detection order:
//
//  1. A FatSecret {"error":{"code","message"}} envelope in the body — FatSecret's
//     normal failure mode is HTTP 200 + error JSON — dispatched via
//     DispatchByFatSecretCode so callers can errors.Is against ErrIPBlocked,
//     ErrRateLimited, ErrUnauthorized, and the rest.
//  2. An HTTP status >= 400 with no parseable envelope, via DispatchByStatus.
//  3. Otherwise nil.
//
// It is safe to call on every response: success bodies carry no error envelope
// and 2xx statuses produce nil.
func FromResponse(status int, body []byte) error {
	if len(body) > 0 {
		var env fatSecretErrorEnvelope
		if err := json.Unmarshal(body, &env); err == nil && env.Error != nil {
			if code := parseErrorCode(env.Error.Code); code != 0 {
				return DispatchByFatSecretCode(code, env.Error.Message, status)
			}
		}
	}
	return DispatchByStatus(status, string(body))
}

// parseErrorCode extracts the integer FatSecret error code from its raw JSON
// value, tolerating both numeric (21) and quoted-string ("21") encodings.
// Returns 0 when the value is empty, null, or unparseable.
func parseErrorCode(raw json.RawMessage) int {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return 0
	}
	s = strings.Trim(s, `"`)
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

// DispatchByFatSecretCode converts a raw FatSecret API error code to the appropriate
// typed error. It is called by the response handler whenever the response body contains
// a FatSecret {"error":{"code":"N","message":"..."}} envelope.
//
// All documented FatSecret error codes are handled explicitly. Unknown codes produce a
// generic *APIError with ErrServer as the sentinel so the caller still gets a non-nil
// error with the original code preserved.
//
// Returns nil only when code is zero, which callers should treat as a no-error sentinel
// (although in practice DispatchByFatSecretCode is not called for success responses).
func DispatchByFatSecretCode(code int, msg string, httpStatus int) error { //nolint:cyclop // dispatch switch covers ~30 distinct FatSecret error codes; extraction would not reduce actual complexity
	if code == 0 {
		return nil
	}

	base := APIError{
		Code:       APIErrorCode(code),
		Message:    msg,
		HTTPStatus: httpStatus,
	}

	switch code {
	// --- OAuth 1.0 errors (codes 2-9) ---
	// These may surface from legacy integrations or misconfigured clients.
	// All map to AuthenticationError / ErrUnauthorized as the closest semantic match.
	case 2: // Invalid signature method
		base.sentinel = ErrUnauthorized
		return &AuthenticationError{APIError: base}
	case 3: // Invalid consumer key
		base.sentinel = ErrUnauthorized
		return &AuthenticationError{APIError: base}
	case 4: // Invalid signature
		base.sentinel = ErrUnauthorized
		return &AuthenticationError{APIError: base}
	case 5: // Invalid / expired timestamp
		base.sentinel = ErrUnauthorized
		return &AuthenticationError{APIError: base}
	case 6: // Invalid / used nonce
		base.sentinel = ErrUnauthorized
		return &AuthenticationError{APIError: base}
	case 7: // Invalid / expired token
		base.sentinel = ErrUnauthorized
		return &AuthenticationError{APIError: base}
	case 8: // Invalid token (alternate)
		base.sentinel = ErrUnauthorized
		return &AuthenticationError{APIError: base}
	case 9: // Invalid access token (OAuth 1.0)
		base.sentinel = ErrUnauthorized
		return &AuthenticationError{APIError: base}

	// --- General errors ---
	case 1: // Unknown error
		base.sentinel = ErrServer
		return &base

	case 10: // Unknown method
		base.sentinel = ErrServer
		return &base

	case 11: // Application request limit reached [FACT: R1 §4]
		base.sentinel = ErrRateLimited
		return &RateLimitError{APIError: base}

	case 12: // User performing too many actions
		base.sentinel = ErrRateLimited
		return &RateLimitError{APIError: base}

	case 13: // Invalid token (OAuth 2.0) [FACT: R1 §1]
		base.sentinel = ErrUnauthorized
		return &AuthenticationError{APIError: base}

	case 14: // Missing scope [FACT: R1 §1]
		base.sentinel = ErrForbidden
		return &PermissionError{APIError: base, MissingScope: extractScope(msg)}

	case 20: // System temporarily unavailable
		base.sentinel = ErrServer
		return &base

	case 21: // Invalid IP address detected [FACT: R1 §1]
		base.sentinel = ErrIPBlocked
		// Use PermissionError because the request is denied by access policy.
		// MissingScope is left empty — the restriction is IP-based, not scope-based.
		return &PermissionError{APIError: base}

	case 22: // Invalid request
		base.sentinel = ErrServer
		return &base

	case 23: // API not found
		base.sentinel = ErrServer
		return &base

	case 24: // Timeout occurred
		base.sentinel = ErrTimeout
		return &base

	// --- Parameter errors (codes 101-109) [FACT: R1 §4] ---
	case 101: // Missing required parameter
		base.sentinel = ErrParameter
		return &ParameterError{APIError: base}
	case 102: // Invalid integer value
		base.sentinel = ErrParameter
		return &ParameterError{APIError: base}
	case 103: // Invalid double value
		base.sentinel = ErrParameter
		return &ParameterError{APIError: base}
	case 104: // Invalid decimal value
		base.sentinel = ErrParameter
		return &ParameterError{APIError: base}
	case 105: // Invalid long value
		base.sentinel = ErrParameter
		return &ParameterError{APIError: base}
	case 106: // Invalid ID
		base.sentinel = ErrParameter
		return &ParameterError{APIError: base}
	case 107: // Value out of range
		base.sentinel = ErrParameter
		return &ParameterError{APIError: base}
	case 108: // Invalid type
		base.sentinel = ErrParameter
		return &ParameterError{APIError: base}
	case 109: // Character limit exceeded
		base.sentinel = ErrParameter
		return &ParameterError{APIError: base}

	// --- Application domain errors (codes 201-211) [FACT: R1 §4] ---
	// These are business-logic violations (duplicate entry, invalid food, etc.).
	// ParameterError is the closest semantic type; Param is left empty because
	// FatSecret messages for these codes do not reliably name a field.
	case 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211:
		base.sentinel = ErrParameter
		return &ParameterError{APIError: base}

	default:
		// Preserve unknown codes in a generic APIError so callers can inspect
		// the raw code value without losing information.
		base.sentinel = ErrServer
		return &base
	}
}

// DispatchByStatus converts an HTTP status code to a typed error for cases where
// the response does not contain a FatSecret error body (e.g., infrastructure-level
// rejections from a load balancer or gateway). It is called by the response handler
// after confirming no parseable FatSecret error envelope is present.
//
// Returns nil for status codes below 400, treating them as non-error responses.
// The body string is stored in APIError.Message for diagnostic purposes.
func DispatchByStatus(status int, body string) error {
	if status < 400 {
		return nil
	}

	base := APIError{
		HTTPStatus: status,
		Message:    body,
	}

	switch {
	case status == 401:
		base.sentinel = ErrUnauthorized
		return &AuthenticationError{APIError: base}

	case status == 403:
		base.sentinel = ErrForbidden
		return &PermissionError{APIError: base}

	case status == 404:
		base.sentinel = ErrNotFound
		return &base

	case status == 408:
		base.sentinel = ErrTimeout
		return &base

	case status == 409:
		base.sentinel = ErrConflict
		return &base

	case status == 429:
		base.sentinel = ErrRateLimited
		return &RateLimitError{APIError: base}

	case status >= 500:
		base.sentinel = ErrServer
		return &base

	default:
		// 4xx codes not listed above (e.g., 400, 405, 410, 422) get a generic
		// APIError with ErrServer as a fallback. Code is 0 (no FatSecret code).
		base.sentinel = ErrServer
		return &base
	}
}

// extractScope attempts to parse a scope name from a FatSecret error message.
// FatSecret messages for code 14 ("Missing scope") include the required scope in
// several observed formats. Returns an empty string if no recognizable pattern is found.
//
// Observed message patterns:
//   - "Missing scope: premier"
//   - "missing scope premier"
//   - "scope 'barcode' is required"
//   - "required scope: nlp"
func extractScope(msg string) string {
	lower := strings.ToLower(msg)

	// Pattern: "missing scope: X" or "missing scope X"
	if idx := strings.Index(lower, "missing scope"); idx != -1 {
		rest := strings.TrimSpace(msg[idx+len("missing scope"):])
		rest = strings.TrimPrefix(rest, ":")
		rest = strings.TrimSpace(rest)
		return extractFirstToken(rest)
	}

	// Pattern: "required scope: X"
	if idx := strings.Index(lower, "required scope"); idx != -1 {
		rest := strings.TrimSpace(msg[idx+len("required scope"):])
		rest = strings.TrimPrefix(rest, ":")
		rest = strings.TrimSpace(rest)
		return extractFirstToken(rest)
	}

	// Pattern: "scope 'X' is required" or "scope X is required"
	if idx := strings.Index(lower, "scope"); idx != -1 {
		rest := strings.TrimSpace(msg[idx+len("scope"):])
		rest = strings.Trim(rest, "'\" ")
		return extractFirstToken(rest)
	}

	return ""
}

// extractFirstToken returns the first whitespace-delimited, punctuation-stripped token
// from s, or an empty string if s is empty.
func extractFirstToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Stop at the first whitespace, single-quote, double-quote, or sentence terminator.
	end := strings.IndexAny(s, " \t\n\r'\".,;)")
	if end == -1 {
		return s
	}
	return s[:end]
}
