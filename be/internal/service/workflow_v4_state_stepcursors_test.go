package service

import (
	"testing"

	"be/internal/clock"
	"be/internal/repo"
)

// TestBuildV4State_OmitsStepCursorsForNonStepwiseInstance guards the existing
// snapshot/read-model shape: an instance with no agent_step_cursors rows
// gets no "step_cursors" key at all (not an empty map).
func TestBuildV4State_OmitsStepCursorsForNonStepwiseInstance(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewWorkflowService(pool, clock.Real())

	wi, err := repo.NewWorkflowInstanceRepo(pool, clock.Real()).Get(instanceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	state := svc.buildV4State(wi)
	if _, ok := state["step_cursors"]; ok {
		t.Errorf(`state["step_cursors"] = %+v, want key absent for a non-stepwise instance`, state["step_cursors"])
	}
}

// TestBuildV4State_IncludesStepCursorsForStepwiseInstance verifies a stepwise
// instance (an agent_step_cursors row present) surfaces "step_cursors" keyed
// by node_id.
func TestBuildV4State_IncludesStepCursorsForStepwiseInstance(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewWorkflowService(pool, clock.Real())

	seedStepwiseReadCursor(t, pool, instanceID, "node-a", stepReadTwoStepsJSON, 0, 1, "", "")

	wi, err := repo.NewWorkflowInstanceRepo(pool, clock.Real()).Get(instanceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	state := svc.buildV4State(wi)
	cursors, ok := state["step_cursors"].(map[string]*StepCursorProgress)
	if !ok {
		t.Fatalf(`state["step_cursors"] = %#v (%T), want map[string]*StepCursorProgress`, state["step_cursors"], state["step_cursors"])
	}
	if _, present := cursors["node-a"]; !present {
		t.Errorf(`step_cursors = %+v, want key "node-a"`, cursors)
	}
}
