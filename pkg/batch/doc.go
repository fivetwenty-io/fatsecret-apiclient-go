// Package batch provides a generic fan-out helper for executing a function
// concurrently over a slice of inputs with a bounded concurrency limit.
// Results are returned in the same order as the input slice. Each result
// carries either a value or an error; a failure on one item does not cancel
// the remaining items. Context cancellation stops launching new goroutines
// and propagates the context error to all items that were not yet started.
package batch
