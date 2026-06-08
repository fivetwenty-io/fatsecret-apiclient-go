package cache

import "time"

// Cache is a simple key-value store for GET response bodies.
// The key is typically a canonical URL with sorted query parameters.
// Only GET requests should be cached; callers are responsible for key construction.
// Implementations must be safe for concurrent use by multiple goroutines.
type Cache interface {
	// Get returns the cached value and true if found, or nil and false otherwise.
	Get(key string) ([]byte, bool)

	// Set stores value under key with the given TTL. A zero or negative TTL
	// means the entry never expires (implementation-defined; MemoryCache treats
	// zero TTL as no expiry).
	Set(key string, value []byte, ttl time.Duration)

	// Delete removes the entry for key. No-op if key is absent.
	Delete(key string)

	// Flush removes all entries from the cache.
	Flush()
}
