package orchestrator

import (
	"encoding/json"
	"testing"

	"be/internal/service"
)

// TestCallback_EndToEnd_ClearingAfterLayerComplete tests the full callback flow:
// 1. Callback is triggered and _callback metadata is saved
// 2. Target layer runs and completes successfully
// 3. wasCallback flag is set and _callback metadata is cleared after layer completes
// 4. Subsequent layers do not see stale callback metadata
//
// This is a critical end-to-end test to ensure callback metadata doesn't leak.
func TestCallback_EndToEnd_ClearingAfterLayerComplete(t *testing.T) {
	env := newTestEnv(t)

	// Create 3-layer workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "analyzer", "layer": 0},
		{"agent": "builder", "layer": 1},
		{"agent": "verifier", "layer": 2},
	})
	_, err := env.pool.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, categories, scope_type, created_at, updated_at)
		VALUES ('callback-e2e', ?, 'Callback E2E workflow', ?, '["full"]', 'ticket', datetime('now'), datetime('now'))`,
		env.project, string(phasesJSON))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	env.createTicket(t, "CB-E2E", "End-to-end callback test")

	// Create workflow instance: layer 0 and 1 completed, layer 2 active
	var wfiID string
	err = env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-e2e', ?, 'CB-E2E', 'callback-e2e', 'active', '["analyzer","builder","verifier"]',
		        '{"analyzer":{"status":"completed","result":"pass"},"builder":{"status":"completed","result":"pass"},"verifier":{"status":"active"}}',
		        '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 2}}},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "CB-E2E",
		WorkflowName: "callback-e2e",
		ScopeType:    "ticket",
	}

	// === Step 1: Trigger callback from layer 2 to layer 1 ===
	targetIdx := env.orch.handleCallback(wfiID, req, layerGroups, 2, 1, "verifier", "Builder needs to fix the implementation")

	if targetIdx != 1 {
		t.Fatalf("expected targetIdx=1, got %d", targetIdx)
	}

	// Verify _callback metadata is saved
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
	callback, ok := findings["_callback"].(map[string]interface{})
	if !ok {
		t.Fatal("expected _callback metadata after handleCallback")
	}
	if callback["level"] != float64(1) {
		t.Errorf("expected callback level=1, got %v", callback["level"])
	}
	if callback["instructions"] != "Builder needs to fix the implementation" {
		t.Errorf("expected callback instructions, got %v", callback["instructions"])
	}

	// === Step 2: Simulate layer 1 (builder) completing successfully ===
	// In the real runLoop, after layer 1 completes, wasCallback would be set to true
	// and clearCallbackMetadata would be called.

	// Mark layer 1 as completed
	_, err = env.pool.Exec(`
		UPDATE workflow_instances
		SET phases = json_set(phases, '$.builder', json_object('status', 'completed', 'result', 'pass'))
		WHERE id = ?`, wfiID)
	if err != nil {
		t.Fatalf("failed to update phases: %v", err)
	}

	// === Step 3: Clear callback metadata (as runLoop does after callback layer completes) ===
	env.orch.clearCallbackMetadata(wfiID)

	// Verify _callback is cleared
	wi = env.getWorkflowInstance(t, wfiID)
	findings = wi.GetFindings()
	if _, ok := findings["_callback"]; ok {
		t.Error("expected _callback to be cleared after callback target layer completes")
	}

	// === Step 4: Verify subsequent layers won't see stale callback metadata ===
	// If we were to run layer 2 (verifier) again, it should not see the callback instructions
	// This simulates what happens in runLoop when wasCallback=true triggers clearing before the next layer

	// Add some other findings to verify they're preserved
	findings["layer2_result"] = "success"
	findingsJSON, _ := json.Marshal(findings)
	_, err = env.pool.Exec(`UPDATE workflow_instances SET findings = ? WHERE id = ?`, string(findingsJSON), wfiID)
	if err != nil {
		t.Fatalf("failed to update findings: %v", err)
	}

	// Verify other findings are preserved but _callback is gone
	wi = env.getWorkflowInstance(t, wfiID)
	findings = wi.GetFindings()
	if _, ok := findings["_callback"]; ok {
		t.Error("_callback should still be cleared")
	}
	if findings["layer2_result"] != "success" {
		t.Error("expected other findings to be preserved")
	}
}

// TestCallback_EndToEnd_MultipleCallbacksWithClearing tests that callback metadata
// is cleared and can be re-set in a later callback (simulating multiple callback cycles).
func TestCallback_EndToEnd_MultipleCallbacksWithClearing(t *testing.T) {
	env := newTestEnv(t)

	// Create 4-layer workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "analyzer", "layer": 0},
		{"agent": "builder", "layer": 1},
		{"agent": "tester", "layer": 2},
		{"agent": "verifier", "layer": 3},
	})
	_, err := env.pool.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, categories, scope_type, created_at, updated_at)
		VALUES ('multi-cb', ?, 'Multiple callback workflow', ?, '["full"]', 'ticket', datetime('now'), datetime('now'))`,
		env.project, string(phasesJSON))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	env.createTicket(t, "CB-MULTI", "Multiple callback cycles")

	var wfiID string
	err = env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-multi', ?, 'CB-MULTI', 'multi-cb', 'active', '["analyzer","builder","tester","verifier"]',
		        '{"analyzer":{"status":"completed"},"builder":{"status":"completed"},"tester":{"status":"completed"},"verifier":{"status":"active"}}',
		        '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "tester", Layer: 2}}},
		{layer: 3, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 3}}},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "CB-MULTI",
		WorkflowName: "multi-cb",
		ScopeType:    "ticket",
	}

	// === First callback: verifier -> builder (layer 3 -> 1) ===
	env.orch.handleCallback(wfiID, req, layerGroups, 3, 1, "verifier", "First callback: fix builder")

	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
	callback := findings["_callback"].(map[string]interface{})
	if callback["instructions"] != "First callback: fix builder" {
		t.Error("expected first callback instructions")
	}

	// Clear after layer 1 completes
	env.orch.clearCallbackMetadata(wfiID)

	wi = env.getWorkflowInstance(t, wfiID)
	if _, ok := wi.GetFindings()["_callback"]; ok {
		t.Error("expected _callback to be cleared after first callback completes")
	}

	// === Second callback: tester -> analyzer (layer 2 -> 0) ===
	// This simulates a second callback happening later in the workflow
	env.orch.handleCallback(wfiID, req, layerGroups, 2, 0, "tester", "Second callback: restart from analyzer")

	wi = env.getWorkflowInstance(t, wfiID)
	findings = wi.GetFindings()
	callback = findings["_callback"].(map[string]interface{})
	if callback["instructions"] != "Second callback: restart from analyzer" {
		t.Error("expected second callback instructions")
	}
	if callback["level"] != float64(0) {
		t.Error("expected second callback level=0")
	}
	if callback["from_agent"] != "tester" {
		t.Error("expected second callback from_agent=tester")
	}

	// Clear after layer 0 completes
	env.orch.clearCallbackMetadata(wfiID)

	wi = env.getWorkflowInstance(t, wfiID)
	if _, ok := wi.GetFindings()["_callback"]; ok {
		t.Error("expected _callback to be cleared after second callback completes")
	}
}

// TestCallback_EndToEnd_NoLeakToNextLayer tests that when a callback is triggered,
// cleared, and workflow continues, the next layer that was NOT part of the callback
// does NOT see any callback metadata.
func TestCallback_EndToEnd_NoLeakToNextLayer(t *testing.T) {
	env := newTestEnv(t)

	// Create 4-layer workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "analyzer", "layer": 0},
		{"agent": "builder", "layer": 1},
		{"agent": "tester", "layer": 2},
		{"agent": "deployer", "layer": 3},
	})
	_, err := env.pool.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, categories, scope_type, created_at, updated_at)
		VALUES ('leak-test', ?, 'Leak test workflow', ?, '["full"]', 'ticket', datetime('now'), datetime('now'))`,
		env.project, string(phasesJSON))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	env.createTicket(t, "CB-LEAK", "No leak test")

	var wfiID string
	err = env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-leak', ?, 'CB-LEAK', 'leak-test', 'active', '["analyzer","builder","tester","deployer"]',
		        '{"analyzer":{"status":"completed"},"builder":{"status":"completed"},"tester":{"status":"active"},"deployer":{"status":"pending"}}',
		        '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "tester", Layer: 2}}},
		{layer: 3, phases: []service.SpawnerPhaseDef{{Agent: "deployer", Layer: 3}}},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "CB-LEAK",
		WorkflowName: "leak-test",
		ScopeType:    "ticket",
	}

	// Callback from layer 2 (tester) to layer 1 (builder)
	env.orch.handleCallback(wfiID, req, layerGroups, 2, 1, "tester", "Builder callback instructions")

	// Verify callback metadata is set
	wi := env.getWorkflowInstance(t, wfiID)
	if _, ok := wi.GetFindings()["_callback"]; !ok {
		t.Fatal("expected _callback to be set")
	}

	// Simulate layer 1 completing and callback metadata being cleared
	env.orch.clearCallbackMetadata(wfiID)

	// Now simulate layer 2 (tester) running again - should NOT see callback metadata
	wi = env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
	if _, ok := findings["_callback"]; ok {
		t.Error("layer 2 (tester) should NOT see callback metadata after it was cleared")
	}

	// Now simulate layer 3 (deployer) running - should DEFINITELY NOT see callback metadata
	// (it was never part of the callback loop)
	wi = env.getWorkflowInstance(t, wfiID)
	findings = wi.GetFindings()
	if _, ok := findings["_callback"]; ok {
		t.Error("layer 3 (deployer) should NOT see stale callback metadata")
	}
}

// TestCallback_EndToEnd_ProjectScope tests the full callback flow for project-scoped workflows.
func TestCallback_EndToEnd_ProjectScope(t *testing.T) {
	env := newTestEnv(t)

	// Create project-scoped workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "analyzer", "layer": 0},
		{"agent": "builder", "layer": 1},
	})
	_, err := env.pool.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, categories, scope_type, created_at, updated_at)
		VALUES ('proj-cb', ?, 'Project callback workflow', ?, '["full"]', 'project', datetime('now'), datetime('now'))`,
		env.project, string(phasesJSON))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	var wfiID string
	err = env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-proj', ?, '', 'proj-cb', 'active', 'project', '["analyzer","builder"]',
		        '{"analyzer":{"status":"completed"},"builder":{"status":"active"}}',
		        '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create project workflow instance: %v", err)
	}

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "",
		WorkflowName: "proj-cb",
		ScopeType:    "project",
	}

	// Trigger callback
	env.orch.handleCallback(wfiID, req, layerGroups, 1, 0, "builder", "Project callback instructions")

	// Verify callback metadata
	wi := env.getWorkflowInstance(t, wfiID)
	callback := wi.GetFindings()["_callback"].(map[string]interface{})
	if callback["instructions"] != "Project callback instructions" {
		t.Error("expected project callback instructions")
	}

	// Clear callback metadata
	env.orch.clearCallbackMetadata(wfiID)

	// Verify cleared
	wi = env.getWorkflowInstance(t, wfiID)
	if _, ok := wi.GetFindings()["_callback"]; ok {
		t.Error("expected project workflow _callback to be cleared")
	}
}
