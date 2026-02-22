package integration

import (
	"encoding/json"
	"testing"

	"be/internal/types"
)

// TestMigration028DropsPhaseColumns verifies that migration 000028 successfully
// drops the phases, phase_order, and current_phase columns from workflow_instances.
func TestMigration028DropsPhaseColumns(t *testing.T) {
	env := NewTestEnv(t)

	droppedColumns := []string{"phases", "phase_order", "current_phase"}

	for _, col := range droppedColumns {
		var count int
		err := env.Pool.QueryRow(`
			SELECT COUNT(*) FROM pragma_table_info('workflow_instances')
			WHERE name = ?`, col).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query schema for column %s: %v", col, err)
		}
		if count != 0 {
			t.Errorf("column %s should not exist in workflow_instances after migration 000028, found %d", col, count)
		}
	}
}

// TestMigration028WorkflowInstancesColumnCount verifies the total column count is exactly 11
// (no phase columns present).
func TestMigration028WorkflowInstancesColumnCount(t *testing.T) {
	env := NewTestEnv(t)

	var count int
	err := env.Pool.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('workflow_instances')`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count columns: %v", err)
	}

	const want = 12 // 11 original + skip_tags (migration 000030)
	if count != want {
		t.Errorf("workflow_instances expected %d columns, got %d", want, count)
	}
}

// TestMigration028WorkflowInstancesExpectedColumns verifies all 11 expected columns exist.
func TestMigration028WorkflowInstancesExpectedColumns(t *testing.T) {
	env := NewTestEnv(t)

	expectedColumns := []string{
		"id", "project_id", "ticket_id", "workflow_id", "scope_type",
		"status", "findings", "retry_count", "parent_session", "created_at", "updated_at",
	}

	for _, col := range expectedColumns {
		var count int
		err := env.Pool.QueryRow(`
			SELECT COUNT(*) FROM pragma_table_info('workflow_instances')
			WHERE name = ?`, col).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query schema for column %s: %v", col, err)
		}
		if count != 1 {
			t.Errorf("expected column %s to exist in workflow_instances, found %d", col, count)
		}
	}
}

// TestMigration028WorkflowInstanceCRUD verifies Create and Read operations work correctly
// on the 11-column schema (no phase columns).
func TestMigration028WorkflowInstanceCRUD(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "MI028-1", "CRUD test")
	env.InitWorkflow(t, "MI028-1")
	wfiID := env.GetWorkflowInstanceID(t, "MI028-1", "test")

	// Read back via raw query — must not reference phase columns
	var id, status, scopeType, workflowID string
	err := env.Pool.QueryRow(`
		SELECT id, status, scope_type, workflow_id
		FROM workflow_instances WHERE id = ?`, wfiID).Scan(&id, &status, &scopeType, &workflowID)
	if err != nil {
		t.Fatalf("failed to read workflow instance: %v", err)
	}
	if id != wfiID {
		t.Errorf("expected id=%s, got %s", wfiID, id)
	}
	if status != "active" {
		t.Errorf("expected status=active, got %s", status)
	}
	if scopeType != "ticket" {
		t.Errorf("expected scope_type=ticket, got %s", scopeType)
	}
	if workflowID != "test" {
		t.Errorf("expected workflow_id=test, got %s", workflowID)
	}
}

// TestMigration028WorkflowInstanceMarshalJSON verifies that WorkflowInstance JSON
// serialization does not include phases, phase_order, or current_phase fields.
func TestMigration028WorkflowInstanceMarshalJSON(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "MI028-JSON-1", "JSON marshal test")
	env.InitWorkflow(t, "MI028-JSON-1")

	wfi, err := env.WorkflowSvc.GetWorkflowInstance(env.ProjectID, "MI028-JSON-1", "test")
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}

	data, err := json.Marshal(wfi)
	if err != nil {
		t.Fatalf("failed to marshal workflow instance: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify dropped fields are not present
	removedFields := []string{"phases", "phase_order", "current_phase"}
	for _, field := range removedFields {
		if _, exists := result[field]; exists {
			t.Errorf("field %s should not exist in WorkflowInstance JSON, found in: %v", field, result)
		}
	}

	// Verify expected fields still present
	expectedFields := []string{"id", "project_id", "workflow_id", "scope_type", "status", "findings", "retry_count"}
	for _, field := range expectedFields {
		if _, exists := result[field]; !exists {
			t.Errorf("expected field %s to exist in WorkflowInstance JSON", field)
		}
	}
}

// TestMigration028PhaseStatusDerivedFromAgentSessions verifies that phase status comes
// from agent_sessions rows, not from dropped phase columns.
func TestMigration028PhaseStatusDerivedFromAgentSessions(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "MI028-PHASE-1", "Phase derivation test")
	env.InitWorkflow(t, "MI028-PHASE-1")
	wfiID := env.GetWorkflowInstanceID(t, "MI028-PHASE-1", "test")

	// Before any sessions: both phases should be pending
	statusRaw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "MI028-PHASE-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	statusData, _ := json.Marshal(statusRaw)
	var status map[string]interface{}
	if err := json.Unmarshal(statusData, &status); err != nil {
		t.Fatalf("failed to unmarshal status: %v", err)
	}

	phases, _ := status["phases"].(map[string]interface{})
	for _, phase := range []string{"analyzer", "builder"} {
		p, _ := phases[phase].(map[string]interface{})
		if p["status"] != "pending" {
			t.Errorf("expected phase %s=pending before any sessions, got %v", phase, p["status"])
		}
	}

	// Insert analyzer session (running) — analyzer should become running
	env.InsertAgentSession(t, "sess-phase-1", "MI028-PHASE-1", wfiID, "analyzer", "analyzer", "sonnet")

	statusRaw2, err := env.WorkflowSvc.GetStatus(env.ProjectID, "MI028-PHASE-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get workflow status after session insert: %v", err)
	}

	statusData2, _ := json.Marshal(statusRaw2)
	var status2 map[string]interface{}
	json.Unmarshal(statusData2, &status2)

	phases2, _ := status2["phases"].(map[string]interface{})
	analyzerPhase, _ := phases2["analyzer"].(map[string]interface{})
	if analyzerPhase["status"] != "in_progress" {
		t.Errorf("expected analyzer=in_progress after session insert, got %v", analyzerPhase["status"])
	}

	// Complete analyzer session — analyzer should become completed
	env.CompleteAgentSession(t, "sess-phase-1", "pass")

	statusRaw3, err := env.WorkflowSvc.GetStatus(env.ProjectID, "MI028-PHASE-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get workflow status after completion: %v", err)
	}

	statusData3, _ := json.Marshal(statusRaw3)
	var status3 map[string]interface{}
	json.Unmarshal(statusData3, &status3)

	phases3, _ := status3["phases"].(map[string]interface{})
	analyzerCompleted, _ := phases3["analyzer"].(map[string]interface{})
	if analyzerCompleted["status"] != "completed" {
		t.Errorf("expected analyzer=completed after session completion, got %v", analyzerCompleted["status"])
	}
}

// TestMigration028ForeignKeyIntegrity verifies FK constraints are intact after migration 000028.
func TestMigration028ForeignKeyIntegrity(t *testing.T) {
	env := NewTestEnv(t)

	var violations int
	err := env.Pool.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check()`).Scan(&violations)
	if err != nil {
		t.Fatalf("failed to run foreign_key_check: %v", err)
	}
	if violations != 0 {
		t.Errorf("expected 0 FK violations after migration 000028, found %d", violations)
	}
}

// TestMigration028EndToEnd is a full end-to-end workflow test verifying all operations
// work correctly after phase columns are dropped.
func TestMigration028EndToEnd(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "MI028-E2E-1", "End to end test")
	env.InitWorkflow(t, "MI028-E2E-1")
	wfiID := env.GetWorkflowInstanceID(t, "MI028-E2E-1", "test")

	// Insert and complete analyzer session
	env.InsertAgentSession(t, "sess-e2e-028-a", "MI028-E2E-1", wfiID, "analyzer", "analyzer", "sonnet")

	// Add findings via socket
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"session_id":  "sess-e2e-028-a",
		"instance_id": wfiID,
		"key":         "analysis",
		"value":       "ok",
	}, nil)

	env.CompleteAgentSession(t, "sess-e2e-028-a", "pass")

	// Insert and complete builder session
	env.InsertAgentSession(t, "sess-e2e-028-b", "MI028-E2E-1", wfiID, "builder", "builder", "opus")
	env.CompleteAgentSession(t, "sess-e2e-028-b", "pass")

	// Verify both phases completed via derived status
	statusRaw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "MI028-E2E-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	statusData, _ := json.Marshal(statusRaw)
	var status map[string]interface{}
	json.Unmarshal(statusData, &status)

	phases, _ := status["phases"].(map[string]interface{})
	for _, phase := range []string{"analyzer", "builder"} {
		p, _ := phases[phase].(map[string]interface{})
		if p["status"] != "completed" {
			t.Errorf("expected phase %s=completed, got %v", phase, p["status"])
		}
	}

	// Confirm no phase columns exist (cross-check mid-test)
	for _, col := range []string{"phases", "phase_order", "current_phase"} {
		var count int
		env.Pool.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('workflow_instances') WHERE name = ?`, col).Scan(&count)
		if count != 0 {
			t.Errorf("column %s still exists in workflow_instances", col)
		}
	}

	// Verify sessions recorded
	sessions, err := env.AgentSvc.GetTicketSessions(env.ProjectID, "MI028-E2E-1", "test")
	if err != nil {
		t.Fatalf("failed to get sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}

	// Verify findings preserved (cross-agent read by agent_type + instance_id)
	findings, err := env.FindingsSvc.Get(&types.FindingsGetRequest{
		AgentType:  "analyzer",
		InstanceID: wfiID,
	})
	if err != nil {
		t.Fatalf("failed to get findings: %v", err)
	}
	findingsMap, ok := findings.(map[string]interface{})
	if !ok {
		t.Fatalf("expected findings map, got %T", findings)
	}
	if findingsMap["analysis"] != "ok" {
		t.Errorf("expected findings[analysis]=ok, got %v", findingsMap["analysis"])
	}
}

// TestMigration028InsertWithoutPhaseColumns verifies that direct SQL INSERT into
// workflow_instances succeeds using only the 11 remaining columns.
func TestMigration028InsertWithoutPhaseColumns(t *testing.T) {
	env := NewTestEnv(t)

	// Create a project-scoped workflow for this test
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})
	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "wf-028-proj",
		Description: "Proj workflow 028",
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	// Direct INSERT using only the 11 columns (no phase columns)
	_, err = env.Pool.Exec(`
		INSERT INTO workflow_instances
			(id, project_id, ticket_id, workflow_id, scope_type, status,
			 findings, retry_count, parent_session, created_at, updated_at)
		VALUES
			('direct-028', ?, '', 'wf-028-proj', 'project', 'active',
			 '{}', 0, NULL, datetime('now'), datetime('now'))`,
		env.ProjectID)
	if err != nil {
		t.Fatalf("INSERT with 11 columns failed: %v", err)
	}

	// Verify row was inserted
	var rowStatus string
	err = env.Pool.QueryRow(`SELECT status FROM workflow_instances WHERE id = 'direct-028'`).Scan(&rowStatus)
	if err != nil {
		t.Fatalf("failed to read inserted row: %v", err)
	}
	if rowStatus != "active" {
		t.Errorf("expected status=active, got %s", rowStatus)
	}
}
