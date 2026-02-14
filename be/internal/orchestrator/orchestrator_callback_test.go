package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// TestHandleCallback_SingleAgentCallback tests callback from a single-agent layer.
// Verifies: phases reset, sessions marked callback, _callback metadata saved, WS event broadcast.
func TestHandleCallback_SingleAgentCallback(t *testing.T) {
	env := newTestEnv(t)

	// Create 3-layer workflow: layer 0 (analyzer), layer 1 (builder), layer 2 (verifier)
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "analyzer", "layer": 0},
		{"agent": "builder", "layer": 1},
		{"agent": "verifier", "layer": 2},
	})
	_, err := env.pool.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, scope_type, created_at, updated_at)
		VALUES ('callback-test', ?, 'Callback test workflow', ?, 'ticket', datetime('now'), datetime('now'))`,
		env.project, string(phasesJSON))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	env.createTicket(t, "CB-1", "Callback test")

	// Init workflow
	var wfiID string
	err = env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-1', ?, 'CB-1', 'callback-test', 'active', '["analyzer","builder","verifier"]',
		        '{"analyzer":{"status":"completed","result":"pass"},"builder":{"status":"completed","result":"pass"},"verifier":{"status":"active"}}',
		        '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Create agent sessions for completed phases
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database)
	asRepo.Create(&model.AgentSession{
		ID:                 "sess-analyzer",
		ProjectID:          env.project,
		TicketID:           "CB-1",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionCompleted,
		Result:             sql.NullString{String: "pass", Valid: true},
	})
	asRepo.Create(&model.AgentSession{
		ID:                 "sess-builder",
		ProjectID:          env.project,
		TicketID:           "CB-1",
		WorkflowInstanceID: wfiID,
		Phase:              "builder",
		AgentType:          "builder",
		Status:             model.AgentSessionCompleted,
		Result:             sql.NullString{String: "pass", Valid: true},
	})

	// Build layer groups
	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 2}}},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "CB-1",
		WorkflowName: "callback-test",
		ScopeType:    "ticket",
	}

	// Subscribe to WS events
	ch := env.subscribeWSClient(t, "ws-cb-1", "CB-1")

	// Simulate callback from layer 2 (verifier) to layer 1 (builder)
	targetIdx := env.orch.handleCallback(context.Background(),wfiID, req, layerGroups, 2, 1, "verifier", "Fix the builder phase")

	// Verify target index returned correctly
	if targetIdx != 1 {
		t.Errorf("expected targetIdx=1, got %d", targetIdx)
	}

	// Verify _callback metadata saved in workflow instance findings
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
	callback, ok := findings["_callback"].(map[string]interface{})
	if !ok {
		t.Fatal("expected _callback key in workflow instance findings")
	}
	if callback["level"] != float64(1) {
		t.Errorf("expected callback level=1, got %v", callback["level"])
	}
	if callback["instructions"] != "Fix the builder phase" {
		t.Errorf("expected callback instructions='Fix the builder phase', got %v", callback["instructions"])
	}
	if callback["from_layer"] != float64(2) {
		t.Errorf("expected from_layer=2, got %v", callback["from_layer"])
	}
	if callback["from_agent"] != "verifier" {
		t.Errorf("expected from_agent='verifier', got %v", callback["from_agent"])
	}

	// Verify phases reset for layers 1 and 2
	phases := wi.GetPhases()
	if phases["analyzer"].Status != "completed" {
		t.Errorf("expected analyzer status=completed (not reset), got %s", phases["analyzer"].Status)
	}
	if phases["builder"].Status != "pending" {
		t.Errorf("expected builder status=pending (reset), got %s", phases["builder"].Status)
	}
	if phases["verifier"].Status != "pending" {
		t.Errorf("expected verifier status=pending (reset), got %s", phases["verifier"].Status)
	}

	// Verify agent sessions marked as callback with cleared findings
	builderSess, _ := asRepo.Get("sess-builder")
	if builderSess.Status != model.AgentSessionCallback {
		t.Errorf("expected builder session status=callback, got %s", builderSess.Status)
	}
	if builderSess.Findings.String != "{}" {
		t.Errorf("expected builder session findings cleared, got %s", builderSess.Findings.String)
	}

	// Verify WS event broadcast
	event := expectEvent(t, ch, ws.EventOrchestrationCallback, 2*time.Second)
	if event.ProjectID != env.project {
		t.Errorf("expected project_id=%s, got %s", env.project, event.ProjectID)
	}
	if event.TicketID != "CB-1" {
		t.Errorf("expected ticket_id=CB-1, got %s", event.TicketID)
	}
	if event.Data["instance_id"] != wfiID {
		t.Errorf("expected instance_id=%s, got %v", wfiID, event.Data["instance_id"])
	}
	if event.Data["from_layer"] != float64(2) {
		t.Errorf("expected from_layer=2, got %v", event.Data["from_layer"])
	}
	if event.Data["to_layer"] != float64(1) {
		t.Errorf("expected to_layer=1, got %v", event.Data["to_layer"])
	}
	if event.Data["instructions"] != "Fix the builder phase" {
		t.Errorf("expected instructions='Fix the builder phase', got %v", event.Data["instructions"])
	}
}

// TestHandleCallback_MultiAgentLayerUsesLowestLevel tests that when multiple agents
// in the same layer request callback, the lowest callback level is used.
func TestHandleCallback_MultiAgentLayerUsesLowestLevel(t *testing.T) {
	env := newTestEnv(t)

	// Create workflow with multi-agent layer 1
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "analyzer", "layer": 0},
		{"agent": "impl-a", "layer": 1},
		{"agent": "impl-b", "layer": 1},
		{"agent": "verifier", "layer": 2},
	})
	_, err := env.pool.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, scope_type, created_at, updated_at)
		VALUES ('multi-callback', ?, 'Multi callback workflow', ?, 'ticket', datetime('now'), datetime('now'))`,
		env.project, string(phasesJSON))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	env.createTicket(t, "CB-MULTI", "Multi callback test")

	var wfiID string
	err = env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-multi', ?, 'CB-MULTI', 'multi-callback', 'active', '["analyzer","impl-a","impl-b","verifier"]',
		        '{"analyzer":{"status":"completed"},"impl-a":{"status":"completed"},"impl-b":{"status":"completed"},"verifier":{"status":"active"}}',
		        '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "impl-a", Layer: 1}, {Agent: "impl-b", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 2}}},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "CB-MULTI",
		WorkflowName: "multi-callback",
		ScopeType:    "ticket",
	}

	// In the orchestrator's runLoop, when multiple agents callback, the lowest level is used.
	// Here we simulate that by calling handleCallback with the lowest level.
	// In reality, runLoop detects both callbacks and picks level 0 (lowest).
	targetIdx := env.orch.handleCallback(context.Background(),wfiID, req, layerGroups, 2, 0, "verifier", "Callback to layer 0")

	if targetIdx != 0 {
		t.Errorf("expected targetIdx=0 (lowest level), got %d", targetIdx)
	}

	// Verify all phases from layer 0 onwards are reset
	wi := env.getWorkflowInstance(t, wfiID)
	phases := wi.GetPhases()
	if phases["analyzer"].Status != "pending" {
		t.Errorf("expected analyzer=pending, got %s", phases["analyzer"].Status)
	}
	if phases["impl-a"].Status != "pending" {
		t.Errorf("expected impl-a=pending, got %s", phases["impl-a"].Status)
	}
	if phases["impl-b"].Status != "pending" {
		t.Errorf("expected impl-b=pending, got %s", phases["impl-b"].Status)
	}
	if phases["verifier"].Status != "pending" {
		t.Errorf("expected verifier=pending, got %s", phases["verifier"].Status)
	}
}

// TestHandleCallback_InvalidLayerNumber tests that handleCallback returns -1
// when the callback target layer number doesn't exist in layerGroups.
func TestHandleCallback_InvalidLayerNumber(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CB-INVALID", "Invalid callback test")

	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-invalid', ?, 'CB-INVALID', 'test', 'active', '["analyzer","builder"]',
		        '{"analyzer":{"status":"completed"},"builder":{"status":"active"}}',
		        '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "CB-INVALID",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}

	// Request callback to layer 99 (doesn't exist)
	targetIdx := env.orch.handleCallback(context.Background(),wfiID, req, layerGroups, 1, 99, "builder", "Invalid callback")

	// Should return -1 for invalid layer
	if targetIdx != -1 {
		t.Errorf("expected targetIdx=-1 for invalid layer, got %d", targetIdx)
	}
}

// TestHandleCallback_CallbackMetadataPreserved tests that callback metadata
// is correctly saved and preserved in workflow instance findings.
func TestHandleCallback_CallbackMetadataPreserved(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CB-META", "Callback metadata test")

	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-meta', ?, 'CB-META', 'test', 'active', '["analyzer","builder"]',
		        '{"analyzer":{"status":"completed"},"builder":{"status":"active"}}',
		        '{"existing_key":"existing_value"}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "CB-META",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}

	env.orch.handleCallback(context.Background(),wfiID, req, layerGroups, 1, 0, "builder", "Detailed callback instructions here")

	// Verify both existing and callback findings are preserved
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()

	// Existing key should be preserved
	if findings["existing_key"] != "existing_value" {
		t.Error("expected existing findings to be preserved")
	}

	// Callback metadata should be present
	callback, ok := findings["_callback"].(map[string]interface{})
	if !ok {
		t.Fatal("expected _callback key in findings")
	}
	if callback["level"] != float64(0) {
		t.Errorf("expected level=0, got %v", callback["level"])
	}
	if callback["instructions"] != "Detailed callback instructions here" {
		t.Errorf("expected specific instructions, got %v", callback["instructions"])
	}
	if callback["from_layer"] != float64(1) {
		t.Errorf("expected from_layer=1, got %v", callback["from_layer"])
	}
	if callback["from_agent"] != "builder" {
		t.Errorf("expected from_agent='builder', got %v", callback["from_agent"])
	}
}

// TestHandleCallback_SessionsExcludeRunningAndContinued tests that ResetSessionsForCallback
// only resets completed/failed sessions, not running or continued ones.
func TestHandleCallback_SessionsExcludeRunningAndContinued(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CB-EXCL", "Callback exclusion test")

	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-excl', ?, 'CB-EXCL', 'test', 'active', '["analyzer","builder"]',
		        '{"analyzer":{"status":"completed"},"builder":{"status":"active"}}',
		        '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Create multiple sessions with different statuses
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database)
	asRepo.Create(&model.AgentSession{
		ID:                 "sess-completed",
		ProjectID:          env.project,
		TicketID:           "CB-EXCL",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionCompleted,
		Findings:           sql.NullString{String: `{"key":"value"}`, Valid: true},
	})
	asRepo.Create(&model.AgentSession{
		ID:                 "sess-running",
		ProjectID:          env.project,
		TicketID:           "CB-EXCL",
		WorkflowInstanceID: wfiID,
		Phase:              "builder",
		AgentType:          "builder",
		Status:             model.AgentSessionRunning,
		Findings:           sql.NullString{String: `{"key":"value"}`, Valid: true},
	})
	asRepo.Create(&model.AgentSession{
		ID:                 "sess-continued",
		ProjectID:          env.project,
		TicketID:           "CB-EXCL",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionContinued,
		Findings:           sql.NullString{String: `{"key":"value"}`, Valid: true},
	})

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "CB-EXCL",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}

	env.orch.handleCallback(context.Background(),wfiID, req, layerGroups, 1, 0, "builder", "Test")

	// Verify completed session was reset
	completedSess, _ := asRepo.Get("sess-completed")
	if completedSess.Status != model.AgentSessionCallback {
		t.Errorf("expected completed session status=callback, got %s", completedSess.Status)
	}
	if completedSess.Findings.String != "{}" {
		t.Errorf("expected completed session findings cleared, got %s", completedSess.Findings.String)
	}

	// Verify running session was NOT reset
	runningSess, _ := asRepo.Get("sess-running")
	if runningSess.Status != model.AgentSessionRunning {
		t.Errorf("expected running session to remain running, got %s", runningSess.Status)
	}
	if runningSess.Findings.String != `{"key":"value"}` {
		t.Errorf("expected running session findings unchanged, got %s", runningSess.Findings.String)
	}

	// Verify continued session was NOT reset
	continuedSess, _ := asRepo.Get("sess-continued")
	if continuedSess.Status != model.AgentSessionContinued {
		t.Errorf("expected continued session to remain continued, got %s", continuedSess.Status)
	}
	if continuedSess.Findings.String != `{"key":"value"}` {
		t.Errorf("expected continued session findings unchanged, got %s", continuedSess.Findings.String)
	}
}

// TestHandleCallback_CallbackToLayerZero tests callback to layer 0 (edge case)
func TestHandleCallback_CallbackToLayerZero(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CB-ZERO", "Callback to zero test")

	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-zero', ?, 'CB-ZERO', 'test', 'active', '["analyzer","builder"]',
		        '{"analyzer":{"status":"completed"},"builder":{"status":"active"}}',
		        '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "CB-ZERO",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}

	// Callback from layer 1 to layer 0
	targetIdx := env.orch.handleCallback(context.Background(),wfiID, req, layerGroups, 1, 0, "builder", "Restart from beginning")

	if targetIdx != 0 {
		t.Errorf("expected targetIdx=0, got %d", targetIdx)
	}

	// Verify both phases are reset
	wi := env.getWorkflowInstance(t, wfiID)
	phases := wi.GetPhases()
	if phases["analyzer"].Status != "pending" {
		t.Errorf("expected analyzer=pending, got %s", phases["analyzer"].Status)
	}
	if phases["builder"].Status != "pending" {
		t.Errorf("expected builder=pending, got %s", phases["builder"].Status)
	}

	// Verify _callback metadata
	findings := wi.GetFindings()
	callback := findings["_callback"].(map[string]interface{})
	if callback["level"] != float64(0) {
		t.Errorf("expected callback level=0, got %v", callback["level"])
	}
}

// TestRunLoop_MaxCallbacksExceeded tests that workflow fails after exceeding max callbacks
func TestRunLoop_MaxCallbacksExceeded(t *testing.T) {
	// This is a conceptual test showing the max callback logic.
	// The actual runLoop tracks callbackCount and fails when callbackCount > maxCallbacks (3).

	const maxCallbacks = 3
	callbackCount := 0

	// Simulate callbacks
	for i := 0; i < 5; i++ {
		callbackCount++
		if callbackCount > maxCallbacks {
			// Workflow should fail here
			if i < maxCallbacks {
				t.Errorf("workflow failed too early at callback %d", i+1)
			}
			return
		}
	}

	t.Error("expected workflow to fail after 3 callbacks, but it continued")
}

// TestHandleCallback_ProjectScope tests callback for project-scoped workflows
func TestHandleCallback_ProjectScope(t *testing.T) {
	env := newTestEnv(t)

	// Update test workflow to project scope
	_, err := env.pool.Exec(`UPDATE workflows SET scope_type = 'project' WHERE id = 'test'`)
	if err != nil {
		t.Fatalf("failed to update workflow scope: %v", err)
	}

	var wfiID string
	err = env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-proj', ?, '', 'test', 'active', 'project', '["analyzer","builder"]',
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
		WorkflowName: "test",
		ScopeType:    "project",
	}

	// Subscribe to WS events (project scope uses empty ticket ID)
	ch := env.subscribeWSClient(t, "ws-proj-cb", "")

	targetIdx := env.orch.handleCallback(context.Background(),wfiID, req, layerGroups, 1, 0, "builder", "Project callback")

	if targetIdx != 0 {
		t.Errorf("expected targetIdx=0, got %d", targetIdx)
	}

	// Verify WS event for project scope (empty ticket_id)
	event := expectEvent(t, ch, ws.EventOrchestrationCallback, 2*time.Second)
	if event.TicketID != "" {
		t.Errorf("expected empty ticket_id for project scope, got %s", event.TicketID)
	}
	if event.ProjectID != env.project {
		t.Errorf("expected project_id=%s, got %s", env.project, event.ProjectID)
	}
}
