package metrics

import "time"

// NoopCollector is a no-operation Collector that discards all observations.
// It is the default used when the caller does not supply a MetricsCollector
// in Options. All operations are safe for concurrent use and have no side effects.
// Snapshot returns a zero-value Snapshot with an empty histogram.
type NoopCollector struct{}

// IncRequests discards the increment.
func (NoopCollector) IncRequests(_, _ string) {}

// IncFailures discards the increment.
func (NoopCollector) IncFailures(_, _ string, _ int) {}

// ObserveDuration discards the observation.
func (NoopCollector) ObserveDuration(_, _ string, _ time.Duration) {}

// IncActiveConnections is a no-op.
func (NoopCollector) IncActiveConnections() {}

// DecActiveConnections is a no-op.
func (NoopCollector) DecActiveConnections() {}

// AddBytesSent discards the value.
func (NoopCollector) AddBytesSent(_ int64) {}

// AddBytesRecv discards the value.
func (NoopCollector) AddBytesRecv(_ int64) {}

// Snapshot returns a zero Snapshot with a nil histogram slice.
func (NoopCollector) Snapshot() Snapshot { return Snapshot{} }
