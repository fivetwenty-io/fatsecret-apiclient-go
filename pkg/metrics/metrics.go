package metrics

import "time"

// Collector is the observability interface for HTTP round-trip metrics.
// Core transport depends only on this interface; the atomic implementation is
// optional. All methods must be safe for concurrent use by multiple goroutines.
type Collector interface {
	// IncRequests increments the total request counter for the given HTTP method
	// and path. Called once per outbound request attempt (including retries).
	IncRequests(method, path string)

	// IncFailures increments the failure counter for the given method, path, and
	// HTTP status code. Called when a request completes with a non-2xx status or
	// a transport-level error (statusCode 0 for transport errors).
	IncFailures(method, path string, statusCode int)

	// ObserveDuration records the round-trip duration for the given method and
	// path. Called after the full response body is read (or an error occurs).
	ObserveDuration(method, path string, d time.Duration)

	// IncActiveConnections increments the gauge of in-flight requests.
	IncActiveConnections()

	// DecActiveConnections decrements the gauge of in-flight requests.
	DecActiveConnections()

	// AddBytesSent adds n to the cumulative bytes-sent counter.
	AddBytesSent(n int64)

	// AddBytesRecv adds n to the cumulative bytes-received counter.
	AddBytesRecv(n int64)

	// Snapshot returns a consistent point-in-time copy of all collected metrics.
	Snapshot() Snapshot
}

// Snapshot is a point-in-time read of all metrics collected by a Collector.
// Fields are plain values; no synchronisation is required after construction.
type Snapshot struct {
	// RequestsTotal is the cumulative number of outbound request attempts.
	RequestsTotal int64

	// FailuresTotal is the cumulative number of failed requests.
	FailuresTotal int64

	// ActiveConnections is the current number of in-flight requests.
	ActiveConnections int64

	// BytesSent is the cumulative number of bytes sent in request bodies.
	BytesSent int64

	// BytesRecv is the cumulative number of bytes received in response bodies.
	BytesRecv int64

	// DurationHistogram holds the per-bucket observation counts for round-trip
	// durations. Buckets use upper-bound millisecond boundaries defined by the
	// implementation. The final bucket (index len-1) is the overflow/+Inf bucket.
	DurationHistogram []BucketCount
}

// BucketCount is one bucket in a duration histogram.
type BucketCount struct {
	// UpperMs is the upper boundary of this bucket in milliseconds.
	// A value of -1 indicates the overflow (+Inf) bucket.
	UpperMs int64

	// Count is the number of observations falling in this bucket
	// (i.e., duration <= UpperMs ms, exclusive of lower-bound bucket).
	Count int64
}
