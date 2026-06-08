// Package metrics defines the Collector interface and built-in implementations
// for instrumenting HTTP interactions. The default NoopCollector discards all
// observations. AtomicCollector provides a dependency-free, lock-free counter
// implementation backed by sync/atomic, with a fixed-boundary duration histogram.
//
// Consumers plug a Collector into client.Options.MetricsCollector; the internal
// HTTP middleware calls the collector around each round-trip.
package metrics
