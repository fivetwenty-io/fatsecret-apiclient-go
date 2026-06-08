package http

import (
	"context"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/internal/ctxkeys"
)

// retryableStatuses are HTTP status codes that trigger automatic retry
// independent of method idempotency.
var retryableStatuses = map[int]bool{
	http.StatusRequestTimeout:      true, // 408
	http.StatusTooManyRequests:     true, // 429
	http.StatusInternalServerError: true, // 500
	http.StatusBadGateway:          true, // 502
	http.StatusServiceUnavailable:  true, // 503
	http.StatusGatewayTimeout:      true, // 504
}

// idempotentMethods are HTTP methods that may be retried automatically without
// side-effect risk.
var idempotentMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
	http.MethodPut:     true,
	http.MethodDelete:  true,
}

// isRetryable reports whether a failed attempt should be retried. Non-idempotent
// methods (POST, PATCH) are only retried when ctxkeys.ForceRetry is true in ctx.
// Network errors (resp nil, err non-nil) are always retried for eligible methods.
func isRetryable(ctx context.Context, method string, resp *http.Response, err error) bool {
	force, _ := ctxkeys.ForceRetry(ctx)
	if !idempotentMethods[method] && !force {
		return false
	}
	if err != nil {
		// Any transport-level error (network unreachable, EOF, DNS) is retryable
		// for eligible methods.
		return true
	}
	return retryableStatuses[resp.StatusCode]
}

// RetryMiddleware returns a Middleware that retries requests up to maxRetries
// additional times (so total attempts = maxRetries+1) with exponential backoff.
// Per-request overrides from ctxkeys.RetryCount and ctxkeys.RetryDelay supersede
// the constructor arguments. Backoff = baseDelay * 2^attempt + 10% jitter.
// The middleware respects context cancellation between attempts.
func RetryMiddleware(maxRetries int, baseDelay time.Duration) Middleware { //nolint:cyclop // retry loop with backoff, context handling, and body-rewind all in one function
	return func(next RoundTripFunc) RoundTripFunc {
		return func(req *http.Request) (*http.Response, error) {
			ctx := req.Context()

			// Per-request overrides.
			maxAttempts := maxRetries
			if n, ok := ctxkeys.RetryCount(ctx); ok {
				if n < 0 {
					n = 0
				}
				maxAttempts = n
			}
			delay := baseDelay
			if d, ok := ctxkeys.RetryDelay(ctx); ok {
				delay = d
			}

			// Ensure the body is captured for replay before the first attempt.
			if err := captureBody(req); err != nil {
				return nil, err
			}

			var (
				resp *http.Response
				err  error
			)

			for attempt := 0; attempt <= maxAttempts; attempt++ {
				// Check context before each attempt (after the first sleep).
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}

				// Rewind body for every attempt after the first.
				if attempt > 0 {
					if req.GetBody != nil {
						body, gbErr := req.GetBody()
						if gbErr != nil {
							return nil, gbErr
						}
						req.Body = body
					}
				}

				resp, err = next(req)

				if attempt >= maxAttempts {
					break
				}
				if !isRetryable(ctx, req.Method, resp, err) {
					break
				}

				// Drain and close the response body before the next attempt to
				// allow connection reuse.
				if resp != nil && resp.Body != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
					resp = nil
				}

				// Exponential backoff with 10% jitter.
				backoff := delay * (1 << uint(attempt))
				jitter := time.Duration(float64(backoff) * 0.10 * rand.Float64()) // #nosec G404 -- math/rand/v2 jitter for retry backoff is not security-sensitive
				wait := backoff + jitter

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(wait):
				}
			}

			return resp, err
		}
	}
}
