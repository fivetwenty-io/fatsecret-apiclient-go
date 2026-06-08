package client

import (
	"context"
	"log/slog"
)

// slogAdapter adapts a *slog.Logger to the Logger interface. It bridges the
// structured fields map[string]any used by the Logger interface to slog's
// variadic attribute model. No external dependencies are required; log/slog is
// part of the Go standard library since Go 1.21.
type slogAdapter struct {
	l *slog.Logger
}

// NewSlogLogger wraps l in a Logger implementation backed by the standard
// library log/slog package. The returned Logger is safe for concurrent use.
// Passing a nil *slog.Logger panics at the first log call; pass slog.Default()
// when no custom logger is needed.
func NewSlogLogger(l *slog.Logger) Logger {
	return &slogAdapter{l: l}
}

// Debug implements Logger.Debug by emitting a slog.LevelDebug record.
func (a *slogAdapter) Debug(msg string, fields map[string]any) {
	a.l.LogAttrs(context.Background(), slog.LevelDebug, msg, attrsFromFields(fields)...)
}

// Info implements Logger.Info by emitting a slog.LevelInfo record.
func (a *slogAdapter) Info(msg string, fields map[string]any) {
	a.l.LogAttrs(context.Background(), slog.LevelInfo, msg, attrsFromFields(fields)...)
}

// Warn implements Logger.Warn by emitting a slog.LevelWarn record.
func (a *slogAdapter) Warn(msg string, fields map[string]any) {
	a.l.LogAttrs(context.Background(), slog.LevelWarn, msg, attrsFromFields(fields)...)
}

// Error implements Logger.Error by emitting a slog.LevelError record.
func (a *slogAdapter) Error(msg string, fields map[string]any) {
	a.l.LogAttrs(context.Background(), slog.LevelError, msg, attrsFromFields(fields)...)
}

// attrsFromFields converts a map[string]any into a slice of slog.Attr values.
// The map iteration order is non-deterministic; callers that need stable output
// for tests should use a handler that sorts attributes.
func attrsFromFields(fields map[string]any) []slog.Attr {
	if len(fields) == 0 {
		return nil
	}
	attrs := make([]slog.Attr, 0, len(fields))
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	return attrs
}
