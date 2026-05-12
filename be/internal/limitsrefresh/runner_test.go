package limitsrefresh

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
)

func newTestDB(t *testing.T) *db.Pool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "runner_test.db")
	if err := rfrCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func newTestRunner(pool *db.Pool, clk clock.Clock, runFunc func(context.Context) error) *Runner {
	return &Runner{
		pool:     pool,
		clock:    clk,
		settings: service.NewGlobalSettingsService(pool, clk),
		limits:   service.NewClaudeLimitsService(pool, clk),
		runFunc:  runFunc,
	}
}

// TestRunner_Tick_GateOff_NotSet verifies tick is a no-op when sync_claude_limits is unset.
func TestRunner_Tick_GateOff_NotSet(t *testing.T) {
	t.Parallel()
	pool := newTestDB(t)
	calls := 0
	r := newTestRunner(pool, clock.Real(), func(ctx context.Context) error {
		calls++
		return nil
	})
	r.tick(context.Background())
	if calls != 0 {
		t.Errorf("runFunc calls = %d, want 0 (gate not set)", calls)
	}
}

// TestRunner_Tick_GateOff_False verifies tick is a no-op when sync_claude_limits = "false".
func TestRunner_Tick_GateOff_False(t *testing.T) {
	t.Parallel()
	pool := newTestDB(t)
	settings := service.NewGlobalSettingsService(pool, clock.Real())
	if err := settings.Set("sync_claude_limits", "false"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	calls := 0
	r := newTestRunner(pool, clock.Real(), func(ctx context.Context) error {
		calls++
		return nil
	})
	r.tick(context.Background())
	if calls != 0 {
		t.Errorf("runFunc calls = %d, want 0 (gate false)", calls)
	}
}

// TestRunner_Tick_GateOn_NoLimits_Runs verifies that when no limits have been recorded
// (UpdatedAt is empty), tick calls runFunc once.
func TestRunner_Tick_GateOn_NoLimits_Runs(t *testing.T) {
	t.Parallel()
	pool := newTestDB(t)
	settings := service.NewGlobalSettingsService(pool, clock.Real())
	if err := settings.Set("sync_claude_limits", "true"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	calls := 0
	r := newTestRunner(pool, clock.Real(), func(ctx context.Context) error {
		calls++
		return nil
	})
	r.tick(context.Background())
	if calls != 1 {
		t.Errorf("runFunc calls = %d, want 1 (no prior limits)", calls)
	}
}

// TestRunner_Tick_FreshLimits_Skips verifies that when limits were updated within
// freshnessWindow, tick does not call runFunc.
func TestRunner_Tick_FreshLimits_Skips(t *testing.T) {
	t.Parallel()
	pool := newTestDB(t)

	fixedTime := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	settings := service.NewGlobalSettingsService(pool, clk)
	if err := settings.Set("sync_claude_limits", "true"); err != nil {
		t.Fatalf("Set sync_claude_limits: %v", err)
	}

	// Seed limits at fixedTime so UpdatedAt == fixedTime RFC3339.
	limits := service.NewClaudeLimitsService(pool, clk)
	if err := limits.Update(service.ClaudeLimits{FiveHourUsedPct: 10, SevenDayUsedPct: 20}); err != nil {
		t.Fatalf("seed limits: %v", err)
	}

	// Clock not advanced → elapsed = 0 < freshnessWindow (30m).
	calls := 0
	r := newTestRunner(pool, clk, func(ctx context.Context) error {
		calls++
		return nil
	})
	r.tick(context.Background())
	if calls != 0 {
		t.Errorf("runFunc calls = %d, want 0 (fresh limits)", calls)
	}
}

// TestRunner_Tick_StaleLimits_Runs verifies that when limits are older than
// freshnessWindow, tick calls runFunc.
func TestRunner_Tick_StaleLimits_Runs(t *testing.T) {
	t.Parallel()
	pool := newTestDB(t)

	fixedTime := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	settings := service.NewGlobalSettingsService(pool, clk)
	if err := settings.Set("sync_claude_limits", "true"); err != nil {
		t.Fatalf("Set sync_claude_limits: %v", err)
	}

	// Seed limits at fixedTime.
	limits := service.NewClaudeLimitsService(pool, clk)
	if err := limits.Update(service.ClaudeLimits{FiveHourUsedPct: 10, SevenDayUsedPct: 20}); err != nil {
		t.Fatalf("seed limits: %v", err)
	}

	// Advance clock past freshnessWindow → elapsed > 30m → stale.
	clk.Advance(31 * time.Minute)

	calls := 0
	r := newTestRunner(pool, clk, func(ctx context.Context) error {
		calls++
		return nil
	})
	r.tick(context.Background())
	if calls != 1 {
		t.Errorf("runFunc calls = %d, want 1 (stale limits)", calls)
	}
}
