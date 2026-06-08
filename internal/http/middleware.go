// Package http provides the middleware chain, transport, and request-building
// utilities for the FatSecret API client. It defines canonical observability
// types (Logger, Hook, HookEvent) that pkg/client re-exports via type alias.
package http

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RoundTripFunc is the common unit in the middleware chain. It matches the
// signature of http.RoundTripper.RoundTrip so it can wrap a real *http.Client.
// Context is carried by req.Context() per net/http conventions.
type RoundTripFunc func(req *http.Request) (*http.Response, error)

// Middleware wraps a RoundTripFunc with additional behavior. Middlewares are
// composed by Chain; each link receives the next link as its argument and
// returns a new RoundTripFunc.
type Middleware func(next RoundTripFunc) RoundTripFunc

// Chain builds an ordered middleware chain terminating at base. The first
// middleware in mws runs outermost (first on the way in, last on the way out).
// Construction iterates mws in reverse so that mws[0] wraps mws[1] which wraps
// … which wraps base.
//
// Intended call order: Cache → Auth → Retry → Logging → base (net/http).
func Chain(base RoundTripFunc, mws ...Middleware) RoundTripFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		base = mws[i](base)
	}
	return base
}

// RedactHeaders returns a shallow copy of h with values replaced by "[REDACTED]"
// for any header whose name matches authorization, password, token, or secret
// (case-insensitive). The original h is not modified.
func RedactHeaders(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, vv := range h {
		lower := strings.ToLower(k)
		if isSensitiveKey(lower) {
			out[k] = []string{"[REDACTED]"}
		} else {
			copied := make([]string, len(vv))
			copy(copied, vv)
			out[k] = copied
		}
	}
	return out
}

// RedactParams returns a shallow copy of v with values replaced by "[REDACTED]"
// for any key whose name matches authorization, password, token, or secret
// (case-insensitive). The original v is not modified.
func RedactParams(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vv := range v {
		lower := strings.ToLower(k)
		if isSensitiveKey(lower) {
			out[k] = []string{"[REDACTED]"}
		} else {
			copied := make([]string, len(vv))
			copy(copied, vv)
			out[k] = copied
		}
	}
	return out
}

// isSensitiveKey reports whether a lowercase key name is a sensitive credential field.
func isSensitiveKey(lower string) bool {
	return strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret")
}

// Logger is the canonical logging interface for the FatSecret client transport.
// pkg/client re-exports this type via alias. Implementations must be safe for
// concurrent use. Each method receives a human-readable message and a map of
// structured fields; callers do not guarantee field map immutability after return.
type Logger interface {
	Debug(msg string, fields map[string]any)
	Info(msg string, fields map[string]any)
	Warn(msg string, fields map[string]any)
	Error(msg string, fields map[string]any)
}

// HookEvent carries a point-in-time snapshot of a completed HTTP round-trip.
// It is passed to every Hook registered on the transport after each request,
// regardless of success or failure.
type HookEvent struct {
	// Method is the uppercase HTTP method of the request (e.g., "GET", "POST").
	Method string

	// URL is the full request URL including query string.
	URL string

	// StatusCode is the HTTP response status code. Zero when a transport-level
	// error occurred before a response was received.
	StatusCode int

	// Duration is the elapsed time from the moment the request was dispatched
	// until the response body was fully read (or the error was recorded).
	Duration time.Duration

	// BytesSent is the number of bytes in the request body. Zero for bodyless
	// requests (GET, HEAD, etc.).
	BytesSent int64

	// BytesRecv is the number of bytes read from the response body.
	BytesRecv int64

	// Error is non-nil when the request failed due to a transport error or when
	// the response handler returned a typed API error.
	Error error
}

// Hook is a callback fired after every HTTP round-trip. Hooks receive a
// populated HookEvent and must not block; long-running work should be
// dispatched to a goroutine inside the hook implementation. Hooks are fired
// even when the transport returns an error.
type Hook func(HookEvent)
