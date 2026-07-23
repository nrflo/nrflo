package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestAgentStepCursorRepo_ListByInstanceScopesToOneInstance verifies
// ListByInstance returns only the cursors under the requested instance,
// ordered by node_id, and never leaks a sibling instance's rows.
func TestAgentStepCursorRepo_ListByInstanceScopesToOneInstance(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-listcur", "wf-listcur")
	seedInstanceForCursor(t, pool, "proj-listcur", "wf-listcur", "wfi-listcur-a")
	seedInstanceForCursor(t, pool, "proj-listcur", "wf-listcur", "wfi-listcur-b")

	r := NewAgentStepCursorRepo(pool, clock.Real())
	for _, ins := range []*model.AgentStepCursor{
		{WorkflowInstanceID: "wfi-listcur-a", NodeID: "node-z", StepsSnapshot: `[]`},
		{WorkflowInstanceID: "wfi-listcur-a", NodeID: "node-a", StepsSnapshot: `[]`},
		{WorkflowInstanceID: "wfi-listcur-b", NodeID: "node-x", StepsSnapshot: `[]`},
	} {
		if err := r.Insert(ins); err != nil {
			t.Fatalf("Insert(%+v): %v", ins, err)
		}
	}

	got, err := r.ListByInstance("wfi-listcur-a")
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListByInstance = %+v, want 2 rows for wfi-listcur-a", got)
	}
	if got[0].NodeID != "node-a" || got[1].NodeID != "node-z" {
		t.Errorf("node order = [%s, %s], want [node-a, node-z] (ORDER BY node_id)", got[0].NodeID, got[1].NodeID)
	}
	for _, c := range got {
		if c.WorkflowInstanceID != "wfi-listcur-a" {
			t.Errorf("cursor %+v leaked from a sibling instance", c)
		}
	}
}

// TestAgentStepCursorRepo_ListByInstanceEmptyReturnsNilNoError verifies an
// instance with no cursor rows returns an empty/nil slice, not an error.
func TestAgentStepCursorRepo_ListByInstanceEmptyReturnsNilNoError(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-listcur2", "wf-listcur2")
	seedInstanceForCursor(t, pool, "proj-listcur2", "wf-listcur2", "wfi-listcur2")

	r := NewAgentStepCursorRepo(pool, clock.Real())
	got, err := r.ListByInstance("wfi-listcur2")
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListByInstance(empty) = %+v, want empty", got)
	}
}
