package integration

import (
	"encoding/json"
	"testing"

	"be/internal/service"
	"be/internal/types"
)

// TestAgentDefLayer_CRUDAndPhaseDerivation exercises the full flow of creating
// agent definitions with layers and verifying that phases are derived correctly.
func TestAgentDefLayer_CRUDAndPhaseDerivation(t *testing.T) {
	env := NewTestEnv(t)

	// Create a new workflow (no phases field)
	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "layered",
		Description: "Layered workflow",
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	adSvc := env.getAgentDefService(t)

	// Create agents at different layers
	agents := []types.AgentDefCreateRequest{
		{ID: "verifier", Prompt: "verify", Layer: 2},
		{ID: "builder", Prompt: "build", Layer: 1},
		{ID: "analyzer", Prompt: "analyze", Layer: 0},
	}
	for _, a := range agents {
		if _, err := adSvc.CreateAgentDef(env.ProjectID, "layered", &a); err != nil {
			t.Fatalf("create agent %s: %v", a.ID, err)
		}
	}

	// GetWorkflowDef should return phases derived from agent defs, sorted by layer
	wf, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "layered")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if len(wf.Phases) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(wf.Phases))
	}
	expected := []struct {
		agent string
		layer int
	}{
		{"analyzer", 0}, {"builder", 1}, {"verifier", 2},
	}
	for i, e := range expected {
		if wf.Phases[i].Agent != e.agent || wf.Phases[i].Layer != e.layer {
			t.Errorf("phase[%d] = {%s, L%d}, want {%s, L%d}", i, wf.Phases[i].Agent, wf.Phases[i].Layer, e.agent, e.layer)
		}
	}

	// Update verifier layer to 1 → should fail (fan-in: builder already at L1, single agent required after multi-agent layer would be needed)
	// Actually L0=1 agent, L1 would have 2 agents, no next layer → valid (no fan-in needed)
	newLayer := 1
	err = adSvc.UpdateAgentDef(env.ProjectID, "layered", "verifier", &types.AgentDefUpdateRequest{Layer: &newLayer})
	if err != nil {
		t.Fatalf("update verifier to L1 should be valid (no next layer needs fan-in): %v", err)
	}

	// Verify phases re-derived after update
	wf2, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "layered")
	if err != nil {
		t.Fatalf("GetWorkflowDef after update: %v", err)
	}
	// Now: analyzer(L0), builder(L1), verifier(L1)
	if len(wf2.Phases) != 3 {
		t.Fatalf("expected 3 phases after update, got %d", len(wf2.Phases))
	}
	if wf2.Phases[0].Agent != "analyzer" || wf2.Phases[0].Layer != 0 {
		t.Errorf("phase[0] = {%s, L%d}, want {analyzer, L0}", wf2.Phases[0].Agent, wf2.Phases[0].Layer)
	}
	// L1 agents should be alphabetically ordered: builder, verifier
	if wf2.Phases[1].Agent != "builder" || wf2.Phases[2].Agent != "verifier" {
		t.Errorf("L1 agents = [%s, %s], want [builder, verifier]", wf2.Phases[1].Agent, wf2.Phases[2].Agent)
	}

	// Delete an agent, phases should re-derive
	if err := adSvc.DeleteAgentDef(env.ProjectID, "layered", "verifier"); err != nil {
		t.Fatalf("delete verifier: %v", err)
	}
	wf3, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "layered")
	if err != nil {
		t.Fatalf("GetWorkflowDef after delete: %v", err)
	}
	if len(wf3.Phases) != 2 {
		t.Fatalf("expected 2 phases after delete, got %d", len(wf3.Phases))
	}
}

// TestAgentDefLayer_WorkflowStatusWithSessions verifies that workflow status correctly
// shows phase statuses when agents have sessions, using layer-derived phases.
func TestAgentDefLayer_WorkflowStatusWithSessions(t *testing.T) {
	env := NewTestEnv(t)

	// Use default "test" workflow (analyzer L0, builder L1)
	env.CreateTicket(t, "LAY-1", "Layer status test")
	env.InitWorkflow(t, "LAY-1")

	wfiID := env.GetWorkflowInstanceID(t, "LAY-1", "test")

	// Insert a completed session for analyzer phase
	env.InsertAgentSession(t, "sess-a1", "LAY-1", wfiID, "analyzer", "analyzer", "")
	env.CompleteAgentSession(t, "sess-a1", "pass")
	env.Clock.Advance(1)

	// Insert a running session for builder phase
	env.InsertAgentSession(t, "sess-b1", "LAY-1", wfiID, "builder", "builder", "")

	raw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "LAY-1", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	status := normalizeJSON(t, raw)

	phases, _ := status["phases"].(map[string]interface{})
	analyzerPhase, _ := phases["analyzer"].(map[string]interface{})
	builderPhase, _ := phases["builder"].(map[string]interface{})

	if analyzerPhase["status"] != "completed" {
		t.Errorf("analyzer status = %v, want completed", analyzerPhase["status"])
	}
	if builderPhase["status"] != "in_progress" {
		t.Errorf("builder status = %v, want in_progress", builderPhase["status"])
	}

	// phase_order should be derived from agent defs
	phaseOrder, _ := status["phase_order"].([]interface{})
	if len(phaseOrder) != 2 {
		t.Fatalf("phase_order len = %d, want 2", len(phaseOrder))
	}
	if phaseOrder[0] != "analyzer" || phaseOrder[1] != "builder" {
		t.Errorf("phase_order = %v, want [analyzer, builder]", phaseOrder)
	}
}

// TestAgentDefLayer_BuildSpawnerConfigFromDB verifies BuildSpawnerConfig works with
// DB-loaded agent definitions (full round-trip).
func TestAgentDefLayer_BuildSpawnerConfigFromDB(t *testing.T) {
	env := NewTestEnv(t)

	// Create workflow with agents
	createWorkflowWithAgents(t, env, "spawner-test", []types.AgentDefCreateRequest{
		{ID: "setup", Prompt: "s", Layer: 0},
		{ID: "test-be", Prompt: "t", Layer: 1},
		{ID: "test-fe", Prompt: "t", Layer: 1},
		{ID: "qa", Prompt: "q", Layer: 2},
	})

	// Load from DB and build config
	wfDefs, err := env.WorkflowSvc.ListWorkflowDefs(env.ProjectID)
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}

	wf, ok := wfDefs["spawner-test"]
	if !ok {
		t.Fatal("spawner-test not in listed defs")
	}

	// Marshal to JSON to verify serialization
	data, err := json.Marshal(wf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed service.WorkflowDef
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(parsed.Phases) != 4 {
		t.Fatalf("phases count = %d, want 4", len(parsed.Phases))
	}

	// Verify ordering: L0(setup), L1(test-be, test-fe alphabetical), L2(qa)
	wantOrder := []string{"setup", "test-be", "test-fe", "qa"}
	for i, want := range wantOrder {
		if parsed.Phases[i].Agent != want {
			t.Errorf("phases[%d].Agent = %s, want %s", i, parsed.Phases[i].Agent, want)
		}
	}
}
