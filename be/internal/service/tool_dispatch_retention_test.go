package service

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

func TestSweepToolDispatches_DefaultRetention_PurgesOlderRows(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupTraceTestEnv(t)

	dispatchRepo := repo.NewDispatchRepo(pool, clock.Real())
	old := "s-old"
	fresh := "s-fresh"
	if err := dispatchRepo.Insert(&model.ToolDispatch{ProjectID: "test-proj", SessionID: &old, ToolName: "t", Input: "{}", Status: model.DispatchStatusSuccess}); err != nil {
		t.Fatalf("Insert old: %v", err)
	}
	if err := dispatchRepo.Insert(&model.ToolDispatch{ProjectID: "test-proj", SessionID: &fresh, ToolName: "t", Input: "{}", Status: model.DispatchStatusSuccess}); err != nil {
		t.Fatalf("Insert fresh: %v", err)
	}
	// Backdate the "old" row past the default 30-day retention window.
	mustExec(t, pool, `UPDATE tool_dispatches SET created_at = ? WHERE session_id = 's-old'`,
		time.Now().UTC().Add(-40*24*time.Hour).Format(time.RFC3339Nano))

	deleted, err := SweepToolDispatches(pool, clock.Real(), time.Now())
	if err != nil {
		t.Fatalf("SweepToolDispatches: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	var remaining int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM tool_dispatches`).Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 1 {
		t.Errorf("remaining = %d, want 1 (fresh row survives)", remaining)
	}
}

func TestSweepToolDispatches_ProjectConfigOverridesDefault(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupTraceTestEnv(t)

	dispatchRepo := repo.NewDispatchRepo(pool, clock.Real())
	sid := "s-recent"
	if err := dispatchRepo.Insert(&model.ToolDispatch{ProjectID: "test-proj", SessionID: &sid, ToolName: "t", Input: "{}", Status: model.DispatchStatusSuccess}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	// Backdate 5 days — inside the default 30-day window but outside a
	// tightened global 1-day retention.
	mustExec(t, pool, `UPDATE tool_dispatches SET created_at = ? WHERE session_id = 's-recent'`,
		time.Now().UTC().Add(-5*24*time.Hour).Format(time.RFC3339Nano))

	if err := pool.SetConfig(ToolCallRetentionDaysKey, "1"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	deleted, err := SweepToolDispatches(pool, clock.Real(), time.Now())
	if err != nil {
		t.Fatalf("SweepToolDispatches: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1 (tightened 1-day retention config honored)", deleted)
	}
}

func TestSweepToolDispatches_NoRows_NoError(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupTraceTestEnv(t)
	deleted, err := SweepToolDispatches(pool, clock.Real(), time.Now())
	if err != nil {
		t.Fatalf("SweepToolDispatches: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}
