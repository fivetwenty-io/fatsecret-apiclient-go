package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMemoryCache_GetSetBasic(t *testing.T) {
	t.Parallel()
	c := NewMemoryCache(10)

	// Miss on empty cache.
	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected miss on empty cache")
	}

	// Set then get.
	c.Set("k1", []byte("hello"), 0)
	val, ok := c.Get("k1")
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if string(val) != "hello" {
		t.Fatalf("value mismatch: got %q, want %q", val, "hello")
	}
}

func TestMemoryCache_LRUEviction(t *testing.T) {
	t.Parallel()

	// maxSize = 3; fill with k1, k2, k3; then access k1 and k2 (promoting them),
	// then add k4 — k3 should be evicted (LRU).
	c := NewMemoryCache(3)
	c.Set("k1", []byte("1"), 0)
	c.Set("k2", []byte("2"), 0)
	c.Set("k3", []byte("3"), 0)

	// Access k1 and k2 to make k3 the LRU.
	c.Get("k1")
	c.Get("k2")

	// Insert k4 — k3 must be evicted.
	c.Set("k4", []byte("4"), 0)

	if _, ok := c.Get("k3"); ok {
		t.Error("k3 should have been evicted (LRU)")
	}
	if _, ok := c.Get("k1"); !ok {
		t.Error("k1 should still be present")
	}
	if _, ok := c.Get("k2"); !ok {
		t.Error("k2 should still be present")
	}
	if _, ok := c.Get("k4"); !ok {
		t.Error("k4 should be present")
	}
}

func TestMemoryCache_LRUEvictionOldest(t *testing.T) {
	t.Parallel()

	// maxSize = 2; fill with k1 then k2; add k3 — k1 (oldest / LRU) evicted.
	c := NewMemoryCache(2)
	c.Set("k1", []byte("1"), 0)
	c.Set("k2", []byte("2"), 0)
	c.Set("k3", []byte("3"), 0)

	if _, ok := c.Get("k1"); ok {
		t.Error("k1 should have been evicted")
	}
	if _, ok := c.Get("k2"); !ok {
		t.Error("k2 should still be present")
	}
	if _, ok := c.Get("k3"); !ok {
		t.Error("k3 should be present")
	}
}

func TestMemoryCache_UpdateExisting(t *testing.T) {
	t.Parallel()
	c := NewMemoryCache(5)
	c.Set("k", []byte("old"), 0)
	c.Set("k", []byte("new"), 0)
	val, ok := c.Get("k")
	if !ok {
		t.Fatal("expected hit")
	}
	if string(val) != "new" {
		t.Fatalf("expected updated value %q, got %q", "new", val)
	}
	if c.Len() != 1 {
		t.Fatalf("expected len 1 after update, got %d", c.Len())
	}
}

func TestMemoryCache_TTLExpiry(t *testing.T) {
	t.Parallel()
	c := NewMemoryCache(10)
	c.Set("exp", []byte("expiring"), 10*time.Millisecond)

	// Should hit immediately.
	if _, ok := c.Get("exp"); !ok {
		t.Fatal("expected hit before expiry")
	}

	// Wait for expiry.
	time.Sleep(20 * time.Millisecond)

	if _, ok := c.Get("exp"); ok {
		t.Error("expected miss after TTL expiry")
	}
}

func TestMemoryCache_Delete(t *testing.T) {
	t.Parallel()
	c := NewMemoryCache(5)
	c.Set("k", []byte("v"), 0)
	c.Delete("k")
	if _, ok := c.Get("k"); ok {
		t.Error("expected miss after Delete")
	}
	// Delete of absent key must not panic.
	c.Delete("absent")
}

func TestMemoryCache_Flush(t *testing.T) {
	t.Parallel()
	c := NewMemoryCache(5)
	for i := 0; i < 5; i++ {
		c.Set(fmt.Sprintf("k%d", i), []byte("v"), 0)
	}
	c.Flush()
	if c.Len() != 0 {
		t.Fatalf("expected empty cache after Flush, got len %d", c.Len())
	}
}

func TestMemoryCache_MaxSizeOne(t *testing.T) {
	t.Parallel()
	c := NewMemoryCache(1)
	c.Set("a", []byte("1"), 0)
	c.Set("b", []byte("2"), 0)
	if c.Len() != 1 {
		t.Fatalf("expected len 1, got %d", c.Len())
	}
	if _, ok := c.Get("a"); ok {
		t.Error("a should have been evicted by b")
	}
	if _, ok := c.Get("b"); !ok {
		t.Error("b should be present")
	}
}

func TestMemoryCache_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()
	c := NewMemoryCache(50)
	const goroutines = 50
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := fmt.Sprintf("key-%d-%d", g, i%10)
				c.Set(key, []byte(fmt.Sprintf("v%d", i)), 0)
				c.Get(key)
				if i%20 == 0 {
					c.Delete(key)
				}
			}
		}()
	}
	wg.Wait()
}

func TestNoopCache(t *testing.T) {
	t.Parallel()
	var nc NoopCache
	if _, ok := nc.Get("x"); ok {
		t.Error("NoopCache.Get must return false")
	}
	nc.Set("x", []byte("y"), time.Minute) // must not panic
	nc.Delete("x")
	nc.Flush()
}
