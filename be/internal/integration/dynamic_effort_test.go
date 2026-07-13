package integration

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// TestPlanMaterialize_ReasoningEffort_DistinctOverridesPerTemplate is the
// DYNWF-8 acceptance test: two fanout_template agent definitions bound to
// the SAME cli_models row ("sonnet") but with different reasoning_effort
// overrides are both referenced by a materialized plan; each materialized
// node's resolved SpawnerAgentConfig (service.LoadMaterializedAgentConfigs
// -- the exact call orchestrator.materializeAndSplice/runLoop/plan_boundary.go
// makes to build the spawner's Config.Agents map before spawning) must carry
// its own def's reasoning_effort, not a value bled from the other template
// or the shared model row.
func TestPlanMaterialize_ReasoningEffort_DistinctOverridesPerTemplate(t *testing.T) {
	// Not t.Parallel(): NewTestEnv uses t.Setenv, which panics under
	// t.Parallel() -- matches every other test in this package.
	env := NewTestEnv(t)

	if _, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "plan-effort-e2e",
		Description: "plan materialize reasoning_effort e2e",
	}); err != nil {
		t.Fatalf("create workflow def: %v", err)
	}

	adSvc := env.getAgentDefService(t)
	highEffort := "high"
	lowEffort := "low"
	if _, err := adSvc.CreateAgentDef(env.ProjectID, "plan-effort-e2e", &types.AgentDefCreateRequest{
		ID:              "worker-high",
		Prompt:          "do work carefully",
		Layer:           0,
		NodeRole:        "fanout_template",
		Description:     "high-effort worker template",
		Model:           "sonnet",
		ExecutionMode:   "cli_interactive",
		ReasoningEffort: &highEffort,
	}); err != nil {
		t.Fatalf("create worker-high fanout_template def: %v", err)
	}
	if _, err := adSvc.CreateAgentDef(env.ProjectID, "plan-effort-e2e", &types.AgentDefCreateRequest{
		ID:              "worker-low",
		Prompt:          "do work quickly",
		Layer:           0,
		NodeRole:        "fanout_template",
		Description:     "low-effort worker template",
		Model:           "sonnet",
		ExecutionMode:   "cli_interactive",
		ReasoningEffort: &lowEffort,
	}); err != nil {
		t.Fatalf("create worker-low fanout_template def: %v", err)
	}

	const ticketID = "PLAN-EFFORT-1"
	env.CreateTicket(t, ticketID, "plan effort e2e")
	wi, err := env.WorkflowSvc.Init(env.ProjectID, ticketID, &types.WorkflowInitRequest{Workflow: "plan-effort-e2e"})
	if err != nil {
		t.Fatalf("init workflow: %v", err)
	}
	wfiID := wi.ID

	manifest := service.PlanManifest{
		Version: 1,
		Goal:    "exercise two templates on the same model row with different efforts",
		Layers: []service.PlanLayer{
			{Layer: 0, Policy: "all", Nodes: []service.PlanNode{
				{ID: "step-high", Template: "worker-high", Instructions: "careful pass"},
				{ID: "step-low", Template: "worker-low", Instructions: "quick pass"},
			}},
			{Layer: 1, Policy: "all", Nodes: []service.PlanNode{
				{ID: "reduce", Template: "worker-high", Instructions: "merge #{NODE_FINDINGS:step-high} and #{NODE_FINDINGS:step-low}"},
			}},
		},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	svc := service.NewPlanService(env.Pool, env.Clock, nil)
	ctx := context.Background()
	rev, err := svc.Revise(ctx, wfiID, types.PlanReviseRequest{Revision: 0, Manifest: raw})
	if err != nil {
		t.Fatalf("Revise: %v", err)
	}
	if _, err := svc.Approve(wfiID, rev.Revision); err != nil {
		t.Fatalf("Approve (auto-materializes): %v", err)
	}

	materializedPhases, _, err := service.LoadInstanceNodePhases(env.Pool, env.Clock, wfiID)
	if err != nil {
		t.Fatalf("LoadInstanceNodePhases: %v", err)
	}
	if len(materializedPhases) != 3 {
		t.Fatalf("materialized phases = %d, want 3", len(materializedPhases))
	}

	agents := service.LoadMaterializedAgentConfigs(env.Pool, env.Clock, env.ProjectID, "plan-effort-e2e", materializedPhases)

	highCfg, ok := agents["worker-high"]
	if !ok {
		t.Fatal("LoadMaterializedAgentConfigs missing worker-high")
	}
	lowCfg, ok := agents["worker-low"]
	if !ok {
		t.Fatal("LoadMaterializedAgentConfigs missing worker-low")
	}

	if highCfg.Model != "sonnet" || lowCfg.Model != "sonnet" {
		t.Fatalf("both templates must resolve the same model row: worker-high.Model=%q worker-low.Model=%q", highCfg.Model, lowCfg.Model)
	}
	if highCfg.ReasoningEffort == nil || *highCfg.ReasoningEffort != "high" {
		t.Errorf("worker-high SpawnerAgentConfig.ReasoningEffort = %v, want \"high\"", highCfg.ReasoningEffort)
	}
	if lowCfg.ReasoningEffort == nil || *lowCfg.ReasoningEffort != "low" {
		t.Errorf("worker-low SpawnerAgentConfig.ReasoningEffort = %v, want \"low\"", lowCfg.ReasoningEffort)
	}
	if highCfg.ReasoningEffort != nil && lowCfg.ReasoningEffort != nil && *highCfg.ReasoningEffort == *lowCfg.ReasoningEffort {
		t.Fatal("two distinct templates on the same model row resolved to the same effective reasoning_effort")
	}

	// Sanity: the materialized nodes carry the right agent_type per node id,
	// which is the key LoadMaterializedAgentConfigs above is indexed by.
	nodeRepo := repo.NewInstanceNodeRepo(env.Pool, env.Clock)
	nodes, err := nodeRepo.List(wfiID)
	if err != nil {
		t.Fatalf("List nodes: %v", err)
	}
	agentByNode := map[string]string{}
	for _, n := range nodes {
		agentByNode[n.NodeID] = n.AgentType
	}
	if agentByNode["step-high"] != "worker-high" || agentByNode["step-low"] != "worker-low" {
		t.Errorf("materialized node->agent mapping = %+v, want step-high->worker-high, step-low->worker-low", agentByNode)
	}
}
