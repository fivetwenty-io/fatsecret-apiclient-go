// Package cache provides the Cache interface and built-in implementations for
// caching GET response bodies. The default NoopCache performs no storage.
// MemoryCache provides a bounded LRU in-memory store with TTL expiry.
//
// Consumers plug a Cache into client.Options.Cache; the internal HTTP middleware
// checks the cache before executing the request and stores responses on miss.
package cache
