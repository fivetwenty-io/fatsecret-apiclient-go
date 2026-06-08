package http

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/internal/ctxkeys"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/metrics"
)

// LogConfig controls logging behaviour for LoggingMiddleware.
type LogConfig struct {
	// Enabled enables structured log output. When false, no log lines are
	// emitted, but metrics and hooks are still fired (see D-08).
	Enabled bool

	// BodySample enables sampling of request and response bodies in log output.
	// When false, body fields are omitted from log entries. Metrics and hooks
	// are unaffected by this setting.
	BodySample bool

	// MaxBytes caps the number of body bytes included in each log entry when
	// BodySample is true. Zero is treated as "no limit" only when BodySample
	// is false; when BodySample is true and MaxBytes is zero, no body bytes
	// are logged. Callers should set a reasonable value such as 4096.
	MaxBytes int
}

// LoggingMiddleware returns a Middleware that instruments every HTTP round-trip
// with structured logging, metrics collection, and hook dispatch. Logging output
// is gated by cfg.Enabled and the per-request ctxkeys.Logging override, but
// metrics and hooks fire unconditionally on every request (D-08).
func LoggingMiddleware(log Logger, hooks []Hook, mc metrics.Collector, cfg LogConfig) Middleware { //nolint:cyclop // handles logging, metrics, and hooks in a single pass to minimize allocations; extraction would add overhead
	return func(next RoundTripFunc) RoundTripFunc {
		return func(req *http.Request) (*http.Response, error) {
			ctx := req.Context()

			// Resolve effective logging flag: per-request override wins.
			loggingEnabled := cfg.Enabled
			if override, ok := ctxkeys.Logging(ctx); ok {
				loggingEnabled = override
			}

			// Merge any per-request extra fields.
			extraFields, _ := ctxkeys.LogFields(ctx)

			method := req.Method
			rawURL := req.URL.String()
			path := req.URL.Path

			// Measure request body size for metrics (GetBody keeps original intact).
			var bytesSent int64
			if req.ContentLength > 0 {
				bytesSent = req.ContentLength
			}

			mc.IncActiveConnections()
			defer mc.DecActiveConnections()

			if loggingEnabled && log != nil {
				fields := buildPreSendFields(req, extraFields)
				log.Debug("fatsecret: sending request", fields)
			}

			start := time.Now()
			resp, err := next(req)
			duration := time.Since(start)

			// Read and buffer the response body so we can measure size,
			// sample it for logging, and restore it for the caller.
			var bytesRecv int64
			var bodyBytes []byte
			statusCode := 0

			if resp != nil {
				statusCode = resp.StatusCode
				if resp.Body != nil {
					bodyBytes, _ = io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					bytesRecv = int64(len(bodyBytes))
					resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
			}

			// Always emit metrics (D-08).
			mc.IncRequests(method, path)
			mc.ObserveDuration(method, path, duration)
			if bytesSent > 0 {
				mc.AddBytesSent(bytesSent)
			}
			if bytesRecv > 0 {
				mc.AddBytesRecv(bytesRecv)
			}
			if err != nil || (statusCode != 0 && (statusCode < 200 || statusCode >= 300)) {
				mc.IncFailures(method, path, statusCode)
			}

			// Build and fire every hook (D-08).
			event := HookEvent{
				Method:     method,
				URL:        rawURL,
				StatusCode: statusCode,
				Duration:   duration,
				BytesSent:  bytesSent,
				BytesRecv:  bytesRecv,
				Error:      err,
			}
			for _, h := range hooks {
				if h != nil {
					h(event)
				}
			}

			// Post-send log output, gated by cfg.Enabled.
			if loggingEnabled && log != nil {
				fields := buildPostSendFields(event, resp, bodyBytes, cfg, extraFields)
				if err != nil || (statusCode != 0 && (statusCode < 200 || statusCode >= 300)) {
					log.Warn("fatsecret: request completed with error", fields)
				} else {
					log.Debug("fatsecret: request completed", fields)
				}
			}

			return resp, err
		}
	}
}

// buildPreSendFields builds the structured log fields map for the pre-send log entry.
func buildPreSendFields(req *http.Request, extra map[string]any) map[string]any {
	fields := map[string]any{
		"method": req.Method,
		"url":    req.URL.String(),
	}
	// Redact sensitive headers before logging.
	redacted := RedactHeaders(req.Header)
	headerMap := make(map[string]string, len(redacted))
	for k, vv := range redacted {
		if len(vv) > 0 {
			headerMap[k] = vv[0]
		}
	}
	fields["headers"] = headerMap

	// Redact query params.
	fields["params"] = RedactParams(req.URL.Query())

	for k, v := range extra {
		fields[k] = v
	}
	return fields
}

// buildPostSendFields builds the structured log fields map for the post-send log entry.
func buildPostSendFields(
	ev HookEvent,
	resp *http.Response,
	bodyBytes []byte,
	cfg LogConfig,
	extra map[string]any,
) map[string]any {
	fields := map[string]any{
		"method":      ev.Method,
		"url":         ev.URL,
		"status_code": ev.StatusCode,
		"duration_ms": ev.Duration.Milliseconds(),
		"bytes_sent":  ev.BytesSent,
		"bytes_recv":  ev.BytesRecv,
	}
	if ev.Error != nil {
		fields["error"] = ev.Error.Error()
	}

	if cfg.BodySample && resp != nil && len(bodyBytes) > 0 {
		sample := bodyBytes
		if cfg.MaxBytes > 0 && len(sample) > cfg.MaxBytes {
			sample = sample[:cfg.MaxBytes]
		}
		// Redact: replace token/password/secret values in body is not feasible
		// without a JSON parser; log the raw bytes truncated.
		fields["body_sample"] = string(sample)
	}

	for k, v := range extra {
		fields[k] = v
	}
	return fields
}
