package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// TestDynamicWorkflow_RevisePlanApprovePlan_MaterializesViaTool is the DYNWF-6
// tool-facing counterpart to TestPlanMaterialize_EndToEnd (plan_materialize_test.go):
// instead of driving service.PlanService directly, it drives the plan
// lifecycle through orchestrator.RevisePlan/ApprovePlan -- the exact methods
// behind the revise_plan/approve_plan builtins -- so the DYNWF-6 additions
// (ownership check, WS broadcast, Approve's auto-materialize) get real
// coverage. The instance is created directly via env.WorkflowSvc.Init and its
// parent_instance_id set by SQL, never via a real StartDynamicWorkflow/Start
// (which would hit the self-drafting plan boundary and try to spawn a real
// planner CLI subprocess -- see dynamic_workflow_2_test.go and CLAUDE.md for
// why that path is forbidden here). The instance is kept 'active' throughout
// (never plan-suspended), so ApprovePlan's resume-trigger never fires --
// that path is exercised separately, and safely, in
// TestDynamicWorkflow_ApprovePlan_ResumesPlanSuspendedChild.
func TestDynamicWorkflow_RevisePlanApprovePlan_MaterializesViaTool(t *testing.T) {
	env := NewTestEnv(t)

	if _, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "dynwf-tool-e2e",
		Description: "dynamic workflow tool-driven plan lifecycle",
	}); err != nil {
		t.Fatalf("create workflow def: %v", err)
	}
	if _, err := env.getAgentDefService(t).CreateAgentDef(env.ProjectID, "dynwf-tool-e2e", &types.AgentDefCreateRequest{
		ID:            "worker-template",
		Prompt:        "do work",
		Layer:         0,
		NodeRole:      "fanout_template",
		Description:   "worker template",
		Model:         "sonnet-5",
		ExecutionMode: "cli_interactive",
	}); err != nil {
		t.Fatalf("create fanout_template def: %v", err)
	}

	const ticketID = "DYNWF-TOOL-1"
	env.CreateTicket(t, ticketID, "dynamic workflow tool e2e")
	wi, err := env.WorkflowSvc.Init(env.ProjectID, ticketID, &types.WorkflowInitRequest{Workflow: "dynwf-tool-e2e"})
	if err != nil {
		t.Fatalf("init workflow: %v", err)
	}
	wfiID := wi.ID

	const callerID = "caller-dynwf-tool-1"
	if _, err := env.Pool.Exec(`UPDATE workflow_instances SET parent_instance_id = ? WHERE id = ?`, callerID, wfiID); err != nil {
		t.Fatalf("set parent_instance_id: %v", err)
	}

	orch := orchestrator.New(env.Pool.Path, env.Hub, env.Clock, nil, "")
	ctx := context.Background()
	_, ch := env.NewWSClient(t, "ws-dynwf-tool-1", ticketID)

	manifest := buildSingleNodeManifest("dynamic workflow tool e2e", "worker-template")
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	// Ownership: a caller id that is not the persisted parent_instance_id must
	// be refused by both RevisePlan and (below) ApprovePlan.
	if _, err := orch.RevisePlan(ctx, "someone-else", env.ProjectID, wfiID, types.PlanReviseRequest{Revision: 0, Manifest: raw}); err == nil {
		t.Fatal("RevisePlan: want error for non-owning caller")
	}

	rev, err := orch.RevisePlan(ctx, callerID, env.ProjectID, wfiID, types.PlanReviseRequest{Revision: 0, Manifest: raw})
	if err != nil {
		t.Fatalf("RevisePlan: %v", err)
	}
	if rev.Revision != 1 {
		t.Errorf("revision = %d, want 1", rev.Revision)
	}
	if rev.Author != model.PlanAuthorCaller {
		t.Errorf("author = %q, want %q", rev.Author, model.PlanAuthorCaller)
	}
	expectEvent(t, ch, ws.EventPlanDrafted, 2*time.Second)

	if _, err := orch.ApprovePlan(ctx, "someone-else", env.ProjectID, wfiID, rev.Revision); err == nil {
		t.Fatal("ApprovePlan: want error for non-owning caller")
	}

	approved, err := orch.ApprovePlan(ctx, callerID, env.ProjectID, wfiID, rev.Revision)
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	if approved.Revision != rev.Revision {
		t.Errorf("approved revision = %d, want %d", approved.Revision, rev.Revision)
	}
	expectEvent(t, ch, ws.EventPlanApproved, 2*time.Second)
	expectEvent(t, ch, ws.EventPlanMaterialized, 2*time.Second)

	// The instance stayed 'active' throughout, so ApprovePlan must not have
	// invoked ResumeAfterPlanApproval -- no further WS event follows.
	expectNoEvent(t, ch, 200*time.Millisecond)

	// Approve already materializes: assert the node landed exactly as
	// TestPlanMaterialize_EndToEnd asserts for the direct-PlanService path.
	nodes, err := repo.NewInstanceNodeRepo(env.Pool, env.Clock).List(wfiID)
	if err != nil {
		t.Fatalf("List nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].NodeID != "step1" {
		t.Fatalf("materialized nodes = %+v, want single node 'step1'", nodes)
	}

	// Simulate the materialized node's completion via a direct session +
	// finding write (no real agent spawn), then read the final result back
	// via GetSubworkflow -- the same read path a run_subworkflow/
	// dynamic_workflow caller uses to poll for completion.
	sessionID := "sess-" + nodes[0].NodeID
	env.InsertAgentSession(t, sessionID, ticketID, wfiID, nodes[0].NodeID, nodes[0].AgentType, "")
	finalValue := json.RawMessage(`"step1-final"`)
	if err := repo.NewFindingRepo(env.Pool, env.Clock).Upsert("session", sessionID, "workflow_final_result", finalValue,
		repo.Denorm{ProjectID: env.ProjectID, WorkflowInstanceID: wfiID, AgentType: nodes[0].AgentType},
		repo.Actor{Source: "agent"},
	); err != nil {
		t.Fatalf("upsert workflow_final_result finding: %v", err)
	}
	env.CompleteAgentSession(t, sessionID, "pass")
	if err := repo.NewWorkflowInstanceRepo(env.Pool, env.Clock).UpdateStatus(wfiID, model.WorkflowInstanceCompleted); err != nil {
		t.Fatalf("UpdateStatus(completed): %v", err)
	}

	state, err := orch.GetSubworkflow(ctx, callerID, env.ProjectID, wfiID, "")
	if err != nil {
		t.Fatalf("GetSubworkflow: %v", err)
	}
	if state.Status != "completed" {
		t.Fatalf("GetSubworkflow status = %q, want completed", state.Status)
	}
	var got string
	if err := json.Unmarshal(state.Result, &got); err != nil {
		t.Fatalf("unmarshal GetSubworkflow result: %v", err)
	}
	if got != "step1-final" {
		t.Errorf("GetSubworkflow result = %q, want %q", got, "step1-final")
	}

	// GetSubworkflow enforces the same ownership check.
	if _, err := orch.GetSubworkflow(ctx, "someone-else", env.ProjectID, wfiID, ""); err == nil {
		t.Fatal("GetSubworkflow: want error for non-owning caller")
	}
}

// buildSingleNodeManifest builds a minimal single-layer, single-node manifest
// -- the multi-layer map/verify/reduce shape is already covered by
// TestPlanMaterialize_EndToEnd's buildMapVerifyReduceManifest; this test's
// focus is the RevisePlan/ApprovePlan tool methods, not manifest breadth.
func buildSingleNodeManifest(goal, templateID string) service.PlanManifest {
	return service.PlanManifest{
		Version: 1,
		Goal:    goal,
		Layers: []service.PlanLayer{
			{Layer: 0, Policy: "all", Nodes: []service.PlanNode{
				{ID: "step1", Template: templateID, Instructions: "do the thing"},
			}},
		},
	}
}
