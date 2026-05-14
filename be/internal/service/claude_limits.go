package service

import (
	"context"
	"strconv"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
)

const claudeLimitsEpsilon = 0.5

// UpdateResult is returned by ClaudeLimitsService.Update.
type UpdateResult struct {
	Changed bool
	Stored  ClaudeLimits
}

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

// Update writes non-sentinel fields, applying a per-window monotonic guard.
// Pct decreases within an active reset window are rejected (epsilon 0.5); resets_at
// changes or an elapsed window clear the guard. Returns UpdateResult{Changed: false}
// when nothing was written; does not bump claude_limits_updated_at in that case.
func (s *ClaudeLimitsService) Update(limits ClaudeLimits) (UpdateResult, error) {
	existing, err := s.Get()
	if err != nil {
		return UpdateResult{}, err
	}

	now := s.clk.Now().UTC()
	var pairs []struct{ k, v string }

	if limits.FiveHourUsedPct >= 0 {
		if s.acceptPct("5h", limits.FiveHourUsedPct, existing.FiveHourUsedPct, existing.FiveHourResetsAt, limits.FiveHourResetsAt, now) {
			pairs = append(pairs, struct{ k, v string }{"claude_5h_used_pct", strconv.FormatFloat(limits.FiveHourUsedPct, 'f', -1, 64)})
		}
	}
	if limits.FiveHourResetsAt != "" {
		pairs = append(pairs, struct{ k, v string }{"claude_5h_resets_at", limits.FiveHourResetsAt})
	}

	if limits.SevenDayUsedPct >= 0 {
		if s.acceptPct("7d", limits.SevenDayUsedPct, existing.SevenDayUsedPct, existing.SevenDayResetsAt, limits.SevenDayResetsAt, now) {
			pairs = append(pairs, struct{ k, v string }{"claude_weekly_used_pct", strconv.FormatFloat(limits.SevenDayUsedPct, 'f', -1, 64)})
		}
	}
	if limits.SevenDayResetsAt != "" {
		pairs = append(pairs, struct{ k, v string }{"claude_weekly_resets_at", limits.SevenDayResetsAt})
	}

	if len(pairs) == 0 {
		return UpdateResult{Changed: false, Stored: existing}, nil
	}

	pairs = append(pairs, struct{ k, v string }{"claude_limits_updated_at", now.Format(time.RFC3339)})
	for _, p := range pairs {
		if err := s.pool.SetConfig(p.k, p.v); err != nil {
			return UpdateResult{}, err
		}
	}

	refreshed, err := s.Get()
	if err != nil {
		return UpdateResult{}, err
	}
	return UpdateResult{Changed: true, Stored: refreshed}, nil
}

// acceptPct returns true if the new pct should be written for the given window.
func (s *ClaudeLimitsService) acceptPct(window string, newPct, existingPct float64, existingResetsAt, newResetsAt string, now time.Time) bool {
	if existingPct < 0 {
		return true
	}
	existingReset, err := time.Parse(time.RFC3339, existingResetsAt)
	if err != nil {
		return true
	}
	if !now.Before(existingReset) {
		return true
	}
	if newResetsAt != "" && newResetsAt != existingResetsAt {
		return true
	}
	if newPct >= existingPct-claudeLimitsEpsilon {
		return true
	}
	logger.Info(context.Background(), "claude limits: rejecting non-monotonic pct",
		"window", window, "old_pct", existingPct, "new_pct", newPct, "resets_at", existingResetsAt)
	return false
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
