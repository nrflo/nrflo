package orchestrator

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// TestForceStopInstance_PlanSuspended_CancelsDraftAndMarksFailed is a direct
// unit test of forceStopInstance (not via the watcher) on a plan-suspended
// instance with a live draft: it must cancel the draft and mark the instance
// failed, taking the "no sessions to fail" skip path without panicking.
func TestForceStopInstance_PlanSuspended_CancelsDraftAndMarksFailed(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")
	if err := repo.NewWorkflowInstanceRepo(env.pool, clock.Real()).UpdateStatus(wfiID, model.WorkflowInstanceWaitingApproval); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	appendDraftPlan(t, env, wfiID, validManifest("goal", "n/a"))

	if err := env.orch.forceStopInstance(wfiID); err != nil {
		t.Fatalf("forceStopInstance: %v", err)
	}

	head, err := repo.NewPlanRepo(env.pool, clock.Real()).GetHead(wfiID)
	if err != nil {
		t.Fatalf("GetHead: %v", err)
	}
	if head.Status != model.PlanStatusCancelled {
		t.Errorf("plan status = %v, want cancelled", head.Status)
	}
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceFailed {
		t.Errorf("instance status = %v, want failed", wi.Status)
	}
}

// TestForceStopInstance_RejectsNonActiveNonPlanSuspended: a terminal status
// (e.g. completed) that is neither active nor plan-suspended must be
// rejected, leaving the row untouched.
func TestForceStopInstance_RejectsNonActiveNonPlanSuspended(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")
	if err := repo.NewWorkflowInstanceRepo(env.pool, clock.Real()).UpdateStatus(wfiID, model.WorkflowInstanceCompleted); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	if err := env.orch.forceStopInstance(wfiID); err == nil {
		t.Fatal("want error for a completed instance")
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceCompleted {
		t.Errorf("status changed to %v, want unchanged completed", wi.Status)
	}
}

// TestStop_PlanSuspendedInstance_NotInRuns_FallsBackAndCancelsDraft calls the
// exported Stop (not forceStopInstance directly) on a plan-suspended instance
// with no runState in o.runs — it must take the fallback path and produce the
// same end state as a direct forceStopInstance call.
func TestStop_PlanSuspendedInstance_NotInRuns_FallsBackAndCancelsDraft(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")
	if err := repo.NewWorkflowInstanceRepo(env.pool, clock.Real()).UpdateStatus(wfiID, model.WorkflowInstanceWaitingInput); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	appendDraftPlan(t, env, wfiID, validManifest("goal", "n/a"))

	env.orch.mu.Lock()
	_, inRuns := env.orch.runs[wfiID]
	env.orch.mu.Unlock()
	if inRuns {
		t.Fatalf("precondition failed: %s unexpectedly has a live runState", wfiID)
	}

	if err := env.orch.Stop(wfiID); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	head, err := repo.NewPlanRepo(env.pool, clock.Real()).GetHead(wfiID)
	if err != nil {
		t.Fatalf("GetHead: %v", err)
	}
	if head.Status != model.PlanStatusCancelled {
		t.Errorf("plan status = %v, want cancelled", head.Status)
	}
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceFailed {
		t.Errorf("instance status = %v, want failed", wi.Status)
	}
}
