package usagelimits

import "sync"

// Cache provides thread-safe storage for cached usage limits data.
type Cache struct {
	mu   sync.RWMutex
	data *UsageLimits
}

// NewCache creates a new empty cache.
func NewCache() *Cache {
	return &Cache{}
}

// Get returns the cached data, or nil if not yet populated.
func (c *Cache) Get() *UsageLimits {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

// Set updates the cached data.
func (c *Cache) Set(data *UsageLimits) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = data
}
