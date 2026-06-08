package cache

import (
	"container/list"
	"sync"
	"time"
)

// entry is the value stored in the LRU list and map.
type entry struct {
	key       string
	value     []byte
	expiresAt time.Time // zero means no expiry
}

// MemoryCache is a bounded LRU in-memory cache with optional TTL expiry.
// When the cache is at capacity, the least-recently-used entry is evicted
// before storing the new one. All operations are safe for concurrent use.
//
// Create with NewMemoryCache; the zero value is not usable.
type MemoryCache struct {
	mu      sync.RWMutex
	maxSize int
	items   map[string]*list.Element // key → list element holding *entry
	lru     *list.List               // front = most-recently used
}

// NewMemoryCache returns a MemoryCache with the given maximum number of entries.
// maxSize must be >= 1; values <= 0 are clamped to 1.
func NewMemoryCache(maxSize int) *MemoryCache {
	if maxSize <= 0 {
		maxSize = 1
	}
	return &MemoryCache{
		maxSize: maxSize,
		items:   make(map[string]*list.Element, maxSize),
		lru:     list.New(),
	}
}

// Get returns the cached value for key and true if the entry exists and has
// not expired. A stale (expired) entry is removed and (nil, false) is returned.
// Get promotes the entry to most-recently-used on a hit.
func (c *MemoryCache) Get(key string) ([]byte, bool) {
	// Fast path: read lock to check existence before acquiring the write lock.
	c.mu.RLock()
	_, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}

	// Promote to MRU and check expiry under write lock.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Re-check after acquiring write lock (another goroutine may have evicted).
	var el *list.Element
	el, ok = c.items[key]
	if !ok {
		return nil, false
	}

	e := el.Value.(*entry)

	// Check expiry.
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		c.removeElement(el)
		return nil, false
	}

	// Promote to front (most recently used).
	c.lru.MoveToFront(el)
	return e.value, true
}

// Set stores value under key with the given TTL. If key already exists, its
// value and expiry are updated and the entry is promoted to most-recently-used.
// When the cache is at capacity and key is new, the least-recently-used entry
// is evicted first. A zero or negative TTL means no expiry.
func (c *MemoryCache) Set(key string, value []byte, ttl time.Duration) {
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry.
	if el, ok := c.items[key]; ok {
		c.lru.MoveToFront(el)
		e := el.Value.(*entry)
		e.value = value
		e.expiresAt = expiresAt
		return
	}

	// Evict LRU if at capacity.
	if c.lru.Len() >= c.maxSize {
		c.removeElement(c.lru.Back())
	}

	// Insert at front.
	e := &entry{key: key, value: value, expiresAt: expiresAt}
	el := c.lru.PushFront(e)
	c.items[key] = el
}

// Delete removes the entry for key. No-op if key is absent.
func (c *MemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeElement(el)
	}
}

// Flush removes all entries from the cache.
func (c *MemoryCache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lru.Init()
	c.items = make(map[string]*list.Element, c.maxSize)
}

// Len returns the number of entries currently in the cache.
// Safe for concurrent use.
func (c *MemoryCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len()
}

// removeElement removes el from both the list and the map.
// Must be called with c.mu held for writing.
func (c *MemoryCache) removeElement(el *list.Element) {
	if el == nil {
		return
	}
	c.lru.Remove(el)
	e := el.Value.(*entry)
	delete(c.items, e.key)
}
