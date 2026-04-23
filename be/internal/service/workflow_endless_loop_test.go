package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupEndlessLoopTestEnv creates a minimal DB stack and workflow service for
// testing that buildV4State emits endless_loop and stop_endless_loop_after_iteration.
func setupEndlessLoopTestEnv(t *testing.T) (*db.Pool, *WorkflowService) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "endless_loop_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		"proj1", now, now); err != nil {
		t.Fatalf("project insert: %v", err)
	}

	svc := NewWorkflowService(pool, clock.Real())

	// Create a project-scoped workflow def
	if _, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:        "wf-proj",
		ScopeType: "project",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef project: %v", err)
	}

	// Create a ticket-scoped workflow def as well
	if _, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:        "wf-ticket",
		ScopeType: "ticket",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef ticket: %v", err)
	}

	return pool, svc
}

// insertWFI directly inserts a workflow instance row with explicit endless-loop flags.
func insertWFI(t *testing.T, pool *db.Pool, id, workflowID, scopeType string, endlessLoop, stopAfter bool) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ticketID := ""
	if scopeType == "ticket" {
		ticketID = "ticket-1"
	}
	_, err := pool.Exec(`
		INSERT INTO workflow_instances (
			id, project_id, ticket_id, workflow_id, scope_type, status,
			findings, retry_count, endless_loop, stop_endless_loop_after_iteration,
			created_at, updated_at
		) VALUES (?, 'proj1', ?, ?, ?, 'active', '{}', 0, ?, ?, ?, ?)`,
		id, ticketID, workflowID, scopeType, endlessLoop, stopAfter, now, now)
	if err != nil {
		t.Fatalf("workflow_instance insert %s: %v", id, err)
	}
}

// assertBoolKey checks that a key in the state map is an exact boolean value.
func assertBoolKey(t *testing.T, state map[string]interface{}, key string, want bool) {
	t.Helper()
	raw, ok := state[key]
	if !ok {
		t.Fatalf("%q missing from state", key)
	}
	got, ok := raw.(bool)
	if !ok {
		t.Fatalf("%q is not bool: %T", key, raw)
	}
	if got != want {
		t.Errorf("%q = %v, want %v", key, got, want)
	}
}

// TestBuildV4State_EndlessLoop_ProjectScopedActive asserts that a project-scoped
// workflow instance with endless_loop=true and stop_endless_loop_after_iteration=false
// emits both keys in the v4 state response.
func TestBuildV4State_EndlessLoop_ProjectScopedActive(t *testing.T) {
	pool, svc := setupEndlessLoopTestEnv(t)

	wfiID := "wfi-proj-endless"
	insertWFI(t, pool, wfiID, "wf-proj", "project", true, false)

	wi, err := svc.GetProjectWorkflowInstance("proj1", "wf-proj")
	if err != nil {
		t.Fatalf("GetProjectWorkflowInstance: %v", err)
	}
	if wi.ID != wfiID {
		t.Fatalf("wi.ID = %q, want %q", wi.ID, wfiID)
	}

	state := svc.buildV4State(wi)

	assertBoolKey(t, state, "endless_loop", true)
	assertBoolKey(t, state, "stop_endless_loop_after_iteration", false)
}

// TestBuildV4State_EndlessLoop_StopAfterIterationFlip flips
// stop_endless_loop_after_iteration from false → true in the DB and verifies the
// response reflects the updated value on re-read (proving the field reads from DB
// state, not a cached construct).
func TestBuildV4State_EndlessLoop_StopAfterIterationFlip(t *testing.T) {
	pool, svc := setupEndlessLoopTestEnv(t)

	wfiID := "wfi-proj-flip"
	insertWFI(t, pool, wfiID, "wf-proj", "project", true, false)

	// Initial read: stop flag false.
	wi, err := svc.GetProjectWorkflowInstance("proj1", "wf-proj")
	if err != nil {
		t.Fatalf("initial GetProjectWorkflowInstance: %v", err)
	}
	state := svc.buildV4State(wi)
	assertBoolKey(t, state, "endless_loop", true)
	assertBoolKey(t, state, "stop_endless_loop_after_iteration", false)

	// Flip the stop flag directly in DB.
	if _, err := pool.Exec(
		`UPDATE workflow_instances SET stop_endless_loop_after_iteration = 1 WHERE id = ?`,
		wfiID); err != nil {
		t.Fatalf("UPDATE stop flag: %v", err)
	}

	// Re-read: the response must now reflect true.
	wi2, err := svc.GetProjectWorkflowInstance("proj1", "wf-proj")
	if err != nil {
		t.Fatalf("post-flip GetProjectWorkflowInstance: %v", err)
	}
	state2 := svc.buildV4State(wi2)
	assertBoolKey(t, state2, "endless_loop", true)
	assertBoolKey(t, state2, "stop_endless_loop_after_iteration", true)
}

// TestBuildV4State_EndlessLoop_TicketScopedDefaultFalse verifies that a
// ticket-scoped workflow instance (endless_loop defaulting to false) still has
// both keys present and set to false — satisfying the acceptance criterion that
// the keys are unconditional.
func TestBuildV4State_EndlessLoop_TicketScopedDefaultFalse(t *testing.T) {
	pool, svc := setupEndlessLoopTestEnv(t)

	wfiID := "wfi-ticket-default"
	insertWFI(t, pool, wfiID, "wf-ticket", "ticket", false, false)

	wi, err := svc.GetWorkflowInstance("proj1", "ticket-1", "wf-ticket")
	if err != nil {
		t.Fatalf("GetWorkflowInstance: %v", err)
	}
	if wi.ID != wfiID {
		t.Fatalf("wi.ID = %q, want %q", wi.ID, wfiID)
	}

	state := svc.buildV4State(wi)

	assertBoolKey(t, state, "endless_loop", false)
	assertBoolKey(t, state, "stop_endless_loop_after_iteration", false)
}

// TestBuildV4State_EndlessLoop_KeysAlwaysPresent is a small matrix covering the
// four combinations of (endless_loop, stop_endless_loop_after_iteration) values
// to guard against any regression that would make the keys conditional.
func TestBuildV4State_EndlessLoop_KeysAlwaysPresent(t *testing.T) {
	cases := []struct {
		name      string
		endless   bool
		stopAfter bool
	}{
		{"both_false", false, false},
		{"endless_only", true, false},
		{"stop_only", false, true}, // nonsensical in practice, but must still serialize
		{"both_true", true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc := setupEndlessLoopTestEnv(t)

			wfiID := "wfi-matrix-" + tc.name
			insertWFI(t, pool, wfiID, "wf-proj", "project", tc.endless, tc.stopAfter)

			wi, err := svc.GetProjectWorkflowInstance("proj1", "wf-proj")
			if err != nil {
				t.Fatalf("GetProjectWorkflowInstance: %v", err)
			}

			state := svc.buildV4State(wi)
			assertBoolKey(t, state, "endless_loop", tc.endless)
			assertBoolKey(t, state, "stop_endless_loop_after_iteration", tc.stopAfter)
		})
	}
}
