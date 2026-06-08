package errors

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for coarse-grained matching via errors.Is.
// Every typed error in this package Unwraps to one of these.
var (
	// ErrUnauthorized signals an invalid, expired, or missing access token.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden signals insufficient OAuth scope or an IP address that is not allowlisted.
	ErrForbidden = errors.New("forbidden")

	// ErrNotFound signals a 404 response when no FatSecret error body is present.
	ErrNotFound = errors.New("not found")

	// ErrConflict signals a 409 response (duplicate resource or conflicting state).
	ErrConflict = errors.New("conflict")

	// ErrServer signals a FatSecret server-side failure or an HTTP 5xx response.
	ErrServer = errors.New("server error")

	// ErrRateLimited signals that the application or user request quota has been exceeded.
	ErrRateLimited = errors.New("rate limited")

	// ErrTimeout signals a request timeout (FatSecret code 24 or HTTP 408).
	ErrTimeout = errors.New("timeout")

	// ErrConnection signals a network-level connection failure (DNS, TCP, etc.).
	ErrConnection = errors.New("connection error")

	// ErrTLS signals a TLS handshake or certificate verification failure.
	ErrTLS = errors.New("TLS error")

	// ErrParameter signals that one or more request parameters are invalid.
	ErrParameter = errors.New("parameter error")

	// ErrIPBlocked signals that the request originated from a non-allowlisted IP address.
	ErrIPBlocked = errors.New("IP address not allowlisted")
)

// APIErrorCode is a FatSecret API error code, carried as an integer.
// The JSON envelope encodes this as a string; callers parse it before construction.
type APIErrorCode int

// APIError is the base error type for all FatSecret API errors.
// It carries the FatSecret error code from the response body, the human-readable
// message, the HTTP status of the response, and a wrapped sentinel for errors.Is.
//
// Typed subtypes embed APIError and inherit Error() and Unwrap(). Callers should
// prefer errors.As with the concrete subtype to access additional fields.
type APIError struct {
	// Code is the integer error code from the FatSecret error body.
	Code APIErrorCode

	// Message is the human-readable error message from the FatSecret error body.
	Message string

	// HTTPStatus is the HTTP response status code. FatSecret frequently returns
	// errors with HTTP 200; this field reflects the actual transport status.
	HTTPStatus int

	// sentinel is one of the Err* package-level errors; returned by Unwrap.
	sentinel error
}

// Error returns a lowercase string representation of the error without a trailing period.
func (e *APIError) Error() string {
	return fmt.Sprintf("fatsecret: code %d: %s (http %d)", e.Code, e.Message, e.HTTPStatus)
}

// Unwrap returns the sentinel error that represents the broad category of this error,
// enabling errors.Is matching against the package-level Err* variables.
func (e *APIError) Unwrap() error {
	return e.sentinel
}

// AuthenticationError is returned when FatSecret rejects the access token.
// It wraps ErrUnauthorized and is produced by OAuth error codes 9, 13, and
// the OAuth 1.0 codes 2-8 (which may appear from legacy integrations).
type AuthenticationError struct {
	APIError
}

// PermissionError is returned when the request is denied due to insufficient
// OAuth scope (FatSecret code 14) or an IP address that is not allowlisted
// (FatSecret code 21). MissingScope is populated only for code 14.
type PermissionError struct {
	APIError

	// MissingScope holds the scope name extracted from the FatSecret error message
	// when the denial was caused by a missing OAuth scope (code 14).
	// Empty string when the cause is an IP restriction or the scope cannot be parsed.
	MissingScope string
}

// ParameterError is returned when one or more request parameters are invalid,
// missing, or out of range. It covers FatSecret codes 101-109 and the
// application-domain codes 201-211.
type ParameterError struct {
	APIError

	// Param holds the parameter name when it can be determined from the error message.
	// Empty string when the parameter name is not present in the FatSecret message.
	Param string
}

// RateLimitError is returned when the application or user request quota is exceeded.
// It covers FatSecret code 11 (application limit) and code 12 (user limit).
// RetryAfter is always zero because FatSecret does not send a Retry-After header.
type RateLimitError struct {
	APIError

	// RetryAfter is always zero. FatSecret does not document or send Retry-After headers.
	RetryAfter time.Duration
}

// ConnectionError wraps a transport-level connection failure such as a DNS resolution
// error or a TCP connect timeout. It does not embed APIError because no FatSecret
// error body is present in this case.
type ConnectionError struct {
	// Cause is the underlying network error returned by the HTTP client.
	Cause error
}

// Error returns a lowercase string describing the connection failure without a trailing period.
func (e *ConnectionError) Error() string {
	return "fatsecret: connection: " + e.Cause.Error()
}

// Unwrap returns ErrConnection, enabling errors.Is(err, ErrConnection) matching.
func (e *ConnectionError) Unwrap() error {
	return ErrConnection
}

// TimeoutError wraps a transport-level timeout. Like ConnectionError, it does not
// embed APIError because the failure occurred before a FatSecret response was received.
// For FatSecret code 24 (timeout reported in response body), DispatchByFatSecretCode
// returns an *APIError with ErrTimeout as the sentinel instead.
type TimeoutError struct {
	// Cause is the underlying timeout error returned by the HTTP client.
	Cause error
}

// Error returns a lowercase string describing the timeout without a trailing period.
func (e *TimeoutError) Error() string {
	return "fatsecret: timeout: " + e.Cause.Error()
}

// Unwrap returns ErrTimeout, enabling errors.Is(err, ErrTimeout) matching.
func (e *TimeoutError) Unwrap() error {
	return ErrTimeout
}

// TLSError wraps a TLS handshake or certificate verification failure.
// Like ConnectionError and TimeoutError, it does not embed APIError.
type TLSError struct {
	// Cause is the underlying TLS error returned by the HTTP client or crypto/tls.
	Cause error
}

// Error returns a lowercase string describing the TLS failure without a trailing period.
func (e *TLSError) Error() string {
	return "fatsecret: tls: " + e.Cause.Error()
}

// Unwrap returns ErrTLS, enabling errors.Is(err, ErrTLS) matching.
func (e *TLSError) Unwrap() error {
	return ErrTLS
}
