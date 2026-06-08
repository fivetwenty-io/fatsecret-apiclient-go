// Package ctxkeys is the single owner of the unexported context-key type used
// for per-request overrides throughout the FatSecret client library. Both the
// public pkg/client WithXxx setter wrappers and the internal transport
// middleware readers import this package to set and retrieve values; no other
// package defines these context keys.
package ctxkeys

import (
	"context"
	"time"
)

// ctxKey is an unexported type for context keys defined in this package.
// Using a named type prevents key collisions with any other package that stores
// values in a context.Context.
type ctxKey uint8

const (
	keyRetries    ctxKey = iota // int: per-request max retry count override
	keyRetryDelay               // time.Duration: per-request base retry delay override
	keyLogging                  // bool: per-request logging enable/disable override
	keyLogFields                // map[string]any: extra structured log fields
	keyForceRetry               // bool: force retry even for non-idempotent requests
)

// WithRetries attaches a per-request retry count override to ctx. The value
// supersedes the client-level Options.MaxRetries for the duration of the
// request. Negative values are stored as-is; the retry middleware treats any
// value less than zero as zero retries.
func WithRetries(ctx context.Context, n int) context.Context {
	return context.WithValue(ctx, keyRetries, n)
}

// WithRetryDelay attaches a per-request base retry delay override to ctx. The
// value supersedes the client-level Options.RetryDelay for the duration of the
// request. A zero duration disables the base delay (jitter may still apply at
// the middleware level).
func WithRetryDelay(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, keyRetryDelay, d)
}

// WithLogging enables or disables request-level logging for a single request,
// overriding the client-level logging configuration. Pass true to enable and
// false to suppress all log output for the request.
func WithLogging(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, keyLogging, enabled)
}

// WithLogFields merges the supplied fields into any log fields already attached
// to ctx, then stores the merged map. When a key exists in both the existing
// map and fields, the value from fields takes precedence. Passing a nil fields
// map is safe and returns a context with a merged copy of whatever was already
// present (or an empty map if nothing was).
func WithLogFields(ctx context.Context, fields map[string]any) context.Context {
	existing, _ := ctx.Value(keyLogFields).(map[string]any)
	merged := make(map[string]any, len(existing)+len(fields))
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range fields {
		merged[k] = v
	}
	return context.WithValue(ctx, keyLogFields, merged)
}

// WithForceRetry opts the request into the retry loop even if the HTTP method
// is non-idempotent (e.g., POST). Pass true to enable forced retry; false
// explicitly disables it, overriding any client-level default.
func WithForceRetry(ctx context.Context, force bool) context.Context {
	return context.WithValue(ctx, keyForceRetry, force)
}

// RetryCount returns the per-request retry count attached to ctx by
// WithRetries. The second return value is false when no override is present.
func RetryCount(ctx context.Context) (int, bool) {
	v, ok := ctx.Value(keyRetries).(int)
	return v, ok
}

// RetryDelay returns the per-request base retry delay attached to ctx by
// WithRetryDelay. The second return value is false when no override is present.
func RetryDelay(ctx context.Context) (time.Duration, bool) {
	v, ok := ctx.Value(keyRetryDelay).(time.Duration)
	return v, ok
}

// Logging returns the per-request logging flag attached to ctx by WithLogging.
// The second return value is false when no override is present, meaning the
// middleware should fall back to the client-level logging configuration.
func Logging(ctx context.Context) (bool, bool) {
	v, ok := ctx.Value(keyLogging).(bool)
	return v, ok
}

// LogFields returns the merged structured log fields attached to ctx by one or
// more calls to WithLogFields. The returned map must be treated as read-only by
// callers. The second return value is false when no fields are present.
func LogFields(ctx context.Context) (map[string]any, bool) {
	v, ok := ctx.Value(keyLogFields).(map[string]any)
	return v, ok
}

// ForceRetry returns the per-request force-retry flag attached to ctx by
// WithForceRetry. The second return value is false when no override is present.
func ForceRetry(ctx context.Context) (bool, bool) {
	v, ok := ctx.Value(keyForceRetry).(bool)
	return v, ok
}
