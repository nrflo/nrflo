package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

const (
	stepReadProjectID = "proj-stepread"
	stepReadWFID      = "wf-stepread"
)

// setupStepwiseReadEnv seeds the minimal project/workflow/workflow_instance
// chain BuildStepCursors' JOIN needs and returns the pool + WorkflowService.
func setupStepwiseReadEnv(t *testing.T, instanceID string) (*db.Pool, *WorkflowService) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "stepread.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Step Read', '/tmp', ?, ?)`,
		stepReadProjectID, now, now)
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'ticket', ?, ?)`,
		stepReadWFID, stepReadProjectID, now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		VALUES (?, ?, '', ?, 'ticket', 'active', 0, ?, ?)`,
		instanceID, stepReadProjectID, stepReadWFID, now, now)

	return pool, NewWorkflowService(pool, clock.Real())
}

// seedStepwiseReadCursor inserts an agent_step_cursors row directly (mirrors
// builtinTestEnv.seedStepCursor in tools_builtin, but service-package-local).
func seedStepwiseReadCursor(t *testing.T, pool *db.Pool, instanceID, nodeID, stepsJSON string, currentIndex, revision int, completedJSON, rejectionsJSON string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if completedJSON == "" {
		completedJSON = "[]"
	}
	if rejectionsJSON == "" {
		rejectionsJSON = "{}"
	}
	mustExec(t, pool, `
		INSERT INTO agent_step_cursors (workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, rejections, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		instanceID, nodeID, stepsJSON, revision, currentIndex, completedJSON, rejectionsJSON, now, now)
}

const stepReadTwoStepsJSON = `[
	{"step_id":"s1","title":"Step One","instruction":"do 1","rotation_allowed":true},
	{"step_id":"s2","title":"Step Two","instruction":"do 2","rotation_allowed":true}
]`

func TestBuildStepCursors_NoRowsReturnsNilForNonStepwiseInstance(t *testing.T) {
	t.Parallel()
	_, svc := setupStepwiseReadEnv(t, "wfi-stepread-empty")

	got := svc.BuildStepCursors("wfi-stepread-empty")
	if got != nil {
		t.Errorf("BuildStepCursors(no cursors) = %+v, want nil", got)
	}
}

func TestBuildStepCursors_StatusDerivationPendingActiveDone(t *testing.T) {
	t.Parallel()
	pool, svc := setupStepwiseReadEnv(t, "wfi-stepread-1")
	seedStepwiseReadCursor(t, pool, "wfi-stepread-1", "node-a", stepReadTwoStepsJSON, 1, 2,
		`[{"step_id":"s1","completed_at":"2026-01-01T00:00:00Z"}]`, "")

	cursors := svc.BuildStepCursors("wfi-stepread-1")
	prog, ok := cursors["node-a"]
	if !ok {
		t.Fatalf("cursors = %+v, want key node-a", cursors)
	}
	if len(prog.Steps) != 2 {
		t.Fatalf("Steps = %+v, want 2 entries", prog.Steps)
	}
	if prog.Steps[0].Status != "done" {
		t.Errorf("Steps[0].Status = %q, want done", prog.Steps[0].Status)
	}
	if prog.Steps[1].Status != "active" {
		t.Errorf("Steps[1].Status = %q, want active", prog.Steps[1].Status)
	}
	if prog.CurrentStepID != "s2" {
		t.Errorf("CurrentStepID = %q, want s2", prog.CurrentStepID)
	}
	if prog.Done {
		t.Error("Done = true, want false (still on s2)")
	}
}

func TestBuildStepCursors_RejectedRetryingWhenRejectionCountPositive(t *testing.T) {
	t.Parallel()
	pool, svc := setupStepwiseReadEnv(t, "wfi-stepread-2")
	seedStepwiseReadCursor(t, pool, "wfi-stepread-2", "node-a", stepReadTwoStepsJSON, 0, 1, "", `{"s1":2}`)

	cursors := svc.BuildStepCursors("wfi-stepread-2")
	prog := cursors["node-a"]
	if prog.Steps[0].Status != "rejected_retrying" {
		t.Errorf("Steps[0].Status = %q, want rejected_retrying", prog.Steps[0].Status)
	}
	if prog.Steps[0].Rejections != 2 {
		t.Errorf("Steps[0].Rejections = %d, want 2", prog.Steps[0].Rejections)
	}
}

func TestBuildStepCursors_DoneWhenCurrentIndexAtEnd(t *testing.T) {
	t.Parallel()
	pool, svc := setupStepwiseReadEnv(t, "wfi-stepread-3")
	seedStepwiseReadCursor(t, pool, "wfi-stepread-3", "node-a", stepReadTwoStepsJSON, 2, 3,
		`[{"step_id":"s1","completed_at":"2026-01-01T00:00:00Z"},{"step_id":"s2","completed_at":"2026-01-01T00:01:00Z"}]`, "")

	cursors := svc.BuildStepCursors("wfi-stepread-3")
	prog := cursors["node-a"]
	if !prog.Done {
		t.Error("Done = false, want true")
	}
	if prog.CurrentStepID != "" {
		t.Errorf("CurrentStepID = %q, want empty when done", prog.CurrentStepID)
	}
	if prog.Steps[0].Status != "done" || prog.Steps[1].Status != "done" {
		t.Errorf("Steps = %+v, want both done", prog.Steps)
	}
}

func TestBuildStepCursors_RotatedPreservedFromCompletedEntry(t *testing.T) {
	t.Parallel()
	pool, svc := setupStepwiseReadEnv(t, "wfi-stepread-4")
	seedStepwiseReadCursor(t, pool, "wfi-stepread-4", "node-a", stepReadTwoStepsJSON, 1, 2,
		`[{"step_id":"s1","completed_at":"2026-01-01T00:00:00Z","rotated":true}]`, "")

	cursors := svc.BuildStepCursors("wfi-stepread-4")
	prog := cursors["node-a"]
	if !prog.Steps[0].Rotated {
		t.Error("Steps[0].Rotated = false, want true")
	}
	if prog.Steps[1].Rotated {
		t.Error("Steps[1].Rotated = true, want false (not yet completed)")
	}
}

func TestBuildStepCursors_MultipleNodeIDsKeyedCorrectly(t *testing.T) {
	t.Parallel()
	pool, svc := setupStepwiseReadEnv(t, "wfi-stepread-5")
	seedStepwiseReadCursor(t, pool, "wfi-stepread-5", "node-a", stepReadTwoStepsJSON, 0, 1, "", "")
	seedStepwiseReadCursor(t, pool, "wfi-stepread-5", "node-b", stepReadTwoStepsJSON, 1, 2,
		`[{"step_id":"s1","completed_at":"2026-01-01T00:00:00Z"}]`, "")

	cursors := svc.BuildStepCursors("wfi-stepread-5")
	if len(cursors) != 2 {
		t.Fatalf("cursors = %+v, want 2 entries", cursors)
	}
	if cursors["node-a"].CurrentIndex != 0 {
		t.Errorf("node-a.CurrentIndex = %d, want 0", cursors["node-a"].CurrentIndex)
	}
	if cursors["node-b"].CurrentIndex != 1 {
		t.Errorf("node-b.CurrentIndex = %d, want 1", cursors["node-b"].CurrentIndex)
	}
}

// TestBuildStepCursors_MalformedSnapshotOmittedNotError verifies a row whose
// steps_snapshot fails to decode is dropped from the result map (via
// stepengine.DecodeCursor's error) rather than failing the whole read.
func TestBuildStepCursors_MalformedSnapshotOmittedNotError(t *testing.T) {
	t.Parallel()
	pool, svc := setupStepwiseReadEnv(t, "wfi-stepread-6")
	seedStepwiseReadCursor(t, pool, "wfi-stepread-6", "node-bad", "not-json", 0, 1, "", "")
	seedStepwiseReadCursor(t, pool, "wfi-stepread-6", "node-good", stepReadTwoStepsJSON, 0, 1, "", "")

	cursors := svc.BuildStepCursors("wfi-stepread-6")
	if _, present := cursors["node-bad"]; present {
		t.Error("node-bad present in result, want omitted (malformed snapshot)")
	}
	if _, present := cursors["node-good"]; !present {
		t.Error("node-good missing from result, want present")
	}
}

// jsonRoundTrip is a sanity check the seeded steps_snapshot decodes as
// expected shape (guards a fixture typo from silently passing every test).
func TestStepReadTwoStepsJSON_DecodesToTwoSteps(t *testing.T) {
	t.Parallel()
	var raw []map[string]interface{}
	if err := json.Unmarshal([]byte(stepReadTwoStepsJSON), &raw); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("fixture step count = %d, want 2", len(raw))
	}
}
