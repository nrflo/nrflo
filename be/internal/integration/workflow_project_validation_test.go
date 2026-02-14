package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/service"
	"be/internal/types"
)

// TestValidateScopeType tests the ValidateScopeType function
func TestValidateScopeType(t *testing.T) {
	tests := []struct {
		name      string
		scopeType string
		expectErr bool
	}{
		{"empty string (default)", "", false},
		{"ticket", "ticket", false},
		{"project", "project", false},
		{"invalid", "invalid", true},
		{"uppercase", "TICKET", true},
		{"mixed case", "Project", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.ValidateScopeType(tt.scopeType)
			if tt.expectErr && err == nil {
				t.Errorf("expected error for scope_type '%s', got nil", tt.scopeType)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error for scope_type '%s', got: %v", tt.scopeType, err)
			}
		})
	}
}

// TestAgentSessionNullTicketID tests that agent_sessions can have NULL ticket_id for project scope
func TestAgentSessionNullTicketID(t *testing.T) {
	env := NewTestEnv(t)

	// Create project workflow instance
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "session-null-test",
		Description: "Test null ticket_id",
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "session-null-test",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	// Get workflow instance ID
	wfi, err := env.WorkflowSvc.GetProjectWorkflowInstance(env.ProjectID, "session-null-test")
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}

	// Insert agent session with empty ticket_id
	sessionID := "test-session-" + t.Name()
	env.InsertAgentSession(t, sessionID, "", wfi.ID, "setup", "setup", "claude:sonnet")

	// Verify session was created
	var retrievedTicketID *string
	err = env.Pool.QueryRow(`
		SELECT ticket_id FROM agent_sessions WHERE id = ?
	`, sessionID).Scan(&retrievedTicketID)
	if err != nil {
		t.Fatalf("failed to query agent session: %v", err)
	}

	if retrievedTicketID != nil && *retrievedTicketID != "" {
		t.Fatalf("expected NULL or empty ticket_id, got %v", *retrievedTicketID)
	}
}

// TestWorkflowInstanceScopeTypeColumn tests that scope_type column exists and has correct values
func TestWorkflowInstanceScopeTypeColumn(t *testing.T) {
	env := NewTestEnv(t)

	// Check ticket-scoped instance
	env.CreateTicket(t, "SCOPE-TEST-1", "Scope test")
	env.InitWorkflow(t, "SCOPE-TEST-1")

	wfiID := env.GetWorkflowInstanceID(t, "SCOPE-TEST-1", "test")
	var ticketScopeType string
	err := env.Pool.QueryRow(`SELECT scope_type FROM workflow_instances WHERE id = ?`, wfiID).Scan(&ticketScopeType)
	if err != nil {
		t.Fatalf("failed to query ticket workflow scope_type: %v", err)
	}

	if ticketScopeType != "ticket" {
		t.Fatalf("expected ticket scope_type, got %v", ticketScopeType)
	}

	// Check project-scoped instance
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err = env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "scope-col-test",
		Description: "Test scope column",
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "scope-col-test",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	wfi, err := env.WorkflowSvc.GetProjectWorkflowInstance(env.ProjectID, "scope-col-test")
	if err != nil {
		t.Fatalf("failed to get project workflow instance: %v", err)
	}

	var projectScopeType string
	err = env.Pool.QueryRow(`SELECT scope_type FROM workflow_instances WHERE id = ?`, wfi.ID).Scan(&projectScopeType)
	if err != nil {
		t.Fatalf("failed to query project workflow scope_type: %v", err)
	}

	if projectScopeType != "project" {
		t.Fatalf("expected project scope_type, got %v", projectScopeType)
	}
}

// TestWorkflowScopeTypeColumn tests that workflows table has scope_type column
func TestWorkflowScopeTypeColumn(t *testing.T) {
	env := NewTestEnv(t)

	// Check default "test" workflow (ticket-scoped)
	wf, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "test")
	if err != nil {
		t.Fatalf("failed to get test workflow: %v", err)
	}

	if wf.ScopeType != "ticket" {
		t.Fatalf("expected test workflow scope_type 'ticket', got %v", wf.ScopeType)
	}

	// Create project-scoped workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err = env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "proj-wf-scope",
		Description: "Project workflow",
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	projWorkflow, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "proj-wf-scope")
	if err != nil {
		t.Fatalf("failed to get project workflow: %v", err)
	}

	if projWorkflow.ScopeType != "project" {
		t.Fatalf("expected scope_type 'project', got %v", projWorkflow.ScopeType)
	}
}

// TestProjectWorkflowMigrationDefaults tests that existing workflows have scope_type=ticket
func TestProjectWorkflowMigrationDefaults(t *testing.T) {
	env := NewTestEnv(t)

	// The "test" workflow seeded in testenv should have scope_type=ticket
	wf, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "test")
	if err != nil {
		t.Fatalf("failed to get test workflow: %v", err)
	}

	if wf.ScopeType != "ticket" {
		t.Fatalf("expected default workflow scope_type 'ticket', got %v", wf.ScopeType)
	}

	// Test workflow instances also default to ticket scope
	env.CreateTicket(t, "MIG-TEST-1", "Migration test")
	env.InitWorkflow(t, "MIG-TEST-1")

	wfiID := env.GetWorkflowInstanceID(t, "MIG-TEST-1", "test")
	var scopeType string
	err = env.Pool.QueryRow(`SELECT scope_type FROM workflow_instances WHERE id = ?`, wfiID).Scan(&scopeType)
	if err != nil {
		t.Fatalf("failed to query scope_type: %v", err)
	}

	if scopeType != "ticket" {
		t.Fatalf("expected workflow instance scope_type 'ticket', got %v", scopeType)
	}
}

// TestProjectWorkflowUpdateScopeType tests updating workflow scope_type
func TestProjectWorkflowUpdateScopeType(t *testing.T) {
	env := NewTestEnv(t)

	// Create ticket-scoped workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "update-scope",
		Description: "Test scope update",
		Phases:      phasesJSON,
		ScopeType:   "ticket",
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Update to project scope
	projectScope := "project"
	err = env.WorkflowSvc.UpdateWorkflowDef(env.ProjectID, "update-scope", &types.WorkflowDefUpdateRequest{
		ScopeType: &projectScope,
	})
	if err != nil {
		t.Fatalf("failed to update workflow scope: %v", err)
	}

	// Verify update
	wf, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "update-scope")
	if err != nil {
		t.Fatalf("failed to get updated workflow: %v", err)
	}

	if wf.ScopeType != "project" {
		t.Fatalf("expected updated scope_type 'project', got %v", wf.ScopeType)
	}

	// Update back to ticket scope
	ticketScope := "ticket"
	err = env.WorkflowSvc.UpdateWorkflowDef(env.ProjectID, "update-scope", &types.WorkflowDefUpdateRequest{
		ScopeType: &ticketScope,
	})
	if err != nil {
		t.Fatalf("failed to update workflow scope back to ticket: %v", err)
	}

	wf, err = env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "update-scope")
	if err != nil {
		t.Fatalf("failed to get workflow after second update: %v", err)
	}

	if wf.ScopeType != "ticket" {
		t.Fatalf("expected updated scope_type 'ticket', got %v", wf.ScopeType)
	}
}

// TestProjectWorkflowInvalidScopeTypeUpdate tests rejecting invalid scope_type values on update
func TestProjectWorkflowInvalidScopeTypeUpdate(t *testing.T) {
	env := NewTestEnv(t)

	// Create workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "invalid-update",
		Description: "Test invalid update",
		Phases:      phasesJSON,
		ScopeType:   "ticket",
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Try to update with invalid scope_type
	invalidScope := "invalid"
	err = env.WorkflowSvc.UpdateWorkflowDef(env.ProjectID, "invalid-update", &types.WorkflowDefUpdateRequest{
		ScopeType: &invalidScope,
	})
	if err == nil {
		t.Fatal("expected error for invalid scope_type update, got nil")
	}

	if !strings.Contains(err.Error(), "scope_type") {
		t.Errorf("expected error to mention scope_type, got: %v", err)
	}
}
