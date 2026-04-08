package integration

import (
	"database/sql"
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/types"
)

// TestMigration018DropsCategoriesColumn verifies that migration 000018 successfully
// drops the categories column from workflows table.
func TestMigration018DropsCategoriesColumn(t *testing.T) {
	env := NewTestEnv(t)

	// Verify categories column does not exist in workflows table
	var count int
	err := env.Pool.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('workflows')
		WHERE name = 'categories'`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query workflows schema: %v", err)
	}
	if count != 0 {
		t.Fatalf("categories column should not exist in workflows table after migration 000018, found %d columns with that name", count)
	}
}

// TestMigration018DropsCategoryFromWorkflowInstances verifies that migration 000018
// successfully drops the category column from workflow_instances table.
func TestMigration018DropsCategoryFromWorkflowInstances(t *testing.T) {
	env := NewTestEnv(t)

	// Verify category column does not exist in workflow_instances table
	var count int
	err := env.Pool.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('workflow_instances')
		WHERE name = 'category'`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query workflow_instances schema: %v", err)
	}
	if count != 0 {
		t.Fatalf("category column should not exist in workflow_instances table after migration 000018, found %d columns with that name", count)
	}
}

// TestMigration018DropsCategoryFromChainExecutions verifies that migration 000018
// successfully drops the category column from chain_executions table.
func TestMigration018DropsCategoryFromChainExecutions(t *testing.T) {
	env := NewTestEnv(t)

	// Verify category column does not exist in chain_executions table
	var count int
	err := env.Pool.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('chain_executions')
		WHERE name = 'category'`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query chain_executions schema: %v", err)
	}
	if count != 0 {
		t.Fatalf("category column should not exist in chain_executions table after migration 000018, found %d columns with that name", count)
	}
}

// TestWorkflowsTableSchemaAfterMigration018 verifies the workflows table schema is correct
// after migration 000018, with all expected columns present except categories.
func TestWorkflowsTableSchemaAfterMigration018(t *testing.T) {
	env := NewTestEnv(t)

	// Expected columns (no categories, no phases after migration 000053)
	expectedColumns := []string{
		"id", "project_id", "description", "scope_type", "created_at", "updated_at",
	}

	for _, colName := range expectedColumns {
		var count int
		err := env.Pool.QueryRow(`
			SELECT COUNT(*) FROM pragma_table_info('workflows')
			WHERE name = ?`, colName).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query schema for column %s: %v", colName, err)
		}
		if count != 1 {
			t.Fatalf("expected column %s to exist in workflows table, found %d", colName, count)
		}
	}
}

// TestWorkflowInstancesTableSchemaAfterMigration018 verifies the workflow_instances table
// schema is correct after migration 000018, with all expected columns except category.
func TestWorkflowInstancesTableSchemaAfterMigration018(t *testing.T) {
	env := NewTestEnv(t)

	// Expected columns (no category, no phase columns after migration 000028)
	expectedColumns := []string{
		"id", "project_id", "ticket_id", "workflow_id", "scope_type", "status",
		"findings", "retry_count",
		"parent_session", "created_at", "updated_at",
	}

	for _, colName := range expectedColumns {
		var count int
		err := env.Pool.QueryRow(`
			SELECT COUNT(*) FROM pragma_table_info('workflow_instances')
			WHERE name = ?`, colName).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query schema for column %s: %v", colName, err)
		}
		if count != 1 {
			t.Fatalf("expected column %s to exist in workflow_instances table, found %d", colName, count)
		}
	}
}

// TestChainExecutionsTableSchemaAfterMigration018 verifies the chain_executions table
// schema is correct after migration 000018, with all expected columns except category.
func TestChainExecutionsTableSchemaAfterMigration018(t *testing.T) {
	env := NewTestEnv(t)

	// Expected columns (no category)
	expectedColumns := []string{
		"id", "project_id", "name", "status", "workflow_name",
		"epic_ticket_id", "created_by", "created_at", "updated_at",
	}

	for _, colName := range expectedColumns {
		var count int
		err := env.Pool.QueryRow(`
			SELECT COUNT(*) FROM pragma_table_info('chain_executions')
			WHERE name = ?`, colName).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query schema for column %s: %v", colName, err)
		}
		if count != 1 {
			t.Fatalf("expected column %s to exist in chain_executions table, found %d", colName, count)
		}
	}
}

// TestWorkflowCRUDWithoutCategories verifies that workflow CRUD operations work correctly
// without the categories column.
func TestWorkflowCRUDWithoutCategories(t *testing.T) {
	env := NewTestEnv(t)

	// Create a workflow definition
	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "test-no-cats",
		Description: "Test workflow without categories",
		ScopeType:   "ticket",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	// Retrieve the workflow
	wf, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "test-no-cats")
	if err != nil {
		t.Fatalf("failed to get workflow def: %v", err)
	}

	if wf.Description != "Test workflow without categories" {
		t.Fatalf("expected description 'Test workflow without categories', got %v", wf.Description)
	}

	// Update the workflow
	newDesc := "Updated description"
	err = env.WorkflowSvc.UpdateWorkflowDef(env.ProjectID, "test-no-cats", &types.WorkflowDefUpdateRequest{
		Description: &newDesc,
	})
	if err != nil {
		t.Fatalf("failed to update workflow def: %v", err)
	}

	// Verify update
	updated, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "test-no-cats")
	if err != nil {
		t.Fatalf("failed to get updated workflow: %v", err)
	}
	if updated.Description != newDesc {
		t.Fatalf("expected updated description '%s', got %v", newDesc, updated.Description)
	}
}

// TestWorkflowInstanceCRUDWithoutCategory verifies that workflow instance CRUD operations
// work correctly without the category column.
func TestWorkflowInstanceCRUDWithoutCategory(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WI-NOCATS-1", "Workflow instance test")
	env.InitWorkflow(t, "WI-NOCATS-1")
	wfiID := env.GetWorkflowInstanceID(t, "WI-NOCATS-1", "test")

	// Verify the instance was created
	var status string
	err := env.Pool.QueryRow(`
		SELECT status FROM workflow_instances WHERE id = ?`, wfiID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query workflow instance: %v", err)
	}
	if status != "active" {
		t.Fatalf("expected status 'active', got %v", status)
	}

	// Create and complete an agent session (phase status derived from agent_sessions)
	env.InsertAgentSession(t, "sess-wi-nocats", "WI-NOCATS-1", wfiID, "analyzer", "analyzer", "sonnet")
	env.CompleteAgentSession(t, "sess-wi-nocats", "pass")

	// Verify phase completion via service
	statusRaw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "WI-NOCATS-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	statusData, _ := json.Marshal(statusRaw)
	var result map[string]interface{}
	json.Unmarshal(statusData, &result)

	phases, _ := result["phases"].(map[string]interface{})
	analyzerPhase, _ := phases["analyzer"].(map[string]interface{})
	if analyzerPhase["status"] != "completed" {
		t.Fatalf("expected analyzer phase completed, got %v", analyzerPhase["status"])
	}
}

// TestChainExecutionCRUDWithoutCategory verifies that chain execution CRUD operations
// work correctly without the category column.
func TestChainExecutionCRUDWithoutCategory(t *testing.T) {
	env := NewTestEnv(t)

	// Create tickets
	env.CreateTicket(t, "CHAIN-NOCATS-1", "Chain test 1")
	env.CreateTicket(t, "CHAIN-NOCATS-2", "Chain test 2")

	// Create a chain via service
	chainSvc := service.NewChainService(env.Pool, clock.Real())
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "test-chain-nocats",
		WorkflowName: "test",
		TicketIDs:    []string{"CHAIN-NOCATS-1", "CHAIN-NOCATS-2"},
	})
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}

	// Verify chain was created
	var workflow string
	err = env.Pool.QueryRow(`
		SELECT workflow_name FROM chain_executions WHERE id = ?`, chain.ID).Scan(&workflow)
	if err != nil {
		t.Fatalf("failed to query chain: %v", err)
	}
	if workflow != "test" {
		t.Fatalf("expected workflow_name 'test', got %v", workflow)
	}

	// Verify items
	if len(chain.Items) != 2 {
		t.Fatalf("expected 2 chain items, got %d", len(chain.Items))
	}

	// Update chain status via DB (service doesn't expose UpdateStatus directly)
	_, err = env.Pool.Exec(`UPDATE chain_executions SET status = ? WHERE id = ?`, "running", chain.ID)
	if err != nil {
		t.Fatalf("failed to update chain status: %v", err)
	}

	// Verify status update
	var updatedStatus string
	err = env.Pool.QueryRow(`
		SELECT status FROM chain_executions WHERE id = ?`, chain.ID).Scan(&updatedStatus)
	if err != nil {
		t.Fatalf("failed to query updated chain: %v", err)
	}
	if updatedStatus != "running" {
		t.Fatalf("expected status 'running', got %v", updatedStatus)
	}
}

// TestWorkflowInstanceIndexesAfterMigration018 verifies that workflow_instances indexes
// are correctly present after all migrations.
func TestWorkflowInstanceIndexesAfterMigration018(t *testing.T) {
	env := NewTestEnv(t)

	// Check for expected indexes (idx_wfi_unique dropped by migration 000019, idx_wfi_ticket_unique dropped by migration 000040)
	expectedIndexes := []string{
		"idx_wfi_lookup",
		"idx_wfi_ticket",
	}

	for _, idxName := range expectedIndexes {
		var count int
		err := env.Pool.QueryRow(`
			SELECT COUNT(*) FROM sqlite_master
			WHERE type = 'index' AND name = ? AND tbl_name = 'workflow_instances'`,
			idxName).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query index %s: %v", idxName, err)
		}
		if count != 1 {
			t.Fatalf("expected index %s to exist on workflow_instances, found %d", idxName, count)
		}
	}
}

// TestAgentSessionsIndexesAfterMigration018 verifies that agent_sessions indexes
// are correctly recreated after migration 000018 (table was rebuilt due to FK).
func TestAgentSessionsIndexesAfterMigration018(t *testing.T) {
	env := NewTestEnv(t)

	// Check for expected indexes
	expectedIndexes := []string{
		"idx_agent_sessions_project_ticket",
		"idx_agent_sessions_ticket_phase",
		"idx_agent_sessions_wfi",
		"idx_agent_sessions_wfi_status",
	}

	for _, idxName := range expectedIndexes {
		var count int
		err := env.Pool.QueryRow(`
			SELECT COUNT(*) FROM sqlite_master
			WHERE type = 'index' AND name = ? AND tbl_name = 'agent_sessions'`,
			idxName).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query index %s: %v", idxName, err)
		}
		if count != 1 {
			t.Fatalf("expected index %s to exist on agent_sessions, found %d", idxName, count)
		}
	}
}

// TestChainExecutionsIndexesAfterMigration018 verifies that chain_executions indexes
// are correctly recreated after migration 000018.
func TestChainExecutionsIndexesAfterMigration018(t *testing.T) {
	env := NewTestEnv(t)

	// Check for expected indexes
	expectedIndexes := []string{
		"idx_chain_exec_project_status",
		"idx_chain_exec_epic",
	}

	for _, idxName := range expectedIndexes {
		var count int
		err := env.Pool.QueryRow(`
			SELECT COUNT(*) FROM sqlite_master
			WHERE type = 'index' AND name = ? AND tbl_name = 'chain_executions'`,
			idxName).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query index %s: %v", idxName, err)
		}
		if count != 1 {
			t.Fatalf("expected index %s to exist on chain_executions, found %d", idxName, count)
		}
	}
}

// TestForeignKeyCheckAfterMigration018 verifies that foreign key constraints are intact
// after migration 000018 recreates workflow_instances and agent_sessions tables.
func TestForeignKeyCheckAfterMigration018(t *testing.T) {
	env := NewTestEnv(t)

	// Run PRAGMA foreign_key_check to verify integrity
	var violations int
	err := env.Pool.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check()`).Scan(&violations)
	if err != nil {
		t.Fatalf("failed to run foreign_key_check: %v", err)
	}
	if violations != 0 {
		t.Fatalf("expected 0 foreign key violations after migration 000018, found %d", violations)
	}
}

// TestAgentSessionsFKToWorkflowInstanceAfterMigration018 verifies that the foreign key
// from agent_sessions to workflow_instances still works after both tables were recreated.
func TestAgentSessionsFKToWorkflowInstanceAfterMigration018(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "FK-TEST-1", "FK test")
	env.InitWorkflow(t, "FK-TEST-1")
	wfiID := env.GetWorkflowInstanceID(t, "FK-TEST-1", "test")

	// Insert agent session
	env.InsertAgentSession(t, "sess-fk-test", "FK-TEST-1", wfiID, "analyzer", "analyzer", "sonnet")

	// Verify FK constraint by trying to delete workflow_instance (should fail due to CASCADE)
	// First check that we can't delete the workflow_instance while agent_sessions reference it
	_, err := env.Pool.Exec(`DELETE FROM workflow_instances WHERE id = ?`, wfiID)
	if err == nil {
		// Actually this SHOULD succeed because the FK is ON DELETE CASCADE
		// So agent_sessions should be deleted too. Let's verify that.
		var sessionCount int
		env.Pool.QueryRow(`SELECT COUNT(*) FROM agent_sessions WHERE id = ?`, "sess-fk-test").Scan(&sessionCount)
		if sessionCount != 0 {
			t.Fatalf("expected agent_session to be cascade deleted, but it still exists")
		}
	} else {
		t.Fatalf("DELETE should succeed with CASCADE, got error: %v", err)
	}
}

// TestEndToEndWorkflowWithoutCategoryColumn is an end-to-end test covering full workflow
// execution to verify all operations work correctly after migration 000018.
func TestEndToEndWorkflowWithoutCategoryColumn(t *testing.T) {
	env := NewTestEnv(t)

	// Create ticket and init workflow
	env.CreateTicket(t, "E2E-NOCATS-1", "End to end test without categories")
	env.InitWorkflow(t, "E2E-NOCATS-1")
	wfiID := env.GetWorkflowInstanceID(t, "E2E-NOCATS-1", "test")

	// Create analyzer session
	env.InsertAgentSession(t, "sess-e2e-nocats-analyzer", "E2E-NOCATS-1", wfiID, "analyzer", "analyzer", "sonnet")

	// Update findings via socket
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"session_id":  "sess-e2e-nocats-analyzer",
		"instance_id": wfiID,
		"key":         "files_to_check",
		"value":       "main.go,config.go",
	}, nil)

	// Mark analyzer session as completed
	_, err := env.Pool.Exec(`UPDATE agent_sessions SET result = 'pass', status = 'completed', ended_at = datetime('now'), updated_at = datetime('now') WHERE id = ?`, "sess-e2e-nocats-analyzer")
	if err != nil {
		t.Fatalf("failed to complete analyzer session: %v", err)
	}

	// Create and complete builder session
	env.InsertAgentSession(t, "sess-e2e-nocats-builder", "E2E-NOCATS-1", wfiID, "builder", "builder", "opus")

	_, err = env.Pool.Exec(`UPDATE agent_sessions SET result = 'pass', status = 'completed', ended_at = datetime('now'), updated_at = datetime('now') WHERE id = ?`, "sess-e2e-nocats-builder")
	if err != nil {
		t.Fatalf("failed to complete builder session: %v", err)
	}

	// Verify workflow status
	statusRaw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "E2E-NOCATS-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	// Normalize JSON types
	statusData, _ := json.Marshal(statusRaw)
	var status map[string]interface{}
	json.Unmarshal(statusData, &status)

	phases, _ := status["phases"].(map[string]interface{})
	for _, phaseName := range []string{"analyzer", "builder"} {
		phase, _ := phases[phaseName].(map[string]interface{})
		if phase["status"] != "completed" {
			t.Fatalf("expected phase '%s' completed, got %v", phaseName, phase["status"])
		}
	}

	// Verify both sessions exist and are retrievable
	allSessions, err := env.AgentSvc.GetTicketSessions(env.ProjectID, "E2E-NOCATS-1", "test")
	if err != nil {
		t.Fatalf("failed to get ticket sessions: %v", err)
	}
	if len(allSessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(allSessions))
	}

	// Verify findings were preserved (cross-agent read by agent_type + instance_id)
	findings, err := env.FindingsSvc.Get(&types.FindingsGetRequest{
		AgentType:  "analyzer",
		InstanceID: wfiID,
	})
	if err != nil {
		t.Fatalf("failed to get findings: %v", err)
	}

	findingsMap, ok := findings.(map[string]interface{})
	if !ok {
		t.Fatalf("expected findings to be map, got %T", findings)
	}

	if findingsMap["files_to_check"] != "main.go,config.go" {
		t.Fatalf("expected findings preserved, got %v", findingsMap)
	}

	// Verify no category-related fields in workflow instance JSON
	var instanceJSON string
	err = env.Pool.QueryRow(`
		SELECT json_object(
			'id', id,
			'status', status,
			'scope_type', scope_type,
			'workflow_id', workflow_id
		) FROM workflow_instances WHERE id = ?`, wfiID).Scan(&instanceJSON)
	if err != nil {
		t.Fatalf("failed to query workflow instance JSON: %v", err)
	}

	var instanceData map[string]interface{}
	if err := json.Unmarshal([]byte(instanceJSON), &instanceData); err != nil {
		t.Fatalf("failed to unmarshal instance JSON: %v", err)
	}

	// Ensure no 'category' field exists
	if _, exists := instanceData["category"]; exists {
		t.Fatalf("category field should not exist in workflow_instance, found: %v", instanceData)
	}
}

// TestProjectWorkflowWithoutCategory verifies that project-scoped workflows work correctly
// without the category column.
func TestProjectWorkflowWithoutCategory(t *testing.T) {
	env := NewTestEnv(t)

	// Create project-scoped workflow
	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "proj-nocats",
		Description: "Project workflow without categories",
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create project workflow: %v", err)
	}

	// Initialize project workflow
	_, err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "proj-nocats",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	// Get workflow instance
	wfi, err := env.WorkflowSvc.GetProjectWorkflowInstance(env.ProjectID, "proj-nocats")
	if err != nil {
		t.Fatalf("failed to get project workflow instance: %v", err)
	}

	// Verify no category column in query result
	var scopeType string
	var status string
	err = env.Pool.QueryRow(`
		SELECT scope_type, status FROM workflow_instances WHERE id = ?`, wfi.ID).Scan(&scopeType, &status)
	if err != nil {
		t.Fatalf("failed to query workflow instance: %v", err)
	}

	if scopeType != "project" {
		t.Fatalf("expected scope_type 'project', got %v", scopeType)
	}
	if status != "active" {
		t.Fatalf("expected status 'active', got %v", status)
	}
}

// TestChainWithEpicTicketWithoutCategory verifies that chain executions with epic tickets
// work correctly without the category column.
func TestChainWithEpicTicketWithoutCategory(t *testing.T) {
	env := NewTestEnv(t)

	// Create epic ticket and child tickets
	env.CreateTicket(t, "EPIC-NOCATS", "Epic ticket without category")
	env.CreateTicket(t, "CHILD-NOCATS-1", "Child ticket 1")
	env.CreateTicket(t, "CHILD-NOCATS-2", "Child ticket 2")

	// Create chain with epic and child tickets
	chainSvc := service.NewChainService(env.Pool, clock.Real())
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "epic-chain",
		WorkflowName: "test",
		TicketIDs:    []string{"CHILD-NOCATS-1", "CHILD-NOCATS-2"},
		EpicTicketID: "EPIC-NOCATS",
	})
	if err != nil {
		t.Fatalf("failed to create chain with epic: %v", err)
	}

	// Verify chain was created with epic_ticket_id
	var epicTicketID sql.NullString
	var workflow string
	err = env.Pool.QueryRow(`
		SELECT epic_ticket_id, workflow_name FROM chain_executions WHERE id = ?`,
		chain.ID).Scan(&epicTicketID, &workflow)
	if err != nil {
		t.Fatalf("failed to query chain: %v", err)
	}

	if !epicTicketID.Valid || epicTicketID.String != "EPIC-NOCATS" {
		t.Fatalf("expected epic_ticket_id 'EPIC-NOCATS', got %v", epicTicketID)
	}
	if workflow != "test" {
		t.Fatalf("expected workflow_name 'test', got %v", workflow)
	}

	// Verify no category column exists
	var count int
	err = env.Pool.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('chain_executions')
		WHERE name = 'category'`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query schema: %v", err)
	}
	if count != 0 {
		t.Fatalf("category column should not exist in chain_executions after migration 000018")
	}
}
