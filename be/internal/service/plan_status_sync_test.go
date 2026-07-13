package service

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// TestRevise_SyncsSuspendedInstanceStatus is the regression guard for a run
// already suspended at the plan boundary: answering a draft's open questions
// (a revise that drops them) must move the instance from waiting_input to
// waiting_approval, otherwise a caller polling get_subworkflow never learns the
// plan became approvable and the run is a dead end.
func TestRevise_SyncsSuspendedInstanceStatus(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	clk := clock.Real()
	svc := NewPlanService(pool, clk, nil)
	ctx := context.Background()

	// Draft with open questions -> the boundary would suspend as waiting_input.
	rev, err := svc.Revise(ctx, instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestWithQuestions("goal"),
	})
	if err != nil {
		t.Fatalf("revise (with questions): %v", err)
	}
	mustExec(t, pool, `UPDATE workflow_instances SET status = ? WHERE id = ?`,
		string(model.WorkflowInstanceWaitingInput), instanceID)

	// Questions answered: revise to a manifest with none.
	if _, err := svc.Revise(ctx, instanceID, types.PlanReviseRequest{
		Revision: rev.Revision, Manifest: validPlanManifestJSON("goal", "do it"),
	}); err != nil {
		t.Fatalf("revise (answered): %v", err)
	}

	wi, err := repo.NewWorkflowInstanceRepo(pool, clk).Get(instanceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if wi.Status != model.WorkflowInstanceWaitingApproval {
		t.Errorf("status after answering questions = %q, want %q", wi.Status, model.WorkflowInstanceWaitingApproval)
	}
}

// TestRevise_DoesNotClobberActiveInstanceStatus locks the other half of the
// guard: a plan drafted while the run is still executing its static layers
// (status 'active') must not have its status rewritten to a plan status.
func TestRevise_DoesNotClobberActiveInstanceStatus(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	clk := clock.Real()
	svc := NewPlanService(pool, clk, nil)

	mustExec(t, pool, `UPDATE workflow_instances SET status = ? WHERE id = ?`,
		string(model.WorkflowInstanceActive), instanceID)

	if _, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	}); err != nil {
		t.Fatalf("revise: %v", err)
	}

	wi, err := repo.NewWorkflowInstanceRepo(pool, clk).Get(instanceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("status of an actively-running instance = %q, want %q (must not be clobbered)", wi.Status, model.WorkflowInstanceActive)
	}
}
