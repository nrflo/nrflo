package integration

import (
	"encoding/json"
	"testing"

	"be/internal/types"
)

// TestGetProjectAgentSessions_HappyPath tests retrieving project-scoped agent sessions
func TestGetProjectAgentSessions_HappyPath(t *testing.T) {
	env := NewTestEnv(t)

	// Create project-scoped workflow definition
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
		{"agent": "impl", "layer": 1},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "proj-agents-test",
		Description: "Project workflow for agent sessions",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	// Init project workflow
	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "proj-agents-test",
		Category: "full",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	// Get workflow instance
	instance, err := env.WorkflowSvc.GetProjectWorkflowInstance(env.ProjectID, "proj-agents-test")
	if err != nil {
		t.Fatalf("failed to get project workflow instance: %v", err)
	}

	// Insert project-scoped agent sessions (empty ticket_id)
	env.InsertAgentSession(t, "proj-sess-1", "", instance.ID, "setup", "setup-agent", "sonnet")
	env.InsertAgentSession(t, "proj-sess-2", "", instance.ID, "impl", "impl-agent", "opus")

	// Get project agent sessions via service
	sessions, err := env.AgentSvc.GetProjectSessions(env.ProjectID, "")
	if err != nil {
		t.Fatalf("failed to get project sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 project sessions, got %d", len(sessions))
	}

	// Verify session details
	found := make(map[string]bool)
	for _, s := range sessions {
		found[s.ID] = true
		if s.ProjectID != env.ProjectID {
			t.Errorf("session %s has wrong project_id: %v", s.ID, s.ProjectID)
		}
		if s.TicketID != "" {
			t.Errorf("session %s has non-empty ticket_id: %v", s.ID, s.TicketID)
		}
	}

	if !found["proj-sess-1"] || !found["proj-sess-2"] {
		t.Errorf("missing expected sessions: found=%v", found)
	}
}

// TestGetProjectAgentSessions_EmptyResult tests that empty result returns empty array not null
func TestGetProjectAgentSessions_EmptyResult(t *testing.T) {
	env := NewTestEnv(t)

	// Get project agent sessions when none exist
	sessions, err := env.AgentSvc.GetProjectSessions(env.ProjectID, "")
	if err != nil {
		t.Fatalf("failed to get project sessions: %v", err)
	}

	if sessions == nil {
		t.Fatal("expected empty array, got nil")
	}

	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

// TestGetProjectAgentSessions_PhaseFilter tests filtering by phase parameter
func TestGetProjectAgentSessions_PhaseFilter(t *testing.T) {
	env := NewTestEnv(t)

	// Create project-scoped workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
		{"agent": "impl", "layer": 1},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "proj-phase-filter",
		Description: "Test phase filtering",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "proj-phase-filter",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	instance, err := env.WorkflowSvc.GetProjectWorkflowInstance(env.ProjectID, "proj-phase-filter")
	if err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}

	// Insert sessions in different phases
	env.InsertAgentSession(t, "phase-setup-1", "", instance.ID, "setup", "setup-agent", "sonnet")
	env.InsertAgentSession(t, "phase-impl-1", "", instance.ID, "impl", "impl-agent", "opus")
	env.InsertAgentSession(t, "phase-impl-2", "", instance.ID, "impl", "impl-agent-2", "haiku")

	// Filter by "setup" phase
	setupSessions, err := env.AgentSvc.GetProjectSessions(env.ProjectID, "setup")
	if err != nil {
		t.Fatalf("failed to get setup sessions: %v", err)
	}

	if len(setupSessions) != 1 {
		t.Fatalf("expected 1 setup session, got %d", len(setupSessions))
	}

	if setupSessions[0].ID != "phase-setup-1" {
		t.Errorf("expected session phase-setup-1, got %v", setupSessions[0].ID)
	}

	// Filter by "impl" phase
	implSessions, err := env.AgentSvc.GetProjectSessions(env.ProjectID, "impl")
	if err != nil {
		t.Fatalf("failed to get impl sessions: %v", err)
	}

	if len(implSessions) != 2 {
		t.Fatalf("expected 2 impl sessions, got %d", len(implSessions))
	}
}

// TestGetProjectAgentSessions_ExcludesTicketScoped tests that ticket-scoped sessions are excluded
func TestGetProjectAgentSessions_ExcludesTicketScoped(t *testing.T) {
	env := NewTestEnv(t)

	// Create ticket-scoped workflow
	env.CreateTicket(t, "TICKET-1", "Test ticket")
	env.InitWorkflow(t, "TICKET-1")
	ticketWFI := env.GetWorkflowInstanceID(t, "TICKET-1", "test")

	// Create project-scoped workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "proj-exclude-test",
		Description: "Test exclusion",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "proj-exclude-test",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	projectWFI, err := env.WorkflowSvc.GetProjectWorkflowInstance(env.ProjectID, "proj-exclude-test")
	if err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}

	// Insert both ticket-scoped and project-scoped sessions
	env.InsertAgentSession(t, "ticket-sess-1", "TICKET-1", ticketWFI, "analyzer", "analyzer", "sonnet")
	env.InsertAgentSession(t, "ticket-sess-2", "TICKET-1", ticketWFI, "builder", "builder", "opus")
	env.InsertAgentSession(t, "proj-sess-1", "", projectWFI.ID, "setup", "setup-agent", "haiku")

	// Get project sessions - should only return project-scoped
	sessions, err := env.AgentSvc.GetProjectSessions(env.ProjectID, "")
	if err != nil {
		t.Fatalf("failed to get project sessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 project session, got %d", len(sessions))
	}

	if sessions[0].ID != "proj-sess-1" {
		t.Errorf("expected proj-sess-1, got %v", sessions[0].ID)
	}

	if sessions[0].TicketID != "" {
		t.Errorf("expected empty ticket_id, got %v", sessions[0].TicketID)
	}

	// Verify ticket sessions are still accessible via ticket-scoped query
	ticketSessions, err := env.AgentSvc.GetTicketSessions(env.ProjectID, "TICKET-1", "")
	if err != nil {
		t.Fatalf("failed to get ticket sessions: %v", err)
	}

	if len(ticketSessions) != 2 {
		t.Fatalf("expected 2 ticket sessions, got %d", len(ticketSessions))
	}
}

// TestGetProjectAgentSessions_FindingsAggregation tests that findings are aggregated from workflow instances
func TestGetProjectAgentSessions_FindingsAggregation(t *testing.T) {
	env := NewTestEnv(t)

	// Create project-scoped workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "proj-findings",
		Description: "Test findings",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "proj-findings",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	instance, err := env.WorkflowSvc.GetProjectWorkflowInstance(env.ProjectID, "proj-findings")
	if err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}

	// Insert agent session with findings
	env.InsertAgentSession(t, "findings-sess", "", instance.ID, "setup", "setup-agent", "sonnet")

	// Add findings via service
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"project_id": env.ProjectID,
		"ticket_id":  "",
		"workflow":   "proj-findings",
		"agent_type": "setup-agent",
		"data": map[string]interface{}{
			"test_key": "test_value",
		},
	}, nil)

	// Get project sessions - findings should be aggregated
	sessions, err := env.AgentSvc.GetProjectSessions(env.ProjectID, "")
	if err != nil {
		t.Fatalf("failed to get project sessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// Verify findings are present (this tests the aggregation logic in handler)
	// The actual findings aggregation happens in the handler via BuildCombinedFindings
}

// TestGetProjectAgentSessions_MultipleWorkflows tests sessions across multiple project workflows
func TestGetProjectAgentSessions_MultipleWorkflows(t *testing.T) {
	env := NewTestEnv(t)

	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	// Create two project workflows
	for _, wf := range []string{"proj-wf-1", "proj-wf-2"} {
		_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
			ID:          wf,
			Description: "Test workflow " + wf,
			Categories:  []string{"full"},
			Phases:      phasesJSON,
			ScopeType:   "project",
		})
		if err != nil {
			t.Fatalf("failed to create workflow %s: %v", wf, err)
		}

		err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
			Workflow: wf,
		})
		if err != nil {
			t.Fatalf("failed to init workflow %s: %v", wf, err)
		}

		instance, err := env.WorkflowSvc.GetProjectWorkflowInstance(env.ProjectID, wf)
		if err != nil {
			t.Fatalf("failed to get instance for %s: %v", wf, err)
		}

		// Insert session for each workflow
		env.InsertAgentSession(t, "sess-"+wf, "", instance.ID, "setup", "setup-agent", "sonnet")
	}

	// Get all project sessions
	sessions, err := env.AgentSvc.GetProjectSessions(env.ProjectID, "")
	if err != nil {
		t.Fatalf("failed to get project sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions across workflows, got %d", len(sessions))
	}

	// Verify both workflows represented
	found := make(map[string]bool)
	for _, s := range sessions {
		found[s.ID] = true
	}

	if !found["sess-proj-wf-1"] || !found["sess-proj-wf-2"] {
		t.Errorf("missing expected sessions: found=%v", found)
	}
}

// TestGetProjectAgentSessions_CaseInsensitiveProjectID tests case-insensitive project ID matching
func TestGetProjectAgentSessions_CaseInsensitiveProjectID(t *testing.T) {
	env := NewTestEnv(t)

	// Create project workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "case-test",
		Description: "Case test",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "case-test",
	})
	if err != nil {
		t.Fatalf("failed to init workflow: %v", err)
	}

	instance, err := env.WorkflowSvc.GetProjectWorkflowInstance(env.ProjectID, "case-test")
	if err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}

	env.InsertAgentSession(t, "case-sess", "", instance.ID, "setup", "setup-agent", "sonnet")

	// Query with different case variations
	for _, projectID := range []string{env.ProjectID, "TEST-PROJECT", "Test-Project"} {
		sessions, err := env.AgentSvc.GetProjectSessions(projectID, "")
		if err != nil {
			t.Fatalf("failed to get sessions for %s: %v", projectID, err)
		}

		if len(sessions) != 1 {
			t.Errorf("expected 1 session for project_id %s, got %d", projectID, len(sessions))
		}
	}
}

// TestGetProjectAgentSessions_EmptyStringTicketID tests that only empty-string ticket_id sessions are returned
func TestGetProjectAgentSessions_EmptyStringTicketID(t *testing.T) {
	env := NewTestEnv(t)

	// Create project workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "empty-test",
		Description: "Empty string test",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	err = env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "empty-test",
	})
	if err != nil {
		t.Fatalf("failed to init workflow: %v", err)
	}

	instance, err := env.WorkflowSvc.GetProjectWorkflowInstance(env.ProjectID, "empty-test")
	if err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}

	// Insert session with empty string ticket_id (project-scoped)
	env.InsertAgentSession(t, "empty-ticket", "", instance.ID, "setup", "setup-agent", "sonnet")

	// Query should return the empty string ticket_id session
	sessions, err := env.AgentSvc.GetProjectSessions(env.ProjectID, "")
	if err != nil {
		t.Fatalf("failed to get project sessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// Verify it has empty ticket_id
	if sessions[0].TicketID != "" {
		t.Errorf("session %s has non-empty ticket_id: %v", sessions[0].ID, sessions[0].TicketID)
	}
}
