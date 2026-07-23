package repo

import (
	"database/sql"
	"errors"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// seedInstanceForCursor inserts the minimal project/workflow/workflow_instance
// row chain agent_step_cursors' FK requires.
func seedInstanceForCursor(t *testing.T, pool db.Querier, projectID, workflowID, instanceID string) {
	t.Helper()
	if _, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		VALUES (?, ?, '', ?, 'ticket', 'active', 0, datetime('now'), datetime('now'))`, instanceID, projectID, workflowID); err != nil {
		t.Fatalf("insert workflow_instance %s: %v", instanceID, err)
	}
}

func TestAgentStepCursor_InsertGetRoundTrip(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-cur1", "wf-cur1")
	seedInstanceForCursor(t, pool, "proj-cur1", "wf-cur1", "wfi-cur1")

	r := NewAgentStepCursorRepo(pool, clock.Real())
	if err := r.Insert(&model.AgentStepCursor{
		WorkflowInstanceID: "wfi-cur1",
		NodeID:             "node-a",
		StepsSnapshot:      `[{"step_id":"s1"}]`,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := r.Get("wfi-cur1", "node-a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Revision != 1 {
		t.Errorf("Revision = %d, want 1 (default)", got.Revision)
	}
	if got.CurrentIndex != 0 {
		t.Errorf("CurrentIndex = %d, want 0 (default)", got.CurrentIndex)
	}
	if got.Completed != "[]" {
		t.Errorf("Completed = %q, want []", got.Completed)
	}
	if got.StepsSnapshot != `[{"step_id":"s1"}]` {
		t.Errorf("StepsSnapshot = %q, want the seeded snapshot", got.StepsSnapshot)
	}
}

// TestAgentStepCursor_RepeatInsertIsNoOp verifies a second Insert for the same
// key does not reset an already-advanced cursor (Snapshot-on-relaunch
// contract): the ON CONFLICT DO NOTHING must leave revision/current_index/
// completed untouched.
func TestAgentStepCursor_RepeatInsertIsNoOp(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-cur2", "wf-cur2")
	seedInstanceForCursor(t, pool, "proj-cur2", "wf-cur2", "wfi-cur2")

	r := NewAgentStepCursorRepo(pool, clock.Real())
	if err := r.Insert(&model.AgentStepCursor{
		WorkflowInstanceID: "wfi-cur2",
		NodeID:             "node-a",
		StepsSnapshot:      `[{"step_id":"s1"},{"step_id":"s2"}]`,
	}); err != nil {
		t.Fatalf("initial Insert: %v", err)
	}

	ok, err := r.Advance("wfi-cur2", "node-a", 1, 0, `[{"step_id":"s1"}]`)
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if !ok {
		t.Fatal("Advance: expected true")
	}

	// Relaunch/retry calls Insert again with the original snapshot.
	if err := r.Insert(&model.AgentStepCursor{
		WorkflowInstanceID: "wfi-cur2",
		NodeID:             "node-a",
		StepsSnapshot:      `[{"step_id":"s1"},{"step_id":"s2"}]`,
	}); err != nil {
		t.Fatalf("repeat Insert: %v", err)
	}

	got, err := r.Get("wfi-cur2", "node-a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Revision != 2 {
		t.Errorf("Revision after repeat Insert = %d, want 2 (unchanged by no-op Insert)", got.Revision)
	}
	if got.CurrentIndex != 1 {
		t.Errorf("CurrentIndex after repeat Insert = %d, want 1 (unchanged by no-op Insert)", got.CurrentIndex)
	}
	if got.Completed != `[{"step_id":"s1"}]` {
		t.Errorf("Completed after repeat Insert = %q, want the advanced value", got.Completed)
	}
}

func TestAgentStepCursor_Advance(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-cur3", "wf-cur3")
	seedInstanceForCursor(t, pool, "proj-cur3", "wf-cur3", "wfi-cur3")

	r := NewAgentStepCursorRepo(pool, clock.Real())
	if err := r.Insert(&model.AgentStepCursor{
		WorkflowInstanceID: "wfi-cur3",
		NodeID:             "node-a",
		StepsSnapshot:      `[{"step_id":"s1"}]`,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	t.Run("stale revision returns false and mutates nothing", func(t *testing.T) {
		ok, err := r.Advance("wfi-cur3", "node-a", 99, 0, `["x"]`)
		if err != nil {
			t.Fatalf("Advance: %v", err)
		}
		if ok {
			t.Fatal("Advance with stale revision: expected false")
		}
		got, gErr := r.Get("wfi-cur3", "node-a")
		if gErr != nil {
			t.Fatalf("Get: %v", gErr)
		}
		if got.Revision != 1 || got.CurrentIndex != 0 || got.Completed != "[]" {
			t.Errorf("cursor mutated by failed Advance: %+v", got)
		}
	})

	t.Run("mismatched current_index returns false", func(t *testing.T) {
		ok, err := r.Advance("wfi-cur3", "node-a", 1, 5, `["x"]`)
		if err != nil {
			t.Fatalf("Advance: %v", err)
		}
		if ok {
			t.Fatal("Advance with mismatched current_index: expected false")
		}
	})

	t.Run("matching revision+index returns true and bumps both", func(t *testing.T) {
		ok, err := r.Advance("wfi-cur3", "node-a", 1, 0, `[{"step_id":"s1"}]`)
		if err != nil {
			t.Fatalf("Advance: %v", err)
		}
		if !ok {
			t.Fatal("Advance with matching (revision, current_index): expected true")
		}
		got, gErr := r.Get("wfi-cur3", "node-a")
		if gErr != nil {
			t.Fatalf("Get: %v", gErr)
		}
		if got.Revision != 2 {
			t.Errorf("Revision = %d, want 2", got.Revision)
		}
		if got.CurrentIndex != 1 {
			t.Errorf("CurrentIndex = %d, want 1", got.CurrentIndex)
		}
		if got.Completed != `[{"step_id":"s1"}]` {
			t.Errorf("Completed = %q, want the completed JSON passed to Advance", got.Completed)
		}
	})
}

func TestAgentStepCursor_GetUnknownKeyReturnsErrNoRows(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-cur4", "wf-cur4")

	r := NewAgentStepCursorRepo(pool, clock.Real())
	_, err := r.Get("wfi-does-not-exist", "node-nope")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Get(unknown) error = %v, want sql.ErrNoRows", err)
	}
}

// TestAgentStepCursor_CascadeDeleteOnWorkflowInstance verifies deleting the
// referenced workflow_instance removes the cursor row via FK CASCADE.
func TestAgentStepCursor_CascadeDeleteOnWorkflowInstance(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-cur5", "wf-cur5")
	seedInstanceForCursor(t, pool, "proj-cur5", "wf-cur5", "wfi-cur5")

	r := NewAgentStepCursorRepo(pool, clock.Real())
	if err := r.Insert(&model.AgentStepCursor{
		WorkflowInstanceID: "wfi-cur5",
		NodeID:             "node-a",
		StepsSnapshot:      `[]`,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if _, err := pool.Exec(`DELETE FROM workflow_instances WHERE id = 'wfi-cur5'`); err != nil {
		t.Fatalf("delete workflow_instance: %v", err)
	}

	if _, err := r.Get("wfi-cur5", "node-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Get after parent delete error = %v, want sql.ErrNoRows", err)
	}
}

// TestAgentStepCursor_DuplicateInsertKeyReturnsNoErrorAndNoOverwrite verifies
// the composite PK is (workflow_instance_id, node_id): two different node_ids
// under the same instance are independent rows.
func TestAgentStepCursor_DuplicateInsertKeyReturnsNoErrorAndNoOverwrite(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-cur6", "wf-cur6")
	seedInstanceForCursor(t, pool, "proj-cur6", "wf-cur6", "wfi-cur6")

	r := NewAgentStepCursorRepo(pool, clock.Real())
	if err := r.Insert(&model.AgentStepCursor{WorkflowInstanceID: "wfi-cur6", NodeID: "node-a", StepsSnapshot: `[]`}); err != nil {
		t.Fatalf("Insert node-a: %v", err)
	}
	if err := r.Insert(&model.AgentStepCursor{WorkflowInstanceID: "wfi-cur6", NodeID: "node-b", StepsSnapshot: `[]`}); err != nil {
		t.Fatalf("Insert node-b: %v", err)
	}

	if _, err := r.Get("wfi-cur6", "node-a"); err != nil {
		t.Errorf("Get node-a: %v", err)
	}
	if _, err := r.Get("wfi-cur6", "node-b"); err != nil {
		t.Errorf("Get node-b: %v", err)
	}
}
