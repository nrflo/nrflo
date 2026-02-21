package service

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// PreferencesService handles global server preferences.
type PreferencesService struct {
	clock clock.Clock
	pool  *db.Pool
}

// NewPreferencesService creates a new preferences service.
func NewPreferencesService(pool *db.Pool, clk clock.Clock) *PreferencesService {
	return &PreferencesService{pool: pool, clock: clk}
}

// Get retrieves a preference by name. Returns (nil, nil) if not found.
func (s *PreferencesService) Get(name string) (*model.Preference, error) {
	pref := &model.Preference{}
	var createdAt, updatedAt string

	err := s.pool.QueryRow(
		"SELECT name, value, created_at, updated_at FROM preferences WHERE name = ?",
		name,
	).Scan(&pref.Name, &pref.Value, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get preference %q: %w", name, err)
	}

	pref.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	pref.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

	return pref, nil
}

// Set upserts a preference. On insert, sets both created_at and updated_at.
// On conflict, updates value and updated_at while preserving created_at.
func (s *PreferencesService) Set(name, value string) error {
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.pool.Exec(`
		INSERT INTO preferences (name, value, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		name, value, now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to set preference %q: %w", name, err)
	}
	return nil
}
