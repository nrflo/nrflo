package repo

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// seedRotationCursor inserts an agent_step_cursors row with the given
// completed JSON (pre-marshalled), for ListRotations tests.
func seedRotationCursor(t *testing.T, pool *db.Pool, instanceID, nodeID string, completed []model.CompletedStep) {
	t.Helper()
	b, err := json.Marshal(completed)
	if err != nil {
		t.Fatalf("marshal completed: %v", err)
	}
	if _, err := pool.Exec(`
		INSERT INTO agent_step_cursors (workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, rejections, created_at, updated_at)
		VALUES (?, ?, '[]', 1, 0, ?, '{}', datetime('now'), datetime('now'))`,
		instanceID, nodeID, string(b)); err != nil {
		t.Fatalf("seed rotation cursor: %v", err)
	}
}

func TestAgentStepCursorRepo_ListRotationsOnlyRotatedEntries(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-rot1", "wf-rot1")
	seedInstanceForCursor(t, pool, "proj-rot1", "wf-rot1", "wfi-rot1")

	seedRotationCursor(t, pool, "wfi-rot1", "node-a", []model.CompletedStep{
		{StepID: "s1", SessionID: "sess-1", CompletedAt: "2026-01-01T00:00:00Z", Rotated: true},
		{StepID: "s2", SessionID: "sess-1", CompletedAt: "2026-01-01T00:01:00Z", Rotated: false},
	})

	r := NewAgentStepCursorRepo(pool, clock.Real())
	got, err := r.ListRotations(50, time.Time{})
	if err != nil {
		t.Fatalf("ListRotations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListRotations = %+v, want 1 entry (only Rotated=true)", got)
	}
	if got[0].StepID != "s1" || got[0].Kind != "step_rotation" || got[0].Status != "rotated" {
		t.Errorf("got[0] = %+v, want step_id=s1 kind=step_rotation status=rotated", got[0])
	}
	if got[0].WorkflowInstanceID != "wfi-rot1" || got[0].NodeID != "node-a" {
		t.Errorf("got[0] = %+v, want workflow_instance_id/node_id set", got[0])
	}
}

func TestAgentStepCursorRepo_ListRotationsNewestFirst(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-rot2", "wf-rot2")
	seedInstanceForCursor(t, pool, "proj-rot2", "wf-rot2", "wfi-rot2")

	seedRotationCursor(t, pool, "wfi-rot2", "node-a", []model.CompletedStep{
		{StepID: "old", CompletedAt: "2026-01-01T00:00:00Z", Rotated: true},
		{StepID: "new", CompletedAt: "2026-01-01T01:00:00Z", Rotated: true},
	})

	r := NewAgentStepCursorRepo(pool, clock.Real())
	got, err := r.ListRotations(50, time.Time{})
	if err != nil {
		t.Fatalf("ListRotations: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListRotations = %+v, want 2 entries", got)
	}
	if got[0].StepID != "new" || got[1].StepID != "old" {
		t.Errorf("order = [%s, %s], want [new, old] (newest first)", got[0].StepID, got[1].StepID)
	}
}

func TestAgentStepCursorRepo_ListRotationsSinceFilters(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-rot3", "wf-rot3")
	seedInstanceForCursor(t, pool, "proj-rot3", "wf-rot3", "wfi-rot3")

	seedRotationCursor(t, pool, "wfi-rot3", "node-a", []model.CompletedStep{
		{StepID: "old", CompletedAt: "2026-01-01T00:00:00Z", Rotated: true},
		{StepID: "new", CompletedAt: "2026-01-01T02:00:00Z", Rotated: true},
	})

	since := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	r := NewAgentStepCursorRepo(pool, clock.Real())
	got, err := r.ListRotations(50, since)
	if err != nil {
		t.Fatalf("ListRotations: %v", err)
	}
	if len(got) != 1 || got[0].StepID != "new" {
		t.Fatalf("ListRotations(since) = %+v, want only [new]", got)
	}
}

func TestAgentStepCursorRepo_ListRotationsLimitCaps(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-rot4", "wf-rot4")
	seedInstanceForCursor(t, pool, "proj-rot4", "wf-rot4", "wfi-rot4")

	seedRotationCursor(t, pool, "wfi-rot4", "node-a", []model.CompletedStep{
		{StepID: "s1", CompletedAt: "2026-01-01T00:00:00Z", Rotated: true},
		{StepID: "s2", CompletedAt: "2026-01-01T00:01:00Z", Rotated: true},
		{StepID: "s3", CompletedAt: "2026-01-01T00:02:00Z", Rotated: true},
	})

	r := NewAgentStepCursorRepo(pool, clock.Real())
	got, err := r.ListRotations(2, time.Time{})
	if err != nil {
		t.Fatalf("ListRotations: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListRotations(limit=2) = %+v, want 2 entries", got)
	}
	if got[0].StepID != "s3" || got[1].StepID != "s2" {
		t.Errorf("order = [%s, %s], want [s3, s2] (newest-first, capped)", got[0].StepID, got[1].StepID)
	}
}

// TestAgentStepCursorRepo_ListRotationsMalformedCompletedSkipped verifies a
// row with unparsable completed JSON is skipped rather than erroring the
// whole call.
func TestAgentStepCursorRepo_ListRotationsMalformedCompletedSkipped(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-rot5", "wf-rot5")
	seedInstanceForCursor(t, pool, "proj-rot5", "wf-rot5", "wfi-rot5")

	if _, err := pool.Exec(`
		INSERT INTO agent_step_cursors (workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, rejections, created_at, updated_at)
		VALUES ('wfi-rot5', 'node-bad', '[]', 1, 0, 'not-json', '{}', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed malformed cursor: %v", err)
	}
	seedRotationCursor(t, pool, "wfi-rot5", "node-good", []model.CompletedStep{
		{StepID: "s1", CompletedAt: "2026-01-01T00:00:00Z", Rotated: true},
	})

	r := NewAgentStepCursorRepo(pool, clock.Real())
	got, err := r.ListRotations(50, time.Time{})
	if err != nil {
		t.Fatalf("ListRotations: %v", err)
	}
	if len(got) != 1 || got[0].NodeID != "node-good" {
		t.Fatalf("ListRotations = %+v, want only node-good's entry", got)
	}
}
