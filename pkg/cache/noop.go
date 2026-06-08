package cache

import "time"

// NoopCache is a no-operation Cache implementation that stores nothing.
// It is the default used when the caller does not supply a Cache in Options.
// All operations are safe for concurrent use and have no side effects.
type NoopCache struct{}

// Get always returns nil, false.
func (NoopCache) Get(_ string) ([]byte, bool) { return nil, false }

// Set discards the value.
func (NoopCache) Set(_ string, _ []byte, _ time.Duration) {}

// Delete is a no-op.
func (NoopCache) Delete(_ string) {}

// Flush is a no-op.
func (NoopCache) Flush() {}
