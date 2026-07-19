package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// addPremiumFanoutTemplate is addFanoutTemplate's premium-model counterpart:
// a fanout_template def bound to opus-4-8 (ModelTierPremium), so a manifest
// bound to it counts against dynwf_max_premium_workers.
func addPremiumFanoutTemplate(t *testing.T, env *testEnv, workflowID, templateID string) {
	t.Helper()
	now := clock.Real().Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	_, err := env.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, node_role, model, created_at, updated_at)
		 VALUES (?, ?, ?, 'fanout_template', 'opus-4-8', ?, ?)`,
		templateID, env.project, workflowID, now, now)
	if err != nil {
		t.Fatalf("insert premium fanout_template def %s: %v", templateID, err)
	}
}

// premiumHeavyManifest builds a 10-node, 3-dense-layer manifest (5+4+1 nodes)
// with every node bound to premiumTemplate, mirroring
// service's own plan_validate_premium_test.go fixture of the same name — the
// final single-node layer is the result-carrying node.
func premiumHeavyManifest(premiumTemplate string) service.PlanManifest {
	mkNodes := func(prefix string, n int) []service.PlanNode {
		nodes := make([]service.PlanNode, n)
		for i := 0; i < n; i++ {
			nodes[i] = service.PlanNode{ID: prefix + string(rune('a'+i)), Template: premiumTemplate, Instructions: "do work"}
		}
		return nodes
	}
	return service.PlanManifest{
		Version: 1,
		Goal:    "premium heavy",
		Layers: []service.PlanLayer{
			{Layer: 0, Policy: "all", Nodes: mkNodes("l0", 5)},
			{Layer: 1, Policy: "all", Nodes: mkNodes("l1", 4)},
			{Layer: 2, Policy: "any", Nodes: mkNodes("l2", 1)},
		},
	}
}

// TestApprovePlan_PremiumHeavyManifest_RejectedInteractively covers the
// interactive-approval reject seam end-to-end through the orchestrator's own
// ApprovePlan (ownership-guarded, WS-broadcasting) entry point: a manifest
// binding more nodes than dynwf_max_premium_workers to a premium-tier
// template must be rejected, leaving the plan in draft with nothing
// materialized.
func TestApprovePlan_PremiumHeavyManifest_RejectedInteractively(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "worker-template")
	addPremiumFanoutTemplate(t, env, "test", "opus-worker")
	childID := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, childID, "parent-1", "waiting_approval", "")

	rev := appendDraftPlan(t, env, childID, premiumHeavyManifest("opus-worker"))

	_, err := env.orch.ApprovePlan(context.Background(), "parent-1", env.project, childID, rev)
	if err == nil {
		t.Fatal("want a reject error for a manifest binding 10 nodes to a premium template over the default cap of 2")
	}
	if !strings.Contains(err.Error(), service.PremiumWorkerCapKey) {
		t.Errorf("error %q does not mention %q", err.Error(), service.PremiumWorkerCapKey)
	}

	draft, derr := service.NewPlanService(env.pool, clock.Real(), nil).GetDraft(childID)
	if derr != nil {
		t.Fatalf("GetDraft: %v", derr)
	}
	if draft.Head.Status != model.PlanStatusDraft {
		t.Errorf("Head.Status = %q after a rejected approve, want unchanged %q", draft.Head.Status, model.PlanStatusDraft)
	}

	nodes, nerr := repo.NewInstanceNodeRepo(env.pool, clock.Real()).List(childID)
	if nerr != nil {
		t.Fatalf("List instance nodes: %v", nerr)
	}
	if len(nodes) != 0 {
		t.Errorf("materialized node count = %d, want 0 (a rejected approve must not materialize anything)", len(nodes))
	}
}

// TestApproveAuto_PremiumHeavyManifest_DowngradesMaterializesAndWritesFinding
// covers the mode=auto downgrade seam end-to-end: PlanService.ApproveAuto (the
// method draftPlanAndProceed calls for mode=auto — see plan_boundary.go) is
// exercised directly against a real DB, asserting the full observable
// contract: the plan approves, materializes with the premium node count
// capped, and a _plan_premium_downgrade warning finding lands on the
// instance. Called directly rather than through a live run (planner CLI
// spawning is forbidden in this suite — see
// TestReloadPlanLayers_NoPlanHead_DraftAttemptFailsWithoutPlannerCLI).
func TestApproveAuto_PremiumHeavyManifest_DowngradesMaterializesAndWritesFinding(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "worker-template")
	addPremiumFanoutTemplate(t, env, "test", "opus-worker")
	childID := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, childID, "parent-1", "waiting_approval", "")

	rev := appendDraftPlan(t, env, childID, premiumHeavyManifest("opus-worker"))

	planSvc := service.NewPlanService(env.pool, clock.Real(), nil)
	approved, err := planSvc.ApproveAuto(childID, rev)
	if err != nil {
		t.Fatalf("ApproveAuto: %v", err)
	}
	if approved.Revision != rev+1 {
		t.Errorf("approved.Revision = %d, want %d (a downgrade revision must be appended)", approved.Revision, rev+1)
	}

	findings, ferr := repo.NewFindingRepo(env.pool, clock.Real()).GetOwn("workflow_instance", childID)
	if ferr != nil {
		t.Fatalf("GetOwn: %v", ferr)
	}
	raw, ok := findings["_plan_premium_downgrade"]
	if !ok {
		t.Fatal("expected a _plan_premium_downgrade finding on the workflow_instance")
	}
	var val struct {
		Cap        int      `json:"cap"`
		Downgraded []string `json:"downgraded"`
	}
	if err := json.Unmarshal(raw, &val); err != nil {
		t.Fatalf("unmarshal _plan_premium_downgrade: %v", err)
	}
	if val.Cap != service.DefaultDynwfMaxPremiumWorkers {
		t.Errorf("finding cap = %d, want %d", val.Cap, service.DefaultDynwfMaxPremiumWorkers)
	}
	if len(val.Downgraded) != 8 {
		t.Errorf("finding downgraded = %v (len %d), want 8", val.Downgraded, len(val.Downgraded))
	}

	nodes, nerr := repo.NewInstanceNodeRepo(env.pool, clock.Real()).List(childID)
	if nerr != nil {
		t.Fatalf("List instance nodes: %v", nerr)
	}
	if len(nodes) != 10 {
		t.Fatalf("materialized node count = %d, want 10 (downgrade rebinds templates, never drops nodes)", len(nodes))
	}
	var premiumCount int
	for _, n := range nodes {
		if n.AgentType == "opus-worker" {
			premiumCount++
		}
	}
	if premiumCount != service.DefaultDynwfMaxPremiumWorkers {
		t.Errorf("materialized premium node count = %d, want %d", premiumCount, service.DefaultDynwfMaxPremiumWorkers)
	}
}
