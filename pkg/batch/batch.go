package batch

import (
	"context"
	"sync"
)

// Result holds the outcome of processing one input item.
// Exactly one of Value or Err is meaningful: if Err is non-nil, Value is the
// zero value for Resp; if Err is nil, Value holds the successful result.
type Result[Resp any] struct {
	Value Resp
	Err   error
}

// Do executes fn for each element of items concurrently with at most concurrency
// goroutines running simultaneously. Results are returned in the same order as
// items regardless of completion order.
//
// Errors from fn are per-item and do not affect other items. If ctx is cancelled
// or its deadline is exceeded before a goroutine is launched for an item, that
// item's Result.Err is set to ctx.Err() without calling fn. Items already
// running are not interrupted, but fn receives the same ctx so well-behaved
// functions will honour cancellation themselves.
//
// Concurrency is clamped to max(1, min(concurrency, len(items))) to avoid
// spawning more goroutines than useful. A concurrency value <= 0 is treated as 1.
func Do[Req any, Resp any](
	ctx context.Context,
	items []Req,
	concurrency int,
	fn func(context.Context, Req) (Resp, error),
) []Result[Resp] {
	n := len(items)
	if n == 0 {
		return []Result[Resp]{}
	}

	// Clamp concurrency.
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > n {
		concurrency = n
	}

	results := make([]Result[Resp], n)

	// Semaphore channel: limits goroutines in flight.
	sem := make(chan struct{}, concurrency)

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		i := i // capture loop variable
		item := items[i]

		// Check for cancellation before acquiring the semaphore slot.
		// This avoids launching work that will never complete meaningfully.
		if err := ctx.Err(); err != nil {
			results[i] = Result[Resp]{Err: err}
			wg.Done()
			continue
		}

		go func() {
			defer wg.Done()

			// Acquire semaphore — blocks until a slot is free.
			// Also watch for context cancellation while waiting.
			select {
			case sem <- struct{}{}:
				// slot acquired
			case <-ctx.Done():
				results[i] = Result[Resp]{Err: ctx.Err()}
				return
			}

			// Release slot when done.
			defer func() { <-sem }()

			// Execute the user function, passing the original context so fn can
			// honour cancellation on its own.
			val, err := fn(ctx, item)
			results[i] = Result[Resp]{Value: val, Err: err}
		}()
	}

	wg.Wait()
	return results
}
