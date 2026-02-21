package usagelimits

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/logger"
	"be/internal/model"
)

const (
	preferenceKey      = "usage_limits"
	stalenessThreshold = 30 * time.Minute
)

// Store abstracts persistence for usage limits (implemented by PreferencesService).
type Store interface {
	Get(name string) (*model.Preference, error)
	Set(name, value string) error
}

// Cache provides thread-safe storage for cached usage limits data
// with optional DB persistence via a Store.
type Cache struct {
	mu    sync.RWMutex
	data  *UsageLimits
	store Store
	clock clock.Clock
}

// NewCache creates a new cache. Pass nil store/clk to disable persistence.
func NewCache(store Store, clk clock.Clock) *Cache {
	return &Cache{store: store, clock: clk}
}

// Get returns the cached data, or nil if not yet populated.
func (c *Cache) Get() *UsageLimits {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

// Set updates the cached data and asynchronously persists to DB if a store is configured.
func (c *Cache) Set(data *UsageLimits) {
	c.mu.Lock()
	c.data = data
	c.mu.Unlock()

	if c.store == nil || data == nil {
		return
	}

	go func() {
		b, err := json.Marshal(data)
		if err != nil {
			logger.Info(context.Background(), "usage-limits: failed to marshal for persistence", "error", err)
			return
		}
		if err := c.store.Set(preferenceKey, string(b)); err != nil {
			logger.Info(context.Background(), "usage-limits: failed to persist to DB", "error", err)
		}
	}()
}

// LoadFromDB attempts to load usage limits from the DB store.
// Returns true if fresh data (updated within 30 minutes) was loaded into cache.
func (c *Cache) LoadFromDB() bool {
	if c.store == nil {
		return false
	}

	pref, err := c.store.Get(preferenceKey)
	if err != nil {
		logger.Info(context.Background(), "usage-limits: failed to load from DB", "error", err)
		return false
	}
	if pref == nil {
		return false
	}

	age := c.clock.Now().Sub(pref.UpdatedAt)
	if age > stalenessThreshold {
		logger.Info(context.Background(), "usage-limits: DB data stale, skipping", "age", age)
		return false
	}

	var data UsageLimits
	if err := json.Unmarshal([]byte(pref.Value), &data); err != nil {
		logger.Info(context.Background(), "usage-limits: failed to unmarshal DB data", "error", err)
		return false
	}

	c.mu.Lock()
	c.data = &data
	c.mu.Unlock()

	return true
}
