package orchestrator

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// TestReloadPlanLayers_CancelledPlan_MarksFailed: a cancelled plan head fails
// the run (worktreeHandled=false — markFailed's own path handles cleanup).
func TestReloadPlanLayers_CancelledPlan_MarksFailed(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl")
	env.createTicket(t, "PB-4", "cancelled plan")
	wfiID := env.initWorkflow(t, "PB-4")

	appendDraftPlan(t, env, wfiID, validManifest("do the thing", "fanout-tmpl"))
	if err := repo.NewPlanRepo(env.pool, clock.Real()).Cancel(wfiID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	svcWf, workflows, agents := buildPlanReloadInputs(t, env, "test")
	layerGroups := groupPhasesByLayer(svcWf.Phases)
	req := RunRequest{ProjectID: env.project, TicketID: "PB-4", WorkflowName: "test", ScopeType: "ticket"}

	_, extended, terminal, worktreeHandled := env.orch.reloadPlanLayers(
		context.Background(), wfiID, req, env.pool, svcWf, layerGroups, map[int]string{}, workflows, agents)

	if extended || !terminal || worktreeHandled {
		t.Fatalf("got (extended=%v, terminal=%v, worktreeHandled=%v), want (false, true, false)", extended, terminal, worktreeHandled)
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceFailed {
		t.Errorf("status = %v, want failed", wi.Status)
	}
}

// TestReloadPlanLayers_ApprovedPlan_MaterializesAndSplices: an approved plan
// is materialized (idempotently) and spliced into the returned layer groups;
// a second call must not duplicate workflow_instance_nodes rows.
func TestReloadPlanLayers_ApprovedPlan_MaterializesAndSplices(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl")
	env.createTicket(t, "PB-5", "approved plan")
	wfiID := env.initWorkflow(t, "PB-5")
	ch := env.subscribeWSClient(t, "ws-pb-5", "PB-5")

	rev := appendDraftPlan(t, env, wfiID, validManifest("do the thing", "fanout-tmpl"))
	planRepo := repo.NewPlanRepo(env.pool, clock.Real())
	if err := planRepo.Approve(wfiID, rev); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	svcWf, workflows, agents := buildPlanReloadInputs(t, env, "test")
	layerGroups := groupPhasesByLayer(svcWf.Phases)
	layerPolicies := map[int]string{}
	req := RunRequest{ProjectID: env.project, TicketID: "PB-5", WorkflowName: "test", ScopeType: "ticket"}

	newGroups, extended, terminal, worktreeHandled := env.orch.reloadPlanLayers(
		context.Background(), wfiID, req, env.pool, svcWf, layerGroups, layerPolicies, workflows, agents)

	if !extended || terminal || worktreeHandled {
		t.Fatalf("got (extended=%v, terminal=%v, worktreeHandled=%v), want (true, false, false)", extended, terminal, worktreeHandled)
	}

	found := false
	for _, lg := range newGroups {
		for _, p := range lg.phases {
			if p.NodeID == "step1" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("newGroups = %+v, want to contain materialized node step1", newGroups)
	}
	expectEvent(t, ch, ws.EventPlanMaterialized, 2*time.Second)

	nodeCountBefore := countInstanceNodes(t, env, wfiID)

	_, extended2, terminal2, _ := env.orch.reloadPlanLayers(
		context.Background(), wfiID, req, env.pool, svcWf, layerGroups, layerPolicies, workflows, agents)
	if !extended2 || terminal2 {
		t.Fatalf("second call: got (extended=%v, terminal=%v), want (true, false)", extended2, terminal2)
	}

	nodeCountAfter := countInstanceNodes(t, env, wfiID)
	if nodeCountAfter != nodeCountBefore {
		t.Errorf("node count changed on idempotent re-materialize: before=%d after=%d", nodeCountBefore, nodeCountAfter)
	}
}

// TestDraftPlanAndProceed_ClaimedBoundary_MaterializesDespiteRevisePlanError
// is the regression test for the live-boundary claim: an approved plan head
// (simulating a concurrent Approve that landed while the inline self-draft
// was in flight) plus planApprovedAtBoundary pre-seeded on the run's
// registered runState. Approving the head first makes the inline
// Revise(revision=0) call itself fail fast with ErrPlanNotDraft (head.Status
// != draft) — RunPlanner has no injectable mock, so the success variant of
// this race cannot be driven directly, but the error path exercises the same
// claimed branch without ever needing a real planner CLI. The naive fix
// (checking DB status instead of the live boundary claim) would strand this
// run: draftPlanAndProceed must still flip the instance to active and
// materialize+splice instead of markFailed/suspending.
func TestDraftPlanAndProceed_ClaimedBoundary_MaterializesDespiteRevisePlanError(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl")
	env.createTicket(t, "PB-7", "claimed boundary despite draft error")
	wfiID := env.initWorkflow(t, "PB-7")
	ch := env.subscribeWSClient(t, "ws-pb-7", "PB-7")

	rev := appendDraftPlan(t, env, wfiID, validManifest("do the thing", "fanout-tmpl"))
	planRepo := repo.NewPlanRepo(env.pool, clock.Real())
	if err := planRepo.Approve(wfiID, rev); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	rs := registerBoundaryRun(t, env, wfiID, false)
	rs.planApprovedAtBoundary = true // simulate a claim that landed before this call

	svcWf, workflows, agents := buildPlanReloadInputs(t, env, "test")
	layerGroups := groupPhasesByLayer(svcWf.Phases)
	layerPolicies := map[int]string{}
	req := RunRequest{ProjectID: env.project, TicketID: "PB-7", WorkflowName: "test", ScopeType: "ticket"}

	_, _, defProjectID, err := env.orch.resolveWorkflowDef(env.pool, env.project, "test")
	if err != nil {
		t.Fatalf("resolveWorkflowDef: %v", err)
	}

	newGroups, extended, terminal, worktreeHandled := env.orch.draftPlanAndProceed(
		context.Background(), wfiID, req, env.pool, svcWf, layerGroups, layerPolicies, workflows, agents, defProjectID)

	if !extended || terminal || worktreeHandled {
		t.Fatalf("got (extended=%v, terminal=%v, worktreeHandled=%v), want (true, false, false)", extended, terminal, worktreeHandled)
	}

	found := false
	for _, lg := range newGroups {
		for _, p := range lg.phases {
			if p.NodeID == "step1" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("newGroups = %+v, want to contain materialized node step1", newGroups)
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("status = %v, want active", wi.Status)
	}
	if countInstanceNodes(t, env, wfiID) == 0 {
		t.Errorf("want workflow_instance_nodes materialized, got none")
	}
	expectEvent(t, ch, ws.EventPlanMaterialized, 2*time.Second)
}

func countInstanceNodes(t *testing.T, env *testEnv, wfiID string) int {
	t.Helper()
	var n int
	if err := env.pool.QueryRow(`SELECT COUNT(*) FROM workflow_instance_nodes WHERE instance_id = ?`, wfiID).Scan(&n); err != nil {
		t.Fatalf("count workflow_instance_nodes: %v", err)
	}
	return n
}

// TestReloadPlanLayers_MaterializeFails_MarksFailed: the referenced template
// is deleted after approval (a second, unreferenced template keeps the
// workflow plan-driven so IsPlanDriven itself doesn't short-circuit to a
// no-op) — Materialize's re-validation must fail and the run must fail.
func TestReloadPlanLayers_MaterializeFails_MarksFailed(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl")
	addFanoutTemplate(t, env, "test", "keep-driven")
	env.createTicket(t, "PB-6", "materialize fails")
	wfiID := env.initWorkflow(t, "PB-6")

	rev := appendDraftPlan(t, env, wfiID, validManifest("do the thing", "fanout-tmpl"))
	planRepo := repo.NewPlanRepo(env.pool, clock.Real())
	if err := planRepo.Approve(wfiID, rev); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	if _, err := env.pool.Exec(
		`DELETE FROM agent_definitions WHERE project_id = ? AND workflow_id = 'test' AND id = 'fanout-tmpl'`,
		env.project); err != nil {
		t.Fatalf("delete referenced template: %v", err)
	}

	svcWf, workflows, agents := buildPlanReloadInputs(t, env, "test")
	layerGroups := groupPhasesByLayer(svcWf.Phases)
	req := RunRequest{ProjectID: env.project, TicketID: "PB-6", WorkflowName: "test", ScopeType: "ticket"}

	_, extended, terminal, worktreeHandled := env.orch.reloadPlanLayers(
		context.Background(), wfiID, req, env.pool, svcWf, layerGroups, map[int]string{}, workflows, agents)

	if extended || !terminal || worktreeHandled {
		t.Fatalf("got (extended=%v, terminal=%v, worktreeHandled=%v), want (false, true, false)", extended, terminal, worktreeHandled)
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceFailed {
		t.Errorf("status = %v, want failed", wi.Status)
	}
}
