package service

import (
	"context"
	"math"
	"strconv"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
)

const (
	claudeLimitsEpsilon      = 0.5
	downgradeConfirmDuration = 5 * time.Minute
)

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

// pendingDowngrade tracks how long a lower pct has been reported for a window
// while the monotonic guard rejected it. After downgradeConfirmDuration of the
// same (resetsAt, pct±epsilon) the value is accepted — handles Anthropic
// revising the percentage down (e.g. usage reconciliation).
type pendingDowngrade struct {
	resetsAt  string
	pct       float64
	firstSeen time.Time
}

// ClaudeLimitsService persists and retrieves Claude rate limit data.
type ClaudeLimitsService struct {
	pool    *db.Pool
	clk     clock.Clock
	mu      sync.Mutex
	pending map[string]pendingDowngrade // keyed by window ("5h"/"7d")
}

// NewClaudeLimitsService creates a new ClaudeLimitsService.
func NewClaudeLimitsService(pool *db.Pool, clk clock.Clock) *ClaudeLimitsService {
	return &ClaudeLimitsService{
		pool:    pool,
		clk:     clk,
		pending: make(map[string]pendingDowngrade),
	}
}

// Update writes non-sentinel fields with two safety rails:
//   - stale-window auto-clear: when the stored resets_at for a window is in the
//     past, both pct and resets_at for that window are cleared before the
//     incoming payload is applied. An incoming pct without a fresh resets_at
//     (orphan pct) is dropped, since it can no longer be attributed to a known
//     reset window.
//   - time-bounded monotonic guard: pct decreases within an active reset window
//     are rejected, but the same lower value persisting for
//     downgradeConfirmDuration is accepted (Anthropic occasionally revises
//     reported pct downward as usage is reconciled).
//
// Returns UpdateResult{Changed: false} when nothing was written; does not bump
// claude_limits_updated_at in that case.
func (s *ClaudeLimitsService) Update(limits ClaudeLimits) (UpdateResult, error) {
	existing, err := s.Get()
	if err != nil {
		return UpdateResult{}, err
	}

	now := s.clk.Now().UTC()
	var pairs []struct{ k, v string }

	fiveHourCleared := s.maybeClearStale("5h", &existing.FiveHourUsedPct, &existing.FiveHourResetsAt, now, &pairs,
		"claude_5h_used_pct", "claude_5h_resets_at")
	sevenDayCleared := s.maybeClearStale("7d", &existing.SevenDayUsedPct, &existing.SevenDayResetsAt, now, &pairs,
		"claude_weekly_used_pct", "claude_weekly_resets_at")

	if limits.FiveHourUsedPct >= 0 && !(fiveHourCleared && limits.FiveHourResetsAt == "") {
		if s.acceptPct("5h", limits.FiveHourUsedPct, existing.FiveHourUsedPct, existing.FiveHourResetsAt, limits.FiveHourResetsAt, now) {
			pairs = append(pairs, struct{ k, v string }{"claude_5h_used_pct", strconv.FormatFloat(limits.FiveHourUsedPct, 'f', -1, 64)})
		}
	}
	if limits.FiveHourResetsAt != "" {
		pairs = append(pairs, struct{ k, v string }{"claude_5h_resets_at", limits.FiveHourResetsAt})
	}

	if limits.SevenDayUsedPct >= 0 && !(sevenDayCleared && limits.SevenDayResetsAt == "") {
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

// maybeClearStale clears a window's stored pct + resets_at when the stored reset
// time is in the past. Mutates the local existing fields to sentinel so the rest
// of Update treats the window as unset. Returns true when a clear was queued.
func (s *ClaudeLimitsService) maybeClearStale(window string, pct *float64, resetsAt *string, now time.Time, pairs *[]struct{ k, v string }, pctKey, resetsKey string) bool {
	if *resetsAt == "" {
		return false
	}
	reset, err := time.Parse(time.RFC3339, *resetsAt)
	if err != nil {
		return false
	}
	if now.Before(reset) {
		return false
	}
	logger.Info(context.Background(), "claude limits: clearing stale window",
		"window", window, "old_pct", *pct, "stale_resets_at", *resetsAt)
	*pairs = append(*pairs,
		struct{ k, v string }{pctKey, ""},
		struct{ k, v string }{resetsKey, ""},
	)
	*pct = -1
	*resetsAt = ""
	s.clearPending(window)
	return true
}

// acceptPct returns true if the new pct should be written for the given window.
// Decreases within an active reset window are rejected unless the same lower
// value has persisted for downgradeConfirmDuration.
func (s *ClaudeLimitsService) acceptPct(window string, newPct, existingPct float64, existingResetsAt, newResetsAt string, now time.Time) bool {
	if existingPct < 0 {
		s.clearPending(window)
		return true
	}
	existingReset, err := time.Parse(time.RFC3339, existingResetsAt)
	if err != nil {
		s.clearPending(window)
		return true
	}
	if !now.Before(existingReset) {
		s.clearPending(window)
		return true
	}
	if newResetsAt != "" && newResetsAt != existingResetsAt {
		s.clearPending(window)
		return true
	}
	if newPct >= existingPct-claudeLimitsEpsilon {
		s.clearPending(window)
		return true
	}
	if s.confirmDowngrade(window, existingResetsAt, newPct, now) {
		logger.Info(context.Background(), "claude limits: accepting confirmed downgrade",
			"window", window, "old_pct", existingPct, "new_pct", newPct, "resets_at", existingResetsAt)
		return true
	}
	logger.Info(context.Background(), "claude limits: rejecting non-monotonic pct",
		"window", window, "old_pct", existingPct, "new_pct", newPct, "resets_at", existingResetsAt)
	return false
}

// confirmDowngrade tracks repeated reports of the same lower pct. Returns true
// once the same (resetsAt, pct±epsilon) has persisted for downgradeConfirmDuration.
// A mismatch (different resetsAt or pct outside epsilon) restarts the timer.
func (s *ClaudeLimitsService) confirmDowngrade(window, resetsAt string, newPct float64, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.pending[window]
	if !ok || entry.resetsAt != resetsAt || math.Abs(entry.pct-newPct) > claudeLimitsEpsilon {
		s.pending[window] = pendingDowngrade{resetsAt: resetsAt, pct: newPct, firstSeen: now}
		return false
	}
	if now.Sub(entry.firstSeen) < downgradeConfirmDuration {
		return false
	}
	delete(s.pending, window)
	return true
}

func (s *ClaudeLimitsService) clearPending(window string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, window)
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
