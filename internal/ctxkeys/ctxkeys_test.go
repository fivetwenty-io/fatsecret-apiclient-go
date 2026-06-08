package ctxkeys

import (
	"context"
	"testing"
	"time"
)

// TestRetries_RoundTrip sets a retry count and reads it back.
func TestRetries_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := WithRetries(context.Background(), 5)
	got, ok := RetryCount(ctx)
	if !ok {
		t.Fatal("RetryCount: expected ok=true, got false")
	}
	if got != 5 {
		t.Fatalf("RetryCount: want 5, got %d", got)
	}
}

// TestRetries_NegativeValue negative values stored as-is.
func TestRetries_NegativeValue(t *testing.T) {
	t.Parallel()
	ctx := WithRetries(context.Background(), -3)
	got, ok := RetryCount(ctx)
	if !ok {
		t.Fatal("RetryCount: expected ok=true, got false")
	}
	if got != -3 {
		t.Fatalf("RetryCount: want -3, got %d", got)
	}
}

// TestRetries_Empty returns zero and false when no value set.
func TestRetries_Empty(t *testing.T) {
	t.Parallel()
	got, ok := RetryCount(context.Background())
	if ok {
		t.Fatal("RetryCount: expected ok=false on empty ctx, got true")
	}
	if got != 0 {
		t.Fatalf("RetryCount: want zero value 0, got %d", got)
	}
}

// TestRetryDelay_RoundTrip sets a delay and reads it back.
func TestRetryDelay_RoundTrip(t *testing.T) {
	t.Parallel()
	d := 250 * time.Millisecond
	ctx := WithRetryDelay(context.Background(), d)
	got, ok := RetryDelay(ctx)
	if !ok {
		t.Fatal("RetryDelay: expected ok=true, got false")
	}
	if got != d {
		t.Fatalf("RetryDelay: want %v, got %v", d, got)
	}
}

// TestRetryDelay_ZeroDuration zero duration stored and retrieved.
func TestRetryDelay_ZeroDuration(t *testing.T) {
	t.Parallel()
	ctx := WithRetryDelay(context.Background(), 0)
	got, ok := RetryDelay(ctx)
	if !ok {
		t.Fatal("RetryDelay: expected ok=true for zero duration, got false")
	}
	if got != 0 {
		t.Fatalf("RetryDelay: want 0, got %v", got)
	}
}

// TestRetryDelay_Empty returns zero and false when no value set.
func TestRetryDelay_Empty(t *testing.T) {
	t.Parallel()
	got, ok := RetryDelay(context.Background())
	if ok {
		t.Fatal("RetryDelay: expected ok=false on empty ctx, got true")
	}
	if got != 0 {
		t.Fatalf("RetryDelay: want zero duration, got %v", got)
	}
}

// TestLogging_RoundTrip_True sets logging=true and reads it back.
func TestLogging_RoundTrip_True(t *testing.T) {
	t.Parallel()
	ctx := WithLogging(context.Background(), true)
	got, ok := Logging(ctx)
	if !ok {
		t.Fatal("Logging: expected ok=true, got false")
	}
	if !got {
		t.Fatal("Logging: want true, got false")
	}
}

// TestLogging_RoundTrip_False sets logging=false and reads it back.
func TestLogging_RoundTrip_False(t *testing.T) {
	t.Parallel()
	ctx := WithLogging(context.Background(), false)
	got, ok := Logging(ctx)
	if !ok {
		t.Fatal("Logging: expected ok=true, got false")
	}
	if got {
		t.Fatal("Logging: want false, got true")
	}
}

// TestLogging_Empty returns false and false when no value set.
func TestLogging_Empty(t *testing.T) {
	t.Parallel()
	got, ok := Logging(context.Background())
	if ok {
		t.Fatal("Logging: expected ok=false on empty ctx, got true")
	}
	if got {
		t.Fatal("Logging: want zero value false, got true")
	}
}

// TestLogFields_RoundTrip stores fields and retrieves the same map identity.
func TestLogFields_RoundTrip(t *testing.T) {
	t.Parallel()
	fields := map[string]any{"key": "value", "n": 42}
	ctx := WithLogFields(context.Background(), fields)
	got, ok := LogFields(ctx)
	if !ok {
		t.Fatal("LogFields: expected ok=true, got false")
	}
	if got["key"] != "value" {
		t.Fatalf("LogFields: want key=value, got %v", got["key"])
	}
	if got["n"] != 42 {
		t.Fatalf("LogFields: want n=42, got %v", got["n"])
	}
}

// TestLogFields_NilMap nil input returns empty merged map, not nil.
func TestLogFields_NilMap(t *testing.T) {
	t.Parallel()
	ctx := WithLogFields(context.Background(), nil)
	got, ok := LogFields(ctx)
	if !ok {
		t.Fatal("LogFields: expected ok=true even for nil input, got false")
	}
	if got == nil {
		t.Fatal("LogFields: returned nil map, want non-nil empty map")
	}
	if len(got) != 0 {
		t.Fatalf("LogFields: want empty map, got %v", got)
	}
}

// TestLogFields_Merge later call merges and overwrites existing keys.
func TestLogFields_Merge(t *testing.T) {
	t.Parallel()
	ctx := WithLogFields(context.Background(), map[string]any{"a": "1", "b": "2"})
	ctx = WithLogFields(ctx, map[string]any{"b": "overwritten", "c": "3"})
	got, ok := LogFields(ctx)
	if !ok {
		t.Fatal("LogFields: expected ok=true, got false")
	}
	if got["a"] != "1" {
		t.Fatalf("LogFields merge: want a=1, got %v", got["a"])
	}
	if got["b"] != "overwritten" {
		t.Fatalf("LogFields merge: want b=overwritten, got %v", got["b"])
	}
	if got["c"] != "3" {
		t.Fatalf("LogFields merge: want c=3, got %v", got["c"])
	}
}

// TestLogFields_Empty returns nil map and false on empty ctx.
func TestLogFields_Empty(t *testing.T) {
	t.Parallel()
	got, ok := LogFields(context.Background())
	if ok {
		t.Fatal("LogFields: expected ok=false on empty ctx, got true")
	}
	if got != nil {
		t.Fatalf("LogFields: want nil, got %v", got)
	}
}

// TestForceRetry_True sets force=true and reads it back.
func TestForceRetry_True(t *testing.T) {
	t.Parallel()
	ctx := WithForceRetry(context.Background(), true)
	got, ok := ForceRetry(ctx)
	if !ok {
		t.Fatal("ForceRetry: expected ok=true, got false")
	}
	if !got {
		t.Fatal("ForceRetry: want true, got false")
	}
}

// TestForceRetry_False sets force=false and reads it back.
func TestForceRetry_False(t *testing.T) {
	t.Parallel()
	ctx := WithForceRetry(context.Background(), false)
	got, ok := ForceRetry(ctx)
	if !ok {
		t.Fatal("ForceRetry: expected ok=true, got false")
	}
	if got {
		t.Fatal("ForceRetry: want false, got true")
	}
}

// TestForceRetry_Empty returns false and false on empty ctx.
func TestForceRetry_Empty(t *testing.T) {
	t.Parallel()
	got, ok := ForceRetry(context.Background())
	if ok {
		t.Fatal("ForceRetry: expected ok=false on empty ctx, got true")
	}
	if got {
		t.Fatal("ForceRetry: want zero value false, got true")
	}
}
