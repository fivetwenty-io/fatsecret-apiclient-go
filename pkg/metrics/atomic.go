package metrics

import (
	"sync/atomic"
	"time"
)

// defaultBucketUppersMs defines the upper boundaries (in milliseconds) of the
// duration histogram buckets used by AtomicCollector. The last value (-1)
// represents the overflow (+Inf) bucket.
var defaultBucketUppersMs = []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, -1}

// atomicBucket is a single histogram bucket backed by an atomic counter.
type atomicBucket struct {
	upperMs int64        // -1 means overflow/+Inf
	count   atomic.Int64 // number of observations in this bucket
}

// AtomicCollector is a dependency-free, lock-free metrics collector.
// Counters use sync/atomic.Int64 for all scalar metrics. The duration
// histogram uses per-bucket atomic counters; bucket selection scans the
// fixed boundary table (12 buckets) in O(1) amortised for typical latencies.
//
// All methods are safe for concurrent use. Create with NewAtomicCollector.
type AtomicCollector struct {
	requestsTotal   atomic.Int64
	failuresTotal   atomic.Int64
	activeConns     atomic.Int64
	bytesSent       atomic.Int64
	bytesRecv       atomic.Int64
	durationBuckets []atomicBucket
}

// NewAtomicCollector returns an AtomicCollector using the default histogram
// bucket boundaries. The returned collector is ready to use.
func NewAtomicCollector() *AtomicCollector {
	c := &AtomicCollector{
		durationBuckets: make([]atomicBucket, len(defaultBucketUppersMs)),
	}
	for i, upper := range defaultBucketUppersMs {
		c.durationBuckets[i].upperMs = upper
	}
	return c
}

// IncRequests increments the total requests counter. The method and path
// arguments are accepted to satisfy the Collector interface but are not
// stored (label cardinality is unbounded; use a Prometheus adapter for
// labeled metrics).
func (c *AtomicCollector) IncRequests(_, _ string) {
	c.requestsTotal.Add(1)
}

// IncFailures increments the failures counter. method, path, and statusCode
// are accepted for interface compatibility; only the count is stored.
func (c *AtomicCollector) IncFailures(_, _ string, _ int) {
	c.failuresTotal.Add(1)
}

// ObserveDuration records d in the appropriate histogram bucket by scanning
// the fixed boundary table from smallest to largest. The method and path
// arguments are accepted for interface compatibility.
func (c *AtomicCollector) ObserveDuration(_, _ string, d time.Duration) {
	ms := d.Milliseconds()
	for i := range c.durationBuckets {
		upper := c.durationBuckets[i].upperMs
		if upper == -1 || ms <= upper {
			c.durationBuckets[i].count.Add(1)
			return
		}
	}
	// Unreachable: the last bucket has upperMs == -1 (overflow), which always
	// matches. Guard in case the bucket table is empty.
	if len(c.durationBuckets) > 0 {
		c.durationBuckets[len(c.durationBuckets)-1].count.Add(1)
	}
}

// IncActiveConnections atomically increments the in-flight request gauge.
func (c *AtomicCollector) IncActiveConnections() {
	c.activeConns.Add(1)
}

// DecActiveConnections atomically decrements the in-flight request gauge.
func (c *AtomicCollector) DecActiveConnections() {
	c.activeConns.Add(-1)
}

// AddBytesSent atomically adds n to the bytes-sent counter.
func (c *AtomicCollector) AddBytesSent(n int64) {
	c.bytesSent.Add(n)
}

// AddBytesRecv atomically adds n to the bytes-received counter.
func (c *AtomicCollector) AddBytesRecv(n int64) {
	c.bytesRecv.Add(n)
}

// Snapshot returns a consistent point-in-time copy of all collected metrics.
// Each field is read via a single atomic load; the snapshot is not a
// transactional read across all counters — brief skew between fields is
// possible under concurrent mutation, which is acceptable for operational
// dashboards. Use an external mutex if strict consistency is required.
func (c *AtomicCollector) Snapshot() Snapshot {
	buckets := make([]BucketCount, len(c.durationBuckets))
	for i := range c.durationBuckets {
		buckets[i] = BucketCount{
			UpperMs: c.durationBuckets[i].upperMs,
			Count:   c.durationBuckets[i].count.Load(),
		}
	}
	return Snapshot{
		RequestsTotal:     c.requestsTotal.Load(),
		FailuresTotal:     c.failuresTotal.Load(),
		ActiveConnections: c.activeConns.Load(),
		BytesSent:         c.bytesSent.Load(),
		BytesRecv:         c.bytesRecv.Load(),
		DurationHistogram: buckets,
	}
}
