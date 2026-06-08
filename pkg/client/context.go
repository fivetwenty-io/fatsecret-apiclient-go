package client

import (
	"context"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/internal/ctxkeys"
)

// WithRetries attaches a per-request retry count override to ctx. The value
// supersedes Options.MaxRetries for the duration of the request. Negative
// values are accepted; the retry middleware treats any value below zero as
// zero retries. The returned context is safe to pass to Client.Do.
func WithRetries(ctx context.Context, n int) context.Context {
	return ctxkeys.WithRetries(ctx, n)
}

// WithRetryDelay attaches a per-request base retry delay override to ctx. The
// value supersedes Options.RetryDelay for the duration of the request. A zero
// duration disables the base delay (jitter may still apply at the middleware
// level). The returned context is safe to pass to Client.Do.
func WithRetryDelay(ctx context.Context, d time.Duration) context.Context {
	return ctxkeys.WithRetryDelay(ctx, d)
}

// WithLogging enables or disables request-level logging for a single request,
// overriding the client-level LogConfig.Enabled setting. Pass true to enable
// and false to suppress all log output for the request.
func WithLogging(ctx context.Context, enabled bool) context.Context {
	return ctxkeys.WithLogging(ctx, enabled)
}

// WithLogFields merges the supplied fields into any log fields already attached
// to ctx, then stores the merged map. When a key exists in both the existing map
// and fields, the value from fields takes precedence. A nil fields map is safe
// and returns a context with a merged copy of the existing fields (or an empty
// map if none are present).
func WithLogFields(ctx context.Context, fields map[string]any) context.Context {
	return ctxkeys.WithLogFields(ctx, fields)
}

// WithForceRetry opts the request into the retry loop even if the HTTP method
// is non-idempotent (e.g., POST). Pass true to enable forced retry; false
// explicitly disables it, overriding any request-level default.
func WithForceRetry(ctx context.Context, force bool) context.Context {
	return ctxkeys.WithForceRetry(ctx, force)
}
