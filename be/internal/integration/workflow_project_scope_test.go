package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/types"
)

// TestProjectWorkflowDefCreate tests creating workflow definitions with scope_type=project
func TestProjectWorkflowDefCreate(t *testing.T) {
	env := NewTestEnv(t)

	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
		{"agent": "impl", "layer": 1},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "project-workflow",
		Description: "Project-scoped workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})

	if err != nil {
		t.Fatalf("failed to create project workflow: %v", err)
	}

	// Verify scope_type is persisted
	retrieved, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "project-workflow")
	if err != nil {
		t.Fatalf("failed to retrieve workflow: %v", err)
	}

	if retrieved.ScopeType != "project" {
		t.Fatalf("expected scope_type 'project', got %v", retrieved.ScopeType)
	}
}

// TestProjectWorkflowDefDefaultScopeType tests that workflows default to scope_type=ticket
func TestProjectWorkflowDefDefaultScopeType(t *testing.T) {
	env := NewTestEnv(t)

	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "default-scope",
		Description: "Default scope workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		// ScopeType omitted - should default to "ticket"
	})

	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Verify scope_type defaults to ticket
	retrieved, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "default-scope")
	if err != nil {
		t.Fatalf("failed to retrieve workflow: %v", err)
	}

	if retrieved.ScopeType != "ticket" {
		t.Fatalf("expected default scope_type 'ticket', got %v", retrieved.ScopeType)
	}
}

// TestProjectWorkflowDefInvalidScopeType tests that invalid scope_type values are rejected
func TestProjectWorkflowDefInvalidScopeType(t *testing.T) {
	env := NewTestEnv(t)

	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "invalid-scope",
		Description: "Invalid scope workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "invalid",
	})

	if err == nil {
		t.Fatal("expected error for invalid scope_type, got nil")
	}

	if !strings.Contains(err.Error(), "scope_type") {
		t.Errorf("expected error message to mention scope_type, got: %v", err)
	}
}

// TestProjectWorkflowInit tests initializing a project-scoped workflow
func TestProjectWorkflowInit(t *testing.T) {
	env := NewTestEnv(t)

	// Create project-scoped workflow definition
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
		{"agent": "impl", "layer": 1},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "proj-init-test",
		Description: "Test project workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	// Verify workflow was created
	_, err = env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "proj-init-test")
	if err != nil {
		t.Fatalf("workflow was not created: %v", err)
	}

	// Initialize project workflow
	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "proj-init-test",
		Category: "full",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	// Get workflow instance
	instance, err := env.WorkflowSvc.GetProjectWorkflowInstance(env.ProjectID, "proj-init-test")
	if err != nil {
		t.Fatalf("failed to get project workflow instance: %v", err)
	}

	if instance.ScopeType != "project" {
		t.Fatalf("expected scope_type 'project', got %v", instance.ScopeType)
	}

	if instance.TicketID != "" {
		t.Fatalf("expected empty ticket_id for project scope, got %v", instance.TicketID)
	}

	if instance.WorkflowID != "proj-init-test" {
		t.Fatalf("expected workflow_id 'proj-init-test', got %v", instance.WorkflowID)
	}
}

// TestProjectWorkflowInitDuplicate tests that duplicate project workflow init is rejected
func TestProjectWorkflowInitDuplicate(t *testing.T) {
	env := NewTestEnv(t)

	// Create project-scoped workflow definition
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "proj-dup-test",
		Description: "Test duplicate",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	// First init
	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "proj-dup-test",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	// Second init should fail
	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "proj-dup-test",
	})
	if err == nil {
		t.Fatal("expected error for duplicate project workflow init, got nil")
	}
}

// TestProjectWorkflowInitWrongScope tests that InitProjectWorkflow rejects ticket-scoped workflows
func TestProjectWorkflowInitWrongScope(t *testing.T) {
	env := NewTestEnv(t)

	// Create ticket-scoped workflow definition
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "ticket-scope-def",
		Description: "Ticket-scoped workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "ticket",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	// Try to init as project workflow
	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "ticket-scope-def",
	})
	if err == nil {
		t.Fatal("expected error when initing ticket-scoped workflow as project workflow, got nil")
	}

	if !strings.Contains(err.Error(), "scope") {
		t.Errorf("expected error to mention scope mismatch, got: %v", err)
	}
}

// TestTicketWorkflowInitWrongScope tests that Init rejects project-scoped workflows
func TestTicketWorkflowInitWrongScope(t *testing.T) {
	env := NewTestEnv(t)

	// Create project-scoped workflow definition
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "proj-scope-def",
		Description: "Project-scoped workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	// Try to init on a ticket
	env.CreateTicket(t, "TICKET-1", "Test ticket")
	err = env.WorkflowSvc.Init(env.ProjectID, "TICKET-1", &types.WorkflowInitRequest{
		Workflow: "proj-scope-def",
	})
	if err == nil {
		t.Fatal("expected error when initing project-scoped workflow on ticket, got nil")
	}

	if !strings.Contains(err.Error(), "scope") {
		t.Errorf("expected error to mention scope mismatch, got: %v", err)
	}
}

// TestProjectWorkflowStateRetrieval tests GET /api/v1/projects/{id}/workflow equivalent
func TestProjectWorkflowStateRetrieval(t *testing.T) {
	env := NewTestEnv(t)

	// Create and init project workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "proj-state-test",
		Description: "Test state retrieval",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "proj-state-test",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	// List project workflow instances
	instances, err := env.WorkflowSvc.ListProjectWorkflowInstances(env.ProjectID)
	if err != nil {
		t.Fatalf("failed to list project workflow instances: %v", err)
	}

	if len(instances) != 1 {
		t.Fatalf("expected 1 project workflow instance, got %d", len(instances))
	}

	if instances[0].WorkflowID != "proj-state-test" {
		t.Fatalf("expected workflow_id 'proj-state-test', got %v", instances[0].WorkflowID)
	}

	if instances[0].ScopeType != "project" {
		t.Fatalf("expected scope_type 'project', got %v", instances[0].ScopeType)
	}

	// Get state for the instance
	state, err := env.WorkflowSvc.GetStatusByInstance(instances[0])
	if err != nil {
		t.Fatalf("failed to get workflow state: %v", err)
	}

	stateMap := state

	if stateMap["scope_type"] != "project" {
		t.Fatalf("expected scope_type 'project' in state, got %v", stateMap["scope_type"])
	}

	if stateMap["workflow"] != "proj-state-test" {
		t.Fatalf("expected workflow 'proj-state-test' in state, got %v", stateMap["workflow"])
	}
}

// TestProjectWorkflowListEmpty tests listing when no project workflows exist
func TestProjectWorkflowListEmpty(t *testing.T) {
	env := NewTestEnv(t)

	instances, err := env.WorkflowSvc.ListProjectWorkflowInstances(env.ProjectID)
	if err != nil {
		t.Fatalf("failed to list project workflow instances: %v", err)
	}

	if len(instances) != 0 {
		t.Fatalf("expected 0 project workflow instances, got %d", len(instances))
	}
}

// TestProjectWorkflowUniqueConstraint tests the unique index on (project_id, ticket_id, workflow_id, scope_type)
func TestProjectWorkflowUniqueConstraint(t *testing.T) {
	env := NewTestEnv(t)

	// Create workflow definition
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "unique-test",
		Description: "Test unique constraint",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	// First init
	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "unique-test",
	})
	if err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Second init with same project + workflow should fail
	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "unique-test",
	})
	if err == nil {
		t.Fatal("expected error for duplicate project workflow, got nil")
	}
}

// TestProjectWorkflowBackwardCompatibility tests that existing ticket workflows still work
func TestProjectWorkflowBackwardCompatibility(t *testing.T) {
	env := NewTestEnv(t)

	// Test that the default "test" workflow (ticket-scoped) still works
	env.CreateTicket(t, "COMPAT-1", "Backward compat test")
	env.InitWorkflow(t, "COMPAT-1")

	// Verify workflow instance has scope_type=ticket
	wfi := env.GetWorkflowInstanceID(t, "COMPAT-1", "test")
	var scopeType string
	err := env.Pool.QueryRow(`SELECT scope_type FROM workflow_instances WHERE id = ?`, wfi).Scan(&scopeType)
	if err != nil {
		t.Fatalf("failed to query scope_type: %v", err)
	}

	if scopeType != "ticket" {
		t.Fatalf("expected scope_type 'ticket', got %v", scopeType)
	}

	// Verify workflow operates normally
	status, err := getWorkflowStatus(t, env, "COMPAT-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	if status["scope_type"] != "ticket" {
		t.Fatalf("expected scope_type 'ticket' in status, got %v", status["scope_type"])
	}

	// Verify phase operations work
	env.StartPhase(t, "COMPAT-1", "analyzer")
	env.CompletePhase(t, "COMPAT-1", "analyzer", "pass")

	status, err = getWorkflowStatus(t, env, "COMPAT-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get workflow status after phase completion: %v", err)
	}

	phases, _ := status["phases"].(map[string]interface{})
	analyzerPhase, _ := phases["analyzer"].(map[string]interface{})
	if analyzerPhase["result"] != "pass" {
		t.Fatalf("expected analyzer result 'pass', got %v", analyzerPhase["result"])
	}
}

// TestProjectWorkflowMixedScopes tests that ticket and project workflows can coexist
func TestProjectWorkflowMixedScopes(t *testing.T) {
	env := NewTestEnv(t)

	// Create project-scoped workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "mixed-project",
		Description: "Project workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create project workflow: %v", err)
	}

	// Create ticket-scoped workflow
	_, err = env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "mixed-ticket",
		Description: "Ticket workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "ticket",
	})
	if err != nil {
		t.Fatalf("failed to create ticket workflow: %v", err)
	}

	// Init project workflow
	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "mixed-project",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	// Init ticket workflow
	env.CreateTicket(t, "MIXED-1", "Mixed test")
	err = env.WorkflowSvc.Init(env.ProjectID, "MIXED-1", &types.WorkflowInitRequest{
		Workflow: "mixed-ticket",
	})
	if err != nil {
		t.Fatalf("failed to init ticket workflow: %v", err)
	}

	// Verify both exist
	projectInstances, err := env.WorkflowSvc.ListProjectWorkflowInstances(env.ProjectID)
	if err != nil {
		t.Fatalf("failed to list project workflows: %v", err)
	}

	if len(projectInstances) != 1 || projectInstances[0].WorkflowID != "mixed-project" {
		t.Fatalf("expected 1 project workflow 'mixed-project', got %d workflows", len(projectInstances))
	}

	ticketStatus, err := getWorkflowStatus(t, env, "MIXED-1", &types.WorkflowGetRequest{
		Workflow: "mixed-ticket",
	})
	if err != nil {
		t.Fatalf("failed to get ticket workflow status: %v", err)
	}

	if ticketStatus["workflow"] != "mixed-ticket" {
		t.Fatalf("expected workflow 'mixed-ticket', got %v", ticketStatus["workflow"])
	}
}
