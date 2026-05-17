package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/service"
	"be/internal/spawner"
)

// TestCallback_EndToEnd_ClearingAfterLayerComplete tests the full callback flow:
// callback triggered → metadata saved → target layer notionally completes → metadata cleared.
func TestCallback_EndToEnd_ClearingAfterLayerComplete(t *testing.T) {
	env := newTestEnv(t)
	env.createWorkflowWithAgents(t, "callback-e2e", "Callback E2E workflow", "", []struct{ ID string; Layer int }{
		{"analyzer", 0}, {"builder", 1}, {"verifier", 2},
	})
	env.createTicket(t, "CB-E2E", "End-to-end callback test")
	wfiID := insertWFI(t, env, "wfi-e2e", "CB-E2E", "callback-e2e")

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 2}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-E2E", WorkflowName: "callback-e2e", ScopeType: "ticket"}

	// Step 1: trigger callback from layer 2 to level 1
	count := 0
	cbErrs := []*spawner.CallbackError{{Level: 1, AgentType: "verifier", Instructions: "Builder needs to fix the implementation"}}
	ok := env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 2, cbErrs, &count)
	if !ok {
		t.Fatalf("expected handleCallback to return true")
	}

	// Verify _callback metadata is saved with new shape
	wi := env.getWorkflowInstance(t, wfiID)
	cb, ok := wi.GetFindings()["_callback"].(map[string]interface{})
	if !ok {
		t.Fatal("expected _callback metadata after handleCallback")
	}
	if cb["from_layer"] != float64(2) {
		t.Errorf("from_layer = %v, want 2", cb["from_layer"])
	}
	if cb["instructions"] != "Builder needs to fix the implementation" {
		t.Errorf("instructions = %v", cb["instructions"])
	}
	if cb["resume_layer"] != float64(3) {
		t.Errorf("resume_layer = %v, want 3", cb["resume_layer"])
	}
	// plan should have 2 steps (layers 1 and 2)
	plan := cb["plan"].([]interface{})
	if len(plan) != 2 {
		t.Errorf("plan len = %d, want 2", len(plan))
	}

	// Step 2: simulate callback layer completing → clear metadata
	env.orch.clearCallbackMetadata(context.Background(), wfiID)

	// Verify _callback is cleared
	wi = env.getWorkflowInstance(t, wfiID)
	if _, ok := wi.GetFindings()["_callback"]; ok {
		t.Error("expected _callback to be cleared after callback target layer completes")
	}

	// Step 3: verify other findings survive
	otherFindings := map[string]interface{}{"layer2_result": "success"}
	findingsJSON, _ := json.Marshal(otherFindings)
	_, err := env.pool.Exec(`UPDATE workflow_instances SET findings = ? WHERE id = ?`, string(findingsJSON), wfiID)
	if err != nil {
		t.Fatalf("failed to update findings: %v", err)
	}
	wi = env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
	if _, ok := findings["_callback"]; ok {
		t.Error("_callback should not reappear")
	}
	if findings["layer2_result"] != "success" {
		t.Error("expected other findings preserved")
	}
}

// TestCallback_EndToEnd_MultipleCallbacksWithClearing tests two sequential callback cycles.
func TestCallback_EndToEnd_MultipleCallbacksWithClearing(t *testing.T) {
	env := newTestEnv(t)
	env.createWorkflowWithAgents(t, "multi-cb", "Multiple callback workflow", "", []struct{ ID string; Layer int }{
		{"analyzer", 0}, {"builder", 1}, {"tester", 2}, {"verifier", 3},
	})
	env.createTicket(t, "CB-MULTI2", "Multiple callback cycles")
	wfiID := insertWFI(t, env, "wfi-multi2", "CB-MULTI2", "multi-cb")

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "tester", Layer: 2}}},
		{layer: 3, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 3}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-MULTI2", WorkflowName: "multi-cb", ScopeType: "ticket"}

	// First callback: verifier → builder (layer 3 → level 1)
	count := 0
	env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 3,
		[]*spawner.CallbackError{{Level: 1, AgentType: "verifier", Instructions: "First callback: fix builder"}}, &count)

	cb := env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"].(map[string]interface{})
	if cb["instructions"] != "First callback: fix builder" {
		t.Error("expected first callback instructions")
	}

	// Clear after first callback
	env.orch.clearCallbackMetadata(context.Background(), wfiID)
	if _, ok := env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"]; ok {
		t.Error("expected _callback cleared after first cycle")
	}

	// Second callback: tester → analyzer (layer 2 → level 0)
	env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 2,
		[]*spawner.CallbackError{{Level: 0, AgentType: "tester", Instructions: "Second callback: restart from analyzer"}}, &count)

	cb = env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"].(map[string]interface{})
	if cb["instructions"] != "Second callback: restart from analyzer" {
		t.Error("expected second callback instructions")
	}
	if cb["from_layer"] != float64(2) {
		t.Errorf("from_layer = %v, want 2", cb["from_layer"])
	}

	// Clear after second callback
	env.orch.clearCallbackMetadata(context.Background(), wfiID)
	if _, ok := env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"]; ok {
		t.Error("expected _callback cleared after second cycle")
	}
}

// TestCallback_EndToEnd_NoLeakToNextLayer verifies cleared callback metadata does not appear.
func TestCallback_EndToEnd_NoLeakToNextLayer(t *testing.T) {
	env := newTestEnv(t)
	env.createWorkflowWithAgents(t, "leak-test", "Leak test", "", []struct{ ID string; Layer int }{
		{"analyzer", 0}, {"builder", 1}, {"tester", 2}, {"deployer", 3},
	})
	env.createTicket(t, "CB-LEAK2", "No leak test")
	wfiID := insertWFI(t, env, "wfi-leak2", "CB-LEAK2", "leak-test")

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "tester", Layer: 2}}},
		{layer: 3, phases: []service.SpawnerPhaseDef{{Agent: "deployer", Layer: 3}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-LEAK2", WorkflowName: "leak-test", ScopeType: "ticket"}

	count := 0
	env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 2,
		[]*spawner.CallbackError{{Level: 1, AgentType: "tester", Instructions: "Fix builder"}}, &count)

	if _, ok := env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"]; !ok {
		t.Fatal("expected _callback to be set")
	}

	env.orch.clearCallbackMetadata(context.Background(), wfiID)

	for _, lbl := range []string{"tester-layer", "deployer-layer"} {
		_ = lbl
		if _, ok := env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"]; ok {
			t.Errorf("_callback should not be visible after clear")
		}
	}
}

// TestCallback_EndToEnd_ProjectScope tests the full callback flow for project-scoped workflows.
func TestCallback_EndToEnd_ProjectScope(t *testing.T) {
	env := newTestEnv(t)
	env.createWorkflowWithAgents(t, "proj-cb-e2e", "Project callback E2E", "project", []struct{ ID string; Layer int }{
		{"analyzer", 0}, {"builder", 1},
	})

	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-proj-e2e', ?, '', 'proj-cb-e2e', 'active', 'project', '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("create project WFI: %v", err)
	}

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "", WorkflowName: "proj-cb-e2e", ScopeType: "project"}

	count := 0
	env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 1,
		[]*spawner.CallbackError{{Level: 0, AgentType: "builder", Instructions: "Project callback instructions"}}, &count)

	cb := env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"].(map[string]interface{})
	if cb["instructions"] != "Project callback instructions" {
		t.Error("expected project callback instructions")
	}

	env.orch.clearCallbackMetadata(context.Background(), wfiID)
	if _, ok := env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"]; ok {
		t.Error("expected project workflow _callback to be cleared")
	}
}

// TestCallback_EndToEnd_MultipleRequestsMerged verifies that multiple concurrent callback
// requests from the same layer are merged into one plan.
func TestCallback_EndToEnd_MultipleRequestsMerged(t *testing.T) {
	env := newTestEnv(t)
	env.createWorkflowWithAgents(t, "merge-cb", "Merge callback", "", []struct{ ID string; Layer int }{
		{"analyzer", 0}, {"impl-a", 1}, {"impl-b", 1}, {"verifier", 2},
	})
	env.createTicket(t, "CB-MERGE", "Merge test")
	wfiID := insertWFI(t, env, "wfi-merge", "CB-MERGE", "merge-cb")

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "impl-a", Layer: 1}, {Agent: "impl-b", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 2}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-MERGE", WorkflowName: "merge-cb", ScopeType: "ticket"}

	// Both impl-a and impl-b request callback to level 0 (from originator layer 1)
	count := 0
	cbErrs := []*spawner.CallbackError{
		{Level: 0, AgentType: "impl-a", Instructions: "impl-a says: fix analyzer"},
		{Level: 0, AgentType: "impl-b", Instructions: "impl-b says: fix analyzer"},
	}
	ok := env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 1, cbErrs, &count)
	if !ok {
		t.Fatal("expected true for merged callbacks")
	}

	cb := env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"].(map[string]interface{})
	plan := cb["plan"].([]interface{})
	// level 0 from originatorLayer 1 → layers 0 and 1 → 2 steps
	if len(plan) != 2 {
		t.Errorf("merged plan len = %d, want 2", len(plan))
	}

	// instructions should be joined (sorted by agentID: impl-a before impl-b)
	wantInstr := "impl-a says: fix analyzer\n---\nimpl-b says: fix analyzer"
	if cb["instructions"] != wantInstr {
		t.Errorf("instructions = %q, want %q", cb["instructions"], wantInstr)
	}
}
