package service

import (
	"strconv"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// ClaudeLimits holds Claude API rate limit state.
type ClaudeLimits struct {
	FiveHourUsedPct  float64 // -1 when unset
	FiveHourResetsAt string
	SevenDayUsedPct  float64 // -1 when unset
	SevenDayResetsAt string
	UpdatedAt        string
}

// ClaudeLimitsService persists and retrieves Claude rate limit data.
type ClaudeLimitsService struct {
	pool *db.Pool
	clk  clock.Clock
}

// NewClaudeLimitsService creates a new ClaudeLimitsService.
func NewClaudeLimitsService(pool *db.Pool, clk clock.Clock) *ClaudeLimitsService {
	return &ClaudeLimitsService{pool: pool, clk: clk}
}

// Update writes all 5 config keys for Claude rate limits.
func (s *ClaudeLimitsService) Update(limits ClaudeLimits) error {
	updatedAt := s.clk.Now().UTC().Format(time.RFC3339)
	pairs := []struct{ k, v string }{
		{"claude_5h_used_pct", strconv.FormatFloat(limits.FiveHourUsedPct, 'f', -1, 64)},
		{"claude_5h_resets_at", limits.FiveHourResetsAt},
		{"claude_weekly_used_pct", strconv.FormatFloat(limits.SevenDayUsedPct, 'f', -1, 64)},
		{"claude_weekly_resets_at", limits.SevenDayResetsAt},
		{"claude_limits_updated_at", updatedAt},
	}
	for _, p := range pairs {
		if err := s.pool.SetConfig(p.k, p.v); err != nil {
			return err
		}
	}
	return nil
}

// Get reads all 5 config keys, returning zero-value struct (pcts = -1) when not set.
func (s *ClaudeLimitsService) Get() (ClaudeLimits, error) {
	zero := ClaudeLimits{FiveHourUsedPct: -1, SevenDayUsedPct: -1}

	fiveHourPctStr, err := s.pool.GetConfig("claude_5h_used_pct")
	if err != nil {
		return zero, err
	}
	fiveHourResetsAt, err := s.pool.GetConfig("claude_5h_resets_at")
	if err != nil {
		return zero, err
	}
	sevenDayPctStr, err := s.pool.GetConfig("claude_weekly_used_pct")
	if err != nil {
		return zero, err
	}
	sevenDayResetsAt, err := s.pool.GetConfig("claude_weekly_resets_at")
	if err != nil {
		return zero, err
	}
	updatedAt, err := s.pool.GetConfig("claude_limits_updated_at")
	if err != nil {
		return zero, err
	}

	fiveHourPct := -1.0
	if fiveHourPctStr != "" {
		if v, parseErr := strconv.ParseFloat(fiveHourPctStr, 64); parseErr == nil {
			fiveHourPct = v
		}
	}
	sevenDayPct := -1.0
	if sevenDayPctStr != "" {
		if v, parseErr := strconv.ParseFloat(sevenDayPctStr, 64); parseErr == nil {
			sevenDayPct = v
		}
	}

	return ClaudeLimits{
		FiveHourUsedPct:  fiveHourPct,
		FiveHourResetsAt: fiveHourResetsAt,
		SevenDayUsedPct:  sevenDayPct,
		SevenDayResetsAt: sevenDayResetsAt,
		UpdatedAt:        updatedAt,
	}, nil
}
