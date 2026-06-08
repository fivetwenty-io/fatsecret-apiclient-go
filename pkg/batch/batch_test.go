package batch

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// identity returns the input as-is; used to verify order preservation.
func identity(_ context.Context, n int) (int, error) {
	return n, nil
}

func TestDo_PreservesOrder(t *testing.T) {
	t.Parallel()
	const n = 100
	items := make([]int, n)
	for i := range items {
		items[i] = i
	}

	results := Do(context.Background(), items, 10, identity)

	if len(results) != n {
		t.Fatalf("len(results): got %d, want %d", len(results), n)
	}
	for i, r := range results {
		if r.Err != nil {
			t.Errorf("results[%d].Err = %v, want nil", i, r.Err)
		}
		if r.Value != i {
			t.Errorf("results[%d].Value = %d, want %d", i, r.Value, i)
		}
	}
}

func TestDo_ErrorPerItem(t *testing.T) {
	t.Parallel()

	items := []int{0, 1, 2, 3, 4}
	sentinel := errors.New("fail")

	fn := func(_ context.Context, n int) (int, error) {
		if n%2 != 0 {
			return 0, sentinel
		}
		return n, nil
	}

	results := Do(context.Background(), items, 3, fn)

	for i, r := range results {
		if items[i]%2 != 0 {
			if !errors.Is(r.Err, sentinel) {
				t.Errorf("results[%d]: expected sentinel error, got %v", i, r.Err)
			}
		} else {
			if r.Err != nil {
				t.Errorf("results[%d]: unexpected error %v", i, r.Err)
			}
			if r.Value != items[i] {
				t.Errorf("results[%d]: value %d, want %d", i, r.Value, items[i])
			}
		}
	}
}

func TestDo_EmptyInput(t *testing.T) {
	t.Parallel()
	results := Do[int, int](context.Background(), nil, 5, func(_ context.Context, n int) (int, error) {
		return n, nil
	})
	if len(results) != 0 {
		t.Fatalf("expected empty results slice, got len %d", len(results))
	}
}

func TestDo_ConcurrencyLimit(t *testing.T) {
	t.Parallel()
	const concurrency = 3
	const n = 20

	var inFlight atomic.Int32
	var peak atomic.Int32

	items := make([]int, n)
	fn := func(_ context.Context, _ int) (int, error) {
		cur := inFlight.Add(1)
		defer inFlight.Add(-1)
		// Track peak concurrency.
		for {
			old := peak.Load()
			if cur <= old {
				break
			}
			if peak.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		return 0, nil
	}

	Do(context.Background(), items, concurrency, fn)

	if p := peak.Load(); p > int32(concurrency) {
		t.Errorf("peak concurrency %d exceeded limit %d", p, concurrency)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately.
	cancel()

	const n = 50
	items := make([]int, n)
	var called atomic.Int32

	fn := func(_ context.Context, _ int) (int, error) {
		called.Add(1)
		return 0, nil
	}

	results := Do(ctx, items, 1, fn)

	if len(results) != n {
		t.Fatalf("expected %d results, got %d", n, len(results))
	}

	// At least some items must carry ctx.Err(); exact count depends on timing.
	var ctxErrCount int
	for _, r := range results {
		if errors.Is(r.Err, context.Canceled) {
			ctxErrCount++
		}
	}
	if ctxErrCount == 0 {
		t.Error("expected at least one item cancelled by context, got none")
	}
}

func TestDo_ConcurrencyClampedToItemCount(t *testing.T) {
	t.Parallel()
	// concurrency > n should not panic or deadlock.
	items := []int{1, 2, 3}
	results := Do(context.Background(), items, 100, identity)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestDo_ConcurrencyZeroClamped(t *testing.T) {
	t.Parallel()
	// concurrency=0 must be treated as 1 and not deadlock.
	items := []int{1, 2}
	results := Do(context.Background(), items, 0, identity)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestDo_LargeOrderPreservation(t *testing.T) {
	t.Parallel()
	const n = 1000
	items := make([]int, n)
	for i := range items {
		items[i] = i
	}

	results := Do(context.Background(), items, 50, func(_ context.Context, v int) (string, error) {
		return fmt.Sprintf("item-%d", v), nil
	})

	for i, r := range results {
		want := fmt.Sprintf("item-%d", i)
		if r.Value != want {
			t.Fatalf("results[%d].Value = %q, want %q", i, r.Value, want)
		}
	}
}
