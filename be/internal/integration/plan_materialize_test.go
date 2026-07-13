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
)

// TestPlanMaterialize_EndToEnd is the headline DYNWF-5 acceptance test: a
// multi-layer plan (3-way map fan-out -> quorum verify -> single reduce) is
// revised+approved through the real PlanService (Approve auto-materializes),
// its nodes' completion simulated via direct session+finding writes (no real
// agent spawn), and the reduce node's result read back exactly as a
// run_subworkflow caller would via orchestrator.GetSubworkflow.
func TestPlanMaterialize_EndToEnd(t *testing.T) {
	// Not t.Parallel(): NewTestEnv uses t.Setenv (NRFLO_SOCKET), which panics
	// under t.Parallel() -- matches every other test in this package.
	env := NewTestEnv(t)

	// A dedicated workflow with NO static agent defs (maxStaticExecutableLayer
	// == -1) so manifest layer L maps 1:1 onto engine layer L -- keeps the
	// layer-offset math trivial for this test.
	if _, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "plan-e2e",
		Description: "plan materialize e2e",
	}); err != nil {
		t.Fatalf("create workflow def: %v", err)
	}
	if _, err := env.getAgentDefService(t).CreateAgentDef(env.ProjectID, "plan-e2e", &types.AgentDefCreateRequest{
		ID:            "worker-template",
		Prompt:        "do work",
		Layer:         0,
		NodeRole:      "fanout_template",
		Model:         "sonnet",
		ExecutionMode: "cli_interactive",
	}); err != nil {
		t.Fatalf("create fanout_template def: %v", err)
	}

	const ticketID = "PLAN-E2E-1"
	env.CreateTicket(t, ticketID, "plan e2e")
	wi, err := env.WorkflowSvc.Init(env.ProjectID, ticketID, &types.WorkflowInitRequest{Workflow: "plan-e2e"})
	if err != nil {
		t.Fatalf("init workflow: %v", err)
	}
	wfiID := wi.ID

	manifest := buildMapVerifyReduceManifest()
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

	nodeRepo := repo.NewInstanceNodeRepo(env.Pool, env.Clock)

	// 1. Materialized nodes: 7 rows, correct layer/agent_type/instructions.
	nodes, err := nodeRepo.List(wfiID)
	if err != nil {
		t.Fatalf("List nodes: %v", err)
	}
	assertMaterializedNodes(t, nodes, manifest)

	// 2. Materialized layer policies.
	policies, err := nodeRepo.ListLayerPolicies(wfiID)
	if err != nil {
		t.Fatalf("ListLayerPolicies: %v", err)
	}
	wantPolicies := map[int]string{0: "all", 1: "quorum:2", 2: "all"}
	for layer, want := range wantPolicies {
		if got := policies[layer]; got != want {
			t.Errorf("layer %d policy = %q, want %q", layer, got, want)
		}
	}

	// 3. Idempotence: re-materializing does not duplicate rows.
	if _, err := svc.Materialize(wfiID); err != nil {
		t.Fatalf("re-Materialize: %v", err)
	}
	nodesAgain, err := nodeRepo.List(wfiID)
	if err != nil {
		t.Fatalf("List nodes (again): %v", err)
	}
	if len(nodesAgain) != 7 {
		t.Fatalf("nodes after re-materialize = %d, want 7 (idempotent)", len(nodesAgain))
	}

	// 4/5. Simulate each node's completion via a direct session + finding
	// write (no real agent spawn), then assert node-scoped read-back.
	simulateNodeCompletions(t, env, ticketID, wfiID, nodes)

	// 6. The reduce node's result is readable via GetSubworkflow, exactly as a
	// run_subworkflow caller would read it back.
	assertReduceResultViaGetSubworkflow(t, env, wfiID)

	// 7. Persisted quorum policy string round-trips through the parser (the
	// aggregation logic itself is orchestrator-internal, covered elsewhere).
	lp, err := service.ParseLayerPolicy(policies[1])
	if err != nil {
		t.Fatalf("ParseLayerPolicy(%q): %v", policies[1], err)
	}
	if got := lp.Required(3); got != 2 {
		t.Errorf("quorum:2 Required(3) = %d, want 2", got)
	}
}

// buildMapVerifyReduceManifest builds a 3-layer plan manifest by hand: a
// 3-way map fan-out (policy "all"), a 3-way quorum:2 verify layer whose nodes
// each reference #{NODE_FINDINGS:map-1} (exercising the DYNWF-3 node-findings
// reference validator at revise time -- the placeholder itself is only
// expanded at real spawn time by spawner/template.go, not exercised here),
// and a single reduce node (required by the final-layer-exactly-one-node
// rule).
func buildMapVerifyReduceManifest() service.PlanManifest {
	return service.PlanManifest{
		Version: 1,
		Goal:    "map-verify-reduce widget",
		Layers: []service.PlanLayer{
			{
				Layer:  0,
				Policy: "all",
				Nodes: []service.PlanNode{
					{ID: "map-1", Template: "worker-template", Instructions: "map chunk 1"},
					{ID: "map-2", Template: "worker-template", Instructions: "map chunk 2"},
					{ID: "map-3", Template: "worker-template", Instructions: "map chunk 3"},
				},
			},
			{
				Layer:  1,
				Policy: "quorum:2",
				Nodes: []service.PlanNode{
					{ID: "verify-1", Template: "worker-template", Instructions: "verify using #{NODE_FINDINGS:map-1}"},
					{ID: "verify-2", Template: "worker-template", Instructions: "verify using #{NODE_FINDINGS:map-1}"},
					{ID: "verify-3", Template: "worker-template", Instructions: "verify using #{NODE_FINDINGS:map-1}"},
				},
			},
			{
				Layer:  2,
				Policy: "all",
				Nodes: []service.PlanNode{
					{ID: "reduce-1", Template: "worker-template", Instructions: "reduce all verified results"},
				},
			},
		},
	}
}

// assertMaterializedNodes checks the 7 materialized rows against the source
// manifest: total count, per-node layer (no offset since the workflow has no
// static defs), agent_type, and instructions surviving verbatim.
func assertMaterializedNodes(t *testing.T, nodes []model.InstanceNode, manifest service.PlanManifest) {
	t.Helper()
	if len(nodes) != 7 {
		t.Fatalf("materialized nodes = %d, want 7", len(nodes))
	}

	type want struct {
		layer        int
		instructions string
	}
	wantByID := make(map[string]want)
	for _, layer := range manifest.Layers {
		for _, n := range layer.Nodes {
			wantByID[n.ID] = want{layer: layer.Layer, instructions: n.Instructions}
		}
	}

	seen := make(map[string]bool)
	for _, n := range nodes {
		w, ok := wantByID[n.NodeID]
		if !ok {
			t.Errorf("unexpected materialized node id %q", n.NodeID)
			continue
		}
		seen[n.NodeID] = true
		if n.Layer != w.layer {
			t.Errorf("node %q layer = %d, want %d", n.NodeID, n.Layer, w.layer)
		}
		if n.AgentType != "worker-template" {
			t.Errorf("node %q agent_type = %q, want %q", n.NodeID, n.AgentType, "worker-template")
		}
		if n.Instructions != w.instructions {
			t.Errorf("node %q instructions = %q, want %q", n.NodeID, n.Instructions, w.instructions)
		}
	}
	for id := range wantByID {
		if !seen[id] {
			t.Errorf("expected materialized node %q not found", id)
		}
	}
}

// simulateNodeCompletions inserts one completed agent_session per
// materialized node (phase/node_id == the plan node id) and a distinct
// "result" session finding per node, then asserts the node-scoped read-back
// (DYNWF-3 GetByNode) resolves each node to its own value, disjoint from its
// siblings, exactly as it does for hand-inserted fan-out sessions in
// findings_node_test.go.
func simulateNodeCompletions(t *testing.T, env *TestEnv, ticketID, wfiID string, nodes []model.InstanceNode) {
	t.Helper()
	findingRepo := repo.NewFindingRepo(env.Pool, env.Clock)

	for _, n := range nodes {
		sessionID := "sess-" + n.NodeID
		env.InsertAgentSession(t, sessionID, ticketID, wfiID, n.NodeID, n.AgentType, "")
		value := json.RawMessage(`"` + n.NodeID + `-output"`)
		if err := findingRepo.Upsert("session", sessionID, "result", value,
			repo.Denorm{ProjectID: env.ProjectID, WorkflowInstanceID: wfiID, AgentType: n.AgentType},
			repo.Actor{Source: "agent"},
		); err != nil {
			t.Fatalf("upsert result finding for node %q: %v", n.NodeID, err)
		}
		env.CompleteAgentSession(t, sessionID, "pass")
		env.Clock.Advance(time.Second)
	}

	var map1Result map[string]interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"node_id":     "map-1",
		"instance_id": wfiID,
	}, &map1Result)
	if map1Result["result"] != "map-1-output" {
		t.Fatalf("node map-1 result = %v, want map-1-output", map1Result["result"])
	}

	var map2Result map[string]interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"node_id":     "map-2",
		"instance_id": wfiID,
	}, &map2Result)
	if map2Result["result"] != "map-2-output" {
		t.Fatalf("node map-2 result = %v, want map-2-output", map2Result["result"])
	}
	if map1Result["result"] == map2Result["result"] {
		t.Fatalf("map-1 and map-2 node-scoped results must be disjoint, both got %v", map1Result["result"])
	}
}

// assertReduceResultViaGetSubworkflow marks the instance completed, writes
// the well-known "workflow_final_result" finding on the reduce-1 node's
// session, links the instance to a fake parent via parent_instance_id, then
// asserts orchestrator.GetSubworkflow (the run_subworkflow caller's read path)
// resolves it -- the DYNWF-5 acceptance criterion that a reduce node's result
// is readable by the parent exactly like any other subworkflow result.
func assertReduceResultViaGetSubworkflow(t *testing.T, env *TestEnv, wfiID string) {
	t.Helper()

	if err := repo.NewWorkflowInstanceRepo(env.Pool, env.Clock).UpdateStatus(wfiID, model.WorkflowInstanceCompleted); err != nil {
		t.Fatalf("UpdateStatus(completed): %v", err)
	}

	const callerInstanceID = "caller-inst-1"
	if _, err := env.Pool.Exec(`UPDATE workflow_instances SET parent_instance_id = ? WHERE id = ?`, callerInstanceID, wfiID); err != nil {
		t.Fatalf("set parent_instance_id: %v", err)
	}

	finalValue := json.RawMessage(`"reduce-1-final"`)
	if err := repo.NewFindingRepo(env.Pool, env.Clock).Upsert("session", "sess-reduce-1", "workflow_final_result", finalValue,
		repo.Denorm{ProjectID: env.ProjectID, WorkflowInstanceID: wfiID, AgentType: "worker-template"},
		repo.Actor{Source: "agent"},
	); err != nil {
		t.Fatalf("upsert workflow_final_result finding: %v", err)
	}

	orch := orchestrator.New(env.Pool.Path, env.Hub, env.Clock, nil, "")
	status, result, _, err := orch.GetSubworkflow(context.Background(), callerInstanceID, env.ProjectID, wfiID, "")
	if err != nil {
		t.Fatalf("GetSubworkflow: %v", err)
	}
	if status != "completed" {
		t.Fatalf("GetSubworkflow status = %q, want completed", status)
	}
	var got string
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("unmarshal GetSubworkflow result: %v", err)
	}
	if got != "reduce-1-final" {
		t.Errorf("GetSubworkflow result = %q, want %q", got, "reduce-1-final")
	}
}
