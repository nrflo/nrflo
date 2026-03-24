package service

import (
	"be/internal/clock"
	"be/internal/db"
)

// GlobalSettingsService provides access to global config key-value store.
type GlobalSettingsService struct {
	pool *db.Pool
	clock clock.Clock
}

// NewGlobalSettingsService creates a new GlobalSettingsService.
func NewGlobalSettingsService(pool *db.Pool, clk clock.Clock) *GlobalSettingsService {
	return &GlobalSettingsService{pool: pool, clock: clk}
}

// Get returns the value for a config key. Returns "" if not found.
func (s *GlobalSettingsService) Get(key string) (string, error) {
	return s.pool.GetConfig(key)
}

// Set upserts a config key-value pair.
func (s *GlobalSettingsService) Set(key, value string) error {
	return s.pool.SetConfig(key, value)
}
