package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// newStaleTestEnv builds a ClaudeLimitsService with a clock pinned at fixedTime
// for deterministic stale-window / downgrade-confirmation tests.
func newStaleTestEnv(t *testing.T, fixedTime time.Time) (*ClaudeLimitsService, *clock.TestClock) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "stale_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	clk := clock.NewTest(fixedTime)
	return NewClaudeLimitsService(pool, clk), clk
}

// TestClaudeLimitsService_Stale_ClearsAndAcceptsFullPayload verifies that when
// the stored resets_at is in the past and a full new payload (pct + fresh
// resets_at) arrives, both fields update cleanly.
func TestClaudeLimitsService_Stale_ClearsAndAcceptsFullPayload(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	svc, clk := newStaleTestEnv(t, start)

	// Seed: 5h window already expired (set at start, advance past reset).
	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  33,
		FiveHourResetsAt: "2026-05-14T11:00:00Z",
		SevenDayUsedPct:  -1,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	clk.Advance(2 * time.Hour) // now > reset

	// Fresh full payload for a new window.
	result, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  10,
		FiveHourResetsAt: "2026-05-14T17:00:00Z",
		SevenDayUsedPct:  -1,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Changed=true")
	}
	got, err := svc.Get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.FiveHourUsedPct != 10 {
		t.Errorf("FiveHourUsedPct = %v, want 10", got.FiveHourUsedPct)
	}
	if got.FiveHourResetsAt != "2026-05-14T17:00:00Z" {
		t.Errorf("FiveHourResetsAt = %q, want fresh value", got.FiveHourResetsAt)
	}
}

// TestClaudeLimitsService_Stale_DropsOrphanPct verifies that a pct arriving
// without a fresh resets_at after the stored window expired is rejected: both
// stored fields are cleared, the orphan pct is NOT written.
func TestClaudeLimitsService_Stale_DropsOrphanPct(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	svc, clk := newStaleTestEnv(t, start)

	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  33,
		FiveHourResetsAt: "2026-05-14T11:00:00Z",
		SevenDayUsedPct:  -1,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	clk.Advance(2 * time.Hour)

	// Orphan pct: no resets_at supplied.
	result, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct: 42,
		SevenDayUsedPct: -1,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Changed=true (stale fields cleared)")
	}
	got, err := svc.Get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.FiveHourUsedPct != -1 {
		t.Errorf("FiveHourUsedPct = %v, want -1 (orphan dropped)", got.FiveHourUsedPct)
	}
	if got.FiveHourResetsAt != "" {
		t.Errorf("FiveHourResetsAt = %q, want empty", got.FiveHourResetsAt)
	}
}

// TestClaudeLimitsService_Stale_BothWindowsCleared verifies the clearing logic
// runs independently per window.
func TestClaudeLimitsService_Stale_BothWindowsCleared(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	svc, clk := newStaleTestEnv(t, start)

	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  33,
		FiveHourResetsAt: "2026-05-14T11:00:00Z",
		SevenDayUsedPct:  80,
		SevenDayResetsAt: "2026-05-14T12:00:00Z",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	clk.Advance(5 * time.Hour) // past both resets

	// Empty-input call: both windows should clear.
	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct: -1,
		SevenDayUsedPct: -1,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := svc.Get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.FiveHourUsedPct != -1 || got.FiveHourResetsAt != "" {
		t.Errorf("5h not cleared: pct=%v resets=%q", got.FiveHourUsedPct, got.FiveHourResetsAt)
	}
	if got.SevenDayUsedPct != -1 || got.SevenDayResetsAt != "" {
		t.Errorf("7d not cleared: pct=%v resets=%q", got.SevenDayUsedPct, got.SevenDayResetsAt)
	}
}

// TestClaudeLimitsService_Downgrade_AcceptedAfterConfirmDuration verifies that
// a consistently-reported lower pct is accepted once it has persisted for
// downgradeConfirmDuration (5 min by default).
func TestClaudeLimitsService_Downgrade_AcceptedAfterConfirmDuration(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	svc, clk := newStaleTestEnv(t, start)

	resetsAt := "2026-05-18T05:00:00Z" // far future, active window
	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  -1,
		SevenDayUsedPct:  80,
		SevenDayResetsAt: resetsAt,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// First downgrade attempt: pct only (no resets_at so we can assert on
	// stored value alone — re-writing the same resets_at would still bump
	// claude_limits_updated_at).
	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct: -1,
		SevenDayUsedPct: 69,
	}); err != nil {
		t.Fatalf("first downgrade: %v", err)
	}
	if got, _ := svc.Get(); got.SevenDayUsedPct != 80 {
		t.Fatalf("after first attempt: SevenDayUsedPct = %v, want 80 (rejected)", got.SevenDayUsedPct)
	}

	// Advance just under the confirmation window — still rejected.
	clk.Advance(downgradeConfirmDuration - time.Second)
	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct: -1,
		SevenDayUsedPct: 69,
	}); err != nil {
		t.Fatalf("mid downgrade: %v", err)
	}
	if got, _ := svc.Get(); got.SevenDayUsedPct != 80 {
		t.Fatalf("just under threshold: SevenDayUsedPct = %v, want 80 (rejected)", got.SevenDayUsedPct)
	}

	// Cross the threshold — accepted.
	clk.Advance(2 * time.Second)
	final, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct: -1,
		SevenDayUsedPct: 69,
	})
	if err != nil {
		t.Fatalf("final downgrade: %v", err)
	}
	if !final.Changed {
		t.Fatal("final downgrade should be accepted")
	}
	got, err := svc.Get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SevenDayUsedPct != 69 {
		t.Errorf("SevenDayUsedPct = %v, want 69", got.SevenDayUsedPct)
	}
}

// TestClaudeLimitsService_Downgrade_TrackerResetsOnChange verifies that when
// the rejected pct changes (Anthropic reports a different lower value), the
// confirmation timer restarts.
func TestClaudeLimitsService_Downgrade_TrackerResetsOnChange(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	svc, clk := newStaleTestEnv(t, start)

	resetsAt := "2026-05-18T05:00:00Z"
	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  -1,
		SevenDayUsedPct:  80,
		SevenDayResetsAt: resetsAt,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// First downgrade: 69. Rejected, pending = {69, t0}.
	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct: -1,
		SevenDayUsedPct: 69,
	}); err != nil {
		t.Fatalf("first: %v", err)
	}

	// Advance past confirm threshold but switch to a DIFFERENT lower pct.
	// Tracker should reset to (50, t1), so still rejected.
	clk.Advance(downgradeConfirmDuration + time.Minute)
	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct: -1,
		SevenDayUsedPct: 50,
	}); err != nil {
		t.Fatalf("switched: %v", err)
	}
	got, err := svc.Get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SevenDayUsedPct != 80 {
		t.Errorf("SevenDayUsedPct = %v, want 80 (still rejected after tracker reset)", got.SevenDayUsedPct)
	}
}
