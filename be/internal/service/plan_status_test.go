package service

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// TestDerivePlanInstanceStatus_NoPlanHead_Planning asserts the "no plan head
// at all" branch returns Planning.
func TestDerivePlanInstanceStatus_NoPlanHead_Planning(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)

	status, err := DerivePlanInstanceStatus(pool, clock.Real(), instanceID)
	if err != nil {
		t.Fatalf("DerivePlanInstanceStatus: %v", err)
	}
	if status != model.WorkflowInstancePlanning {
		t.Errorf("status = %q, want %q", status, model.WorkflowInstancePlanning)
	}
}

// TestDerivePlanInstanceStatus_DraftWithQuestions_WaitingInput asserts a draft
// head with open questions returns WaitingInput.
func TestDerivePlanInstanceStatus_DraftWithQuestions_WaitingInput(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	if _, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestWithQuestions("goal"),
	}); err != nil {
		t.Fatalf("revise: %v", err)
	}

	status, err := DerivePlanInstanceStatus(pool, clock.Real(), instanceID)
	if err != nil {
		t.Fatalf("DerivePlanInstanceStatus: %v", err)
	}
	if status != model.WorkflowInstanceWaitingInput {
		t.Errorf("status = %q, want %q", status, model.WorkflowInstanceWaitingInput)
	}
}

// TestDerivePlanInstanceStatus_DraftNoQuestions_WaitingApproval asserts an
// approvable draft (no open questions) returns WaitingApproval.
func TestDerivePlanInstanceStatus_DraftNoQuestions_WaitingApproval(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	if _, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	}); err != nil {
		t.Fatalf("revise: %v", err)
	}

	status, err := DerivePlanInstanceStatus(pool, clock.Real(), instanceID)
	if err != nil {
		t.Fatalf("DerivePlanInstanceStatus: %v", err)
	}
	if status != model.WorkflowInstanceWaitingApproval {
		t.Errorf("status = %q, want %q", status, model.WorkflowInstanceWaitingApproval)
	}
}

// TestDerivePlanInstanceStatus_ApprovedHead_StillPlanning documents/locks the
// actual (possibly surprising) function contract: an approved head is not
// draft and not "no head", so it falls into the `head.Status != draft` branch
// and returns Planning. The orchestrator boundary never calls
// DerivePlanInstanceStatus for an approved head anyway (it materializes
// instead), but the function itself must behave this way.
func TestDerivePlanInstanceStatus_ApprovedHead_StillPlanning(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if _, err := svc.Approve(instanceID, rev.Revision); err != nil {
		t.Fatalf("approve: %v", err)
	}

	status, err := DerivePlanInstanceStatus(pool, clock.Real(), instanceID)
	if err != nil {
		t.Fatalf("DerivePlanInstanceStatus: %v", err)
	}
	if status != model.WorkflowInstancePlanning {
		t.Errorf("status = %q, want %q (approved head is not draft, per the function's own contract)", status, model.WorkflowInstancePlanning)
	}
}

// TestDerivePlanInstanceStatus_RunningPlannerChild_Planning asserts a running
// `_planner` child session forces Planning regardless of the plan head's own
// state — a waiting_approval-shaped draft head is present too, to prove the
// running-planner check short-circuits before the head is even consulted.
func TestDerivePlanInstanceStatus_RunningPlannerChild_Planning(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	if _, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	}); err != nil {
		t.Fatalf("revise: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, status, created_at, updated_at)
		 VALUES ('sess-planner-running', ?, '', ?, 'plan', '_planner', 'planner', 'running', ?, ?)`,
		planTestProjectID, instanceID, now, now)

	status, err := DerivePlanInstanceStatus(pool, clock.Real(), instanceID)
	if err != nil {
		t.Fatalf("DerivePlanInstanceStatus: %v", err)
	}
	if status != model.WorkflowInstancePlanning {
		t.Errorf("status = %q, want %q (running planner child must short-circuit ahead of the draft-shaped head)", status, model.WorkflowInstancePlanning)
	}
}

// TestSetPlanInstanceStatus_OnlyOverwritesPlanSuspendedStatuses is table-driven
// over the guard: an 'active' status must never be clobbered by a
// plan-lifecycle write, while any plan-suspended status is freely overwritten.
func TestSetPlanInstanceStatus_OnlyOverwritesPlanSuspendedStatuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		start model.WorkflowInstanceStatus
		want  model.WorkflowInstanceStatus
	}{
		{name: "active_untouched", start: model.WorkflowInstanceActive, want: model.WorkflowInstanceActive},
		{name: "waiting_input_overwritten", start: model.WorkflowInstanceWaitingInput, want: model.WorkflowInstancePlanReady},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool, instanceID := setupPlanTestEnv(t)
			clk := clock.Real()
			mustExec(t, pool, `UPDATE workflow_instances SET status = ? WHERE id = ?`, string(tc.start), instanceID)

			if err := SetPlanInstanceStatus(pool, clk, instanceID, model.WorkflowInstancePlanReady); err != nil {
				t.Fatalf("SetPlanInstanceStatus: %v", err)
			}

			wi, err := repo.NewWorkflowInstanceRepo(pool, clk).Get(instanceID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if wi.Status != tc.want {
				t.Errorf("status after SetPlanInstanceStatus(from %q) = %q, want %q", tc.start, wi.Status, tc.want)
			}
		})
	}
}
