package zapadapter

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/client"
)

// zapAdapter adapts a *zap.Logger to the client.Logger interface. It converts
// the map[string]any fields used by the Logger interface to zap.Field values so
// that all structured data flows through zap's high-performance encoder.
type zapAdapter struct {
	l *zap.Logger
}

// NewZapLogger wraps l in a client.Logger implementation backed by go.uber.org/zap.
// The returned Logger is safe for concurrent use. Passing a nil *zap.Logger
// panics at the first log call; pass zap.NewNop() when no output is desired.
func NewZapLogger(l *zap.Logger) client.Logger {
	return &zapAdapter{l: l}
}

// Debug implements client.Logger.Debug.
func (a *zapAdapter) Debug(msg string, fields map[string]any) {
	a.l.Debug(msg, fieldsFromMap(fields)...)
}

// Info implements client.Logger.Info.
func (a *zapAdapter) Info(msg string, fields map[string]any) {
	a.l.Info(msg, fieldsFromMap(fields)...)
}

// Warn implements client.Logger.Warn.
func (a *zapAdapter) Warn(msg string, fields map[string]any) {
	a.l.Warn(msg, fieldsFromMap(fields)...)
}

// Error implements client.Logger.Error.
func (a *zapAdapter) Error(msg string, fields map[string]any) {
	a.l.Error(msg, fieldsFromMap(fields)...)
}

// fieldsFromMap converts a map[string]any to a slice of zap.Field values.
// Each value is encoded via zap.Any, which selects the most efficient zap
// encoder for the concrete type. Map iteration order is non-deterministic;
// callers that need deterministic field ordering should sort the fields before
// passing them or use a custom zap encoder.
func fieldsFromMap(fields map[string]any) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	zfields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		// Use the zap type-specific constructors for common types to avoid
		// the reflect-based path in zap.Any where possible.
		switch val := v.(type) {
		case string:
			zfields = append(zfields, zap.String(k, val))
		case int:
			zfields = append(zfields, zap.Int(k, val))
		case int64:
			zfields = append(zfields, zap.Int64(k, val))
		case float64:
			zfields = append(zfields, zap.Float64(k, val))
		case bool:
			zfields = append(zfields, zap.Bool(k, val))
		case error:
			zfields = append(zfields, zap.NamedError(k, val))
		case zapcore.ObjectMarshaler:
			zfields = append(zfields, zap.Object(k, val))
		default:
			zfields = append(zfields, zap.Any(k, v))
		}
	}
	return zfields
}
