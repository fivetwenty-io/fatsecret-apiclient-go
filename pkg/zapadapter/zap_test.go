package zapadapter

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// newObservedLogger returns a *zap.Logger backed by an in-memory observer and
// the observer sink for assertion. The logger records all levels.
func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return zap.New(core), logs
}

// findField searches logged fields for one with the given key.
func findField(fields []zap.Field, key string) (zap.Field, bool) {
	for _, f := range fields {
		if f.Key == key {
			return f, true
		}
	}
	return zap.Field{}, false
}

// ---------------------------------------------------------------------------
// Level routing
// ---------------------------------------------------------------------------

func TestNewZapLogger_Debug(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	logger.Debug("debug message", map[string]any{"k": "v"})

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}
	entry := logs.All()[0]
	if entry.Level != zapcore.DebugLevel {
		t.Errorf("want DebugLevel, got %v", entry.Level)
	}
	if entry.Message != "debug message" {
		t.Errorf("want message 'debug message', got %q", entry.Message)
	}
}

func TestNewZapLogger_Info(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	logger.Info("info message", nil)

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}
	if logs.All()[0].Level != zapcore.InfoLevel {
		t.Errorf("want InfoLevel, got %v", logs.All()[0].Level)
	}
	if logs.All()[0].Message != "info message" {
		t.Errorf("want message 'info message', got %q", logs.All()[0].Message)
	}
}

func TestNewZapLogger_Warn(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	logger.Warn("warn message", nil)

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}
	if logs.All()[0].Level != zapcore.WarnLevel {
		t.Errorf("want WarnLevel, got %v", logs.All()[0].Level)
	}
}

func TestNewZapLogger_Error(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	logger.Error("error message", nil)

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}
	if logs.All()[0].Level != zapcore.ErrorLevel {
		t.Errorf("want ErrorLevel, got %v", logs.All()[0].Level)
	}
}

// ---------------------------------------------------------------------------
// fieldsFromMap type-switch branches
// ---------------------------------------------------------------------------

func TestFieldsFromMap_EmptyNil(t *testing.T) {
	t.Parallel()
	result := fieldsFromMap(nil)
	if result != nil {
		t.Errorf("nil input: want nil slice, got %v", result)
	}
	result = fieldsFromMap(map[string]any{})
	if result != nil {
		t.Errorf("empty map: want nil slice, got %v", result)
	}
}

func TestFieldsFromMap_StringType(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	logger.Info("msg", map[string]any{"mystr": "hello"})

	entry := logs.All()[0]
	f, ok := findField(entry.Context, "mystr")
	if !ok {
		t.Fatal("field 'mystr' not found in logged context")
	}
	if f.Type != zapcore.StringType {
		t.Errorf("want StringType, got %v", f.Type)
	}
	if f.String != "hello" {
		t.Errorf("want 'hello', got %q", f.String)
	}
}

func TestFieldsFromMap_IntType(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	logger.Info("msg", map[string]any{"myint": 42})

	entry := logs.All()[0]
	f, ok := findField(entry.Context, "myint")
	if !ok {
		t.Fatal("field 'myint' not found")
	}
	if f.Type != zapcore.Int64Type {
		t.Errorf("want Int64Type, got %v", f.Type)
	}
	if f.Integer != 42 {
		t.Errorf("want 42, got %d", f.Integer)
	}
}

func TestFieldsFromMap_Int64Type(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	var v int64 = 99
	logger.Info("msg", map[string]any{"i64": v})

	entry := logs.All()[0]
	f, ok := findField(entry.Context, "i64")
	if !ok {
		t.Fatal("field 'i64' not found")
	}
	if f.Type != zapcore.Int64Type {
		t.Errorf("want Int64Type, got %v", f.Type)
	}
	if f.Integer != 99 {
		t.Errorf("want 99, got %d", f.Integer)
	}
}

func TestFieldsFromMap_Float64Type(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	logger.Info("msg", map[string]any{"myfloat": 3.14})

	entry := logs.All()[0]
	f, ok := findField(entry.Context, "myfloat")
	if !ok {
		t.Fatal("field 'myfloat' not found")
	}
	if f.Type != zapcore.Float64Type {
		t.Errorf("want Float64Type, got %v", f.Type)
	}
}

func TestFieldsFromMap_BoolType(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	logger.Info("msg", map[string]any{"flag": true})

	entry := logs.All()[0]
	f, ok := findField(entry.Context, "flag")
	if !ok {
		t.Fatal("field 'flag' not found")
	}
	if f.Type != zapcore.BoolType {
		t.Errorf("want BoolType, got %v", f.Type)
	}
	if f.Integer != 1 {
		t.Errorf("want integer=1 for true bool, got %d", f.Integer)
	}
}

func TestFieldsFromMap_ErrorType(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	sentinel := errors.New("boom")
	logger.Error("msg", map[string]any{"err": sentinel})

	entry := logs.All()[0]
	// zap.NamedError stores under the given key with type ErrorType.
	f, ok := findField(entry.Context, "err")
	if !ok {
		t.Fatal("field 'err' not found")
	}
	if f.Type != zapcore.ErrorType {
		t.Errorf("want ErrorType, got %v", f.Type)
	}
}

func TestFieldsFromMap_DefaultAny(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	// []int is not handled by any specific branch, falls to default zap.Any.
	logger.Info("msg", map[string]any{"slice": []int{1, 2, 3}})

	entry := logs.All()[0]
	_, ok := findField(entry.Context, "slice")
	if !ok {
		t.Fatal("field 'slice' not found — default Any branch not hit")
	}
}

func TestFieldsFromMap_MultipleFields(t *testing.T) {
	t.Parallel()
	zapL, logs := newObservedLogger()
	logger := NewZapLogger(zapL)

	logger.Info("multi", map[string]any{
		"s": "text",
		"i": 7,
		"b": false,
	})

	entry := logs.All()[0]
	if len(entry.Context) != 3 {
		t.Fatalf("want 3 fields, got %d", len(entry.Context))
	}
}
