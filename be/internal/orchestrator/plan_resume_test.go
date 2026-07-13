package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// TestResumeAfterPlanApproval_AlreadyActive_Noop: an already-active instance
// is left untouched — the boundary inside that active run's own runLoop
// materializes inline instead.
func TestResumeAfterPlanApproval_AlreadyActive_Noop(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "PR-1", "already active")
	wfiID := env.initWorkflow(t, "PR-1")

	if err := env.orch.ResumeAfterPlanApproval(context.Background(), wfiID); err != nil {
		t.Fatalf("ResumeAfterPlanApproval: %v", err)
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("status = %v, want unchanged active", wi.Status)
	}
	env.orch.mu.Lock()
	_, running := env.orch.runs[wfiID]
	env.orch.mu.Unlock()
	if running {
		t.Errorf("no-op resume must not register a runState")
	}
}

// TestResumeAfterPlanApproval_NotPlanSuspended_Errors: a terminal, non-plan
// status (e.g. completed) is rejected.
func TestResumeAfterPlanApproval_NotPlanSuspended_Errors(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "PR-2", "not plan suspended")
	wfiID := env.initWorkflow(t, "PR-2")
	if err := repo.NewWorkflowInstanceRepo(env.pool, clock.Real()).UpdateStatus(wfiID, model.WorkflowInstanceCompleted); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	if err := env.orch.ResumeAfterPlanApproval(context.Background(), wfiID); err == nil {
		t.Fatal("want error for non-plan-suspended instance")
	}
}

// TestResumeAfterPlanApproval_NoMaterializedNodes_Errors: plan-suspended but
// nothing has been materialized yet.
func TestResumeAfterPlanApproval_NoMaterializedNodes_Errors(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl")
	env.createTicket(t, "PR-3", "no materialized nodes")
	wfiID := env.initWorkflow(t, "PR-3")
	if err := repo.NewWorkflowInstanceRepo(env.pool, clock.Real()).UpdateStatus(wfiID, model.WorkflowInstanceWaitingApproval); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	err := env.orch.ResumeAfterPlanApproval(context.Background(), wfiID)
	if err == nil || !strings.Contains(err.Error(), "no materialized nodes") {
		t.Fatalf("err = %v, want mention of 'no materialized nodes'", err)
	}
}

// TestResumeAfterPlanApproval_HappyPath_RelaunchesAtMinMaterializedLayer
// materializes a plan directly (bypassing reloadPlanLayers) then resumes: a
// fresh runState must be registered and EventWorkflowResumed broadcast. The
// relaunched runLoop will attempt to spawn a real agent for the materialized
// fanout_template node (materialized nodes never enter agentTags — see
// orchestrator_skip.go buildAgentTags — so skip_tags cannot suppress it);
// stop immediately, this test only asserts the resume/relaunch mechanics.
func TestResumeAfterPlanApproval_HappyPath_RelaunchesAtMinMaterializedLayer(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl")
	env.createTicket(t, "PR-4", "happy path resume")
	wfiID := env.initWorkflow(t, "PR-4")
	ch := env.subscribeWSClient(t, "ws-pr-4", "PR-4")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	if err := wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceWaitingApproval); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	rev := appendDraftPlan(t, env, wfiID, validManifest("do the thing", "fanout-tmpl"))
	planRepo := repo.NewPlanRepo(env.pool, clock.Real())
	if err := planRepo.Approve(wfiID, rev); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := service.NewPlanService(env.pool, clock.Real(), env.orch).Materialize(wfiID); err != nil {
		t.Fatalf("Materialize: %v", err)
	}

	if err := env.orch.ResumeAfterPlanApproval(context.Background(), wfiID); err != nil {
		t.Fatalf("ResumeAfterPlanApproval: %v", err)
	}

	env.orch.mu.Lock()
	_, running := env.orch.runs[wfiID]
	env.orch.mu.Unlock()
	if !running {
		t.Errorf("want a runState registered for %s after resume", wfiID)
	}

	expectEvent(t, ch, ws.EventWorkflowResumed, 2*time.Second)

	env.stopAndWaitRun(t, wfiID)
}
