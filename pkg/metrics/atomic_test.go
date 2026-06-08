package metrics

import (
	"sync"
	"testing"
	"time"
)

func TestAtomicCollector_BasicCounts(t *testing.T) {
	t.Parallel()
	c := NewAtomicCollector()

	c.IncRequests("GET", "/foods")
	c.IncRequests("GET", "/foods")
	c.IncFailures("GET", "/foods", 500)
	c.AddBytesSent(100)
	c.AddBytesRecv(200)
	c.IncActiveConnections()
	c.IncActiveConnections()
	c.DecActiveConnections()

	snap := c.Snapshot()

	if snap.RequestsTotal != 2 {
		t.Errorf("RequestsTotal: got %d, want 2", snap.RequestsTotal)
	}
	if snap.FailuresTotal != 1 {
		t.Errorf("FailuresTotal: got %d, want 1", snap.FailuresTotal)
	}
	if snap.BytesSent != 100 {
		t.Errorf("BytesSent: got %d, want 100", snap.BytesSent)
	}
	if snap.BytesRecv != 200 {
		t.Errorf("BytesRecv: got %d, want 200", snap.BytesRecv)
	}
	if snap.ActiveConnections != 1 {
		t.Errorf("ActiveConnections: got %d, want 1", snap.ActiveConnections)
	}
}

func TestAtomicCollector_DurationHistogram(t *testing.T) {
	t.Parallel()
	c := NewAtomicCollector()

	// Observations: 3ms (→ ≤5ms bucket), 20ms (→ ≤25ms bucket), 15000ms (→ overflow, > max boundary 10000ms)
	c.ObserveDuration("GET", "/a", 3*time.Millisecond)
	c.ObserveDuration("GET", "/a", 20*time.Millisecond)
	c.ObserveDuration("GET", "/a", 15000*time.Millisecond)

	snap := c.Snapshot()
	if len(snap.DurationHistogram) == 0 {
		t.Fatal("expected non-empty DurationHistogram")
	}

	// Find ≤5ms bucket.
	fiveMs := findBucket(snap.DurationHistogram, 5)
	if fiveMs == nil {
		t.Fatal("≤5ms bucket not found in histogram")
	}
	if fiveMs.Count != 1 {
		t.Errorf("≤5ms bucket: got %d, want 1", fiveMs.Count)
	}

	// Find ≤25ms bucket.
	twentyFiveMs := findBucket(snap.DurationHistogram, 25)
	if twentyFiveMs == nil {
		t.Fatal("≤25ms bucket not found")
	}
	if twentyFiveMs.Count != 1 {
		t.Errorf("≤25ms bucket: got %d, want 1", twentyFiveMs.Count)
	}

	// Find overflow bucket (UpperMs == -1).
	overflow := findBucket(snap.DurationHistogram, -1)
	if overflow == nil {
		t.Fatal("overflow bucket not found")
	}
	if overflow.Count != 1 {
		t.Errorf("overflow bucket: got %d, want 1", overflow.Count)
	}
}

func findBucket(buckets []BucketCount, upperMs int64) *BucketCount {
	for i := range buckets {
		if buckets[i].UpperMs == upperMs {
			return &buckets[i]
		}
	}
	return nil
}

// TestAtomicCollector_ConcurrentIncrements verifies race-freedom under concurrent
// mutation. Run with -race to exercise the race detector.
func TestAtomicCollector_ConcurrentIncrements(t *testing.T) {
	t.Parallel()
	c := NewAtomicCollector()

	const goroutines = 100
	const increments = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < increments; j++ {
				c.IncRequests("GET", "/test")
				c.IncFailures("GET", "/test", 500)
				c.AddBytesSent(1)
				c.AddBytesRecv(1)
				c.IncActiveConnections()
				c.DecActiveConnections()
				c.ObserveDuration("GET", "/test", time.Duration(j)*time.Millisecond)
			}
		}()
	}
	wg.Wait()

	snap := c.Snapshot()
	const expected = int64(goroutines * increments)

	if snap.RequestsTotal != expected {
		t.Errorf("RequestsTotal: got %d, want %d", snap.RequestsTotal, expected)
	}
	if snap.FailuresTotal != expected {
		t.Errorf("FailuresTotal: got %d, want %d", snap.FailuresTotal, expected)
	}
	if snap.BytesSent != expected {
		t.Errorf("BytesSent: got %d, want %d", snap.BytesSent, expected)
	}
	if snap.BytesRecv != expected {
		t.Errorf("BytesRecv: got %d, want %d", snap.BytesRecv, expected)
	}
	// ActiveConnections net should be zero (equal inc/dec).
	if snap.ActiveConnections != 0 {
		t.Errorf("ActiveConnections: got %d, want 0", snap.ActiveConnections)
	}

	// All observations must be accounted for across histogram buckets.
	var totalObs int64
	for _, b := range snap.DurationHistogram {
		totalObs += b.Count
	}
	if totalObs != expected {
		t.Errorf("histogram total: got %d, want %d", totalObs, expected)
	}
}

func TestNoopCollector(t *testing.T) {
	t.Parallel()
	var nc NoopCollector
	nc.IncRequests("GET", "/x")
	nc.IncFailures("GET", "/x", 404)
	nc.ObserveDuration("GET", "/x", time.Second)
	nc.IncActiveConnections()
	nc.DecActiveConnections()
	nc.AddBytesSent(42)
	nc.AddBytesRecv(42)
	snap := nc.Snapshot()
	if snap.RequestsTotal != 0 || snap.FailuresTotal != 0 || snap.BytesSent != 0 {
		t.Error("NoopCollector.Snapshot must return zero values")
	}
}
