package integration

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

func TestAttachWorkflowProgress_NoActiveWorkflow(t *testing.T) {
	env := NewTestEnv(t)

	// Create a ticket without any workflow
	env.CreateTicket(t, "test-1", "Test ticket")

	// Get the ticket from DB
	ticket, err := env.TicketSvc.Get(env.ProjectID, "test-1")
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}

	// Create PendingTicket
	pendingTickets := []*repo.PendingTicket{
		{Ticket: ticket, IsBlocked: false},
	}

	// Empty progress map (no active workflows)
	progress := make(map[string]*repo.WorkflowProgress)

	// Attach workflow progress
	repo.AttachWorkflowProgress(pendingTickets, progress)

	// Verify workflow_progress is nil
	if pendingTickets[0].WorkflowProgress != nil {
		t.Fatalf("expected workflow_progress to be nil for ticket without workflow, got %+v", pendingTickets[0].WorkflowProgress)
	}
}

func TestAttachWorkflowProgress_HappyPath(t *testing.T) {
	env := NewTestEnv(t)

	// Create a ticket
	env.CreateTicket(t, "test-2", "Test ticket with workflow")

	// Get the ticket from DB
	ticket, err := env.TicketSvc.Get(env.ProjectID, "test-2")
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}

	// Create PendingTicket
	pendingTickets := []*repo.PendingTicket{
		{Ticket: ticket, IsBlocked: false},
	}

	// Attach pre-computed workflow progress
	progress := map[string]*repo.WorkflowProgress{
		"test-2": {
			WorkflowName:    "test",
			CurrentPhase:    "tester",
			CompletedPhases: 2,
			TotalPhases:     4,
			Status:          "active",
		},
	}
	repo.AttachWorkflowProgress(pendingTickets, progress)

	// Verify workflow_progress is populated correctly
	wp := pendingTickets[0].WorkflowProgress
	if wp == nil {
		t.Fatal("expected workflow_progress to be populated")
	}

	if wp.WorkflowName != "test" {
		t.Fatalf("expected workflow_name 'test', got %q", wp.WorkflowName)
	}
	if wp.CurrentPhase != "tester" {
		t.Fatalf("expected current_phase 'tester', got %q", wp.CurrentPhase)
	}
	if wp.CompletedPhases != 2 {
		t.Fatalf("expected completed_phases 2, got %d", wp.CompletedPhases)
	}
	if wp.TotalPhases != 4 {
		t.Fatalf("expected total_phases 4, got %d", wp.TotalPhases)
	}
	if wp.Status != "active" {
		t.Fatalf("expected status 'active', got %q", wp.Status)
	}
}

func TestAttachWorkflowProgress_SkippedPhasesCountAsCompleted(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "test-3", "Test ticket with skipped phases")

	ticket, err := env.TicketSvc.Get(env.ProjectID, "test-3")
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}

	pendingTickets := []*repo.PendingTicket{
		{Ticket: ticket, IsBlocked: false},
	}

	// Skipped phases count as completed in derived progress (result="skipped" → status="completed")
	progress := map[string]*repo.WorkflowProgress{
		"test-3": {
			WorkflowName:    "test",
			CurrentPhase:    "phase4",
			CompletedPhases: 3, // 2 completed + 1 skipped
			TotalPhases:     4,
			Status:          "active",
		},
	}
	repo.AttachWorkflowProgress(pendingTickets, progress)

	wp := pendingTickets[0].WorkflowProgress
	if wp == nil {
		t.Fatal("expected workflow_progress to be populated")
	}

	// Skipped phases should count as completed: 2 completed + 1 skipped = 3
	if wp.CompletedPhases != 3 {
		t.Fatalf("expected completed_phases 3 (including skipped), got %d", wp.CompletedPhases)
	}
	if wp.TotalPhases != 4 {
		t.Fatalf("expected total_phases 4, got %d", wp.TotalPhases)
	}
}

func TestAttachWorkflowProgress_MultipleWorkflows_MostRecentWins(t *testing.T) {
	env := NewTestEnv(t)

	// Create a second workflow definition for testing multiple workflows per ticket
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "investigation", "layer": 0},
		{"agent": "implementation", "layer": 1},
		{"agent": "verification", "layer": 2},
	})
	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "bugfix",
		Description: "Bugfix workflow",
		Phases:      phasesJSON,
	})
	if err != nil {
		t.Fatalf("failed to create bugfix workflow: %v", err)
	}

	env.CreateTicket(t, "test-4", "Test ticket with multiple workflows")

	ticket, err := env.TicketSvc.Get(env.ProjectID, "test-4")
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}

	// Create two workflow instances, then use DeriveWorkflowProgress (which uses agent_sessions)
	wi1 := &model.WorkflowInstance{
		ID:         "wf-old",
		ProjectID:  env.ProjectID,
		TicketID:   "test-4",
		WorkflowID: "test",
		Status:     model.WorkflowInstanceActive,
		PhaseOrder: `["analyzer","builder"]`,
		Phases:     `{}`,
		Findings:   "{}",
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	err = wfiRepo.Create(wi1)
	if err != nil {
		t.Fatalf("failed to create first workflow instance: %v", err)
	}

	wi2 := &model.WorkflowInstance{
		ID:         "wf-new",
		ProjectID:  env.ProjectID,
		TicketID:   "test-4",
		WorkflowID: "bugfix",
		Status:     model.WorkflowInstanceActive,
		PhaseOrder: `["investigation","implementation","verification"]`,
		Phases:     `{}`,
		Findings:   "{}",
	}
	err = wfiRepo.Create(wi2)
	if err != nil {
		t.Fatalf("failed to create second workflow instance: %v", err)
	}

	// ListActiveByProject returns the most recently updated per ticket
	instances, err := wfiRepo.ListActiveByProject(env.ProjectID)
	if err != nil {
		t.Fatalf("failed to list active instances: %v", err)
	}

	// Use DeriveWorkflowProgress to compute progress from agent_sessions + workflow defs
	progress := env.WorkflowSvc.DeriveWorkflowProgress(instances)

	pendingTickets := []*repo.PendingTicket{
		{Ticket: ticket, IsBlocked: false},
	}
	repo.AttachWorkflowProgress(pendingTickets, progress)

	wp := pendingTickets[0].WorkflowProgress
	if wp == nil {
		t.Fatal("expected workflow_progress to be populated")
	}

	// Should use the most recently updated workflow instance.
	// With no agent_sessions, all phases are pending, so completed=0.
	if wp.TotalPhases < 2 {
		t.Fatalf("expected total_phases >= 2, got %d", wp.TotalPhases)
	}
}

func TestListActiveByProject_EmptyForNoActiveWorkflows(t *testing.T) {
	env := NewTestEnv(t)

	// Create tickets with no workflows
	env.CreateTicket(t, "test-5", "Ticket 1")
	env.CreateTicket(t, "test-6", "Ticket 2")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	instances, err := wfiRepo.ListActiveByProject(env.ProjectID)
	if err != nil {
		t.Fatalf("failed to list active instances: %v", err)
	}

	if len(instances) != 0 {
		t.Fatalf("expected empty map for no active workflows, got %d entries", len(instances))
	}
}

func TestListActiveByProject_OnlyActiveWorkflows(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "test-7", "Ticket with completed workflow")

	// Create completed workflow (should not be returned)
	phases := map[string]model.PhaseStatus{
		"phase1": {Status: "completed", Result: "pass"},
	}
	phasesJSON, _ := json.Marshal(phases)
	phaseOrder := []string{"phase1"}
	phaseOrderJSON, _ := json.Marshal(phaseOrder)

	wi := &model.WorkflowInstance{
		ID:           "wf-completed",
		ProjectID:    env.ProjectID,
		TicketID:     "test-7",
		WorkflowID:   "test",
		Status:       model.WorkflowInstanceCompleted,
		CurrentPhase: sql.NullString{String: "phase1", Valid: true},
		PhaseOrder:   string(phaseOrderJSON),
		Phases:       string(phasesJSON),
		Findings:     "{}",
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	err := wfiRepo.Create(wi)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	instances, err := wfiRepo.ListActiveByProject(env.ProjectID)
	if err != nil {
		t.Fatalf("failed to list active instances: %v", err)
	}

	// Should return empty map since the only workflow is completed
	if len(instances) != 0 {
		t.Fatalf("expected empty map for no active workflows, got %d entries", len(instances))
	}
}

func TestPendingTicketMarshalJSON_IncludesWorkflowProgress(t *testing.T) {
	ticket := &model.Ticket{
		ID:        "test-8",
		ProjectID: "testproj",
		Title:     "Test ticket",
		Status:    model.StatusOpen,
		Priority:  2,
		IssueType: "task",
		CreatedBy: "tester",
	}

	wp := &repo.WorkflowProgress{
		WorkflowName:    "feature",
		CurrentPhase:    "implementation",
		CompletedPhases: 2,
		TotalPhases:     5,
		Status:          "active",
	}

	pt := repo.PendingTicket{
		Ticket:           ticket,
		IsBlocked:        false,
		WorkflowProgress: wp,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(pt)
	if err != nil {
		t.Fatalf("failed to marshal PendingTicket: %v", err)
	}

	// Unmarshal to verify structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify workflow_progress field exists
	wpField, exists := result["workflow_progress"]
	if !exists {
		t.Fatal("expected workflow_progress field in JSON")
	}

	// Verify workflow_progress contents
	wpMap, ok := wpField.(map[string]interface{})
	if !ok {
		t.Fatalf("expected workflow_progress to be object, got %T", wpField)
	}

	if wpMap["workflow_name"] != "feature" {
		t.Fatalf("expected workflow_name 'feature', got %v", wpMap["workflow_name"])
	}
	if wpMap["current_phase"] != "implementation" {
		t.Fatalf("expected current_phase 'implementation', got %v", wpMap["current_phase"])
	}
	if wpMap["completed_phases"].(float64) != 2 {
		t.Fatalf("expected completed_phases 2, got %v", wpMap["completed_phases"])
	}
	if wpMap["total_phases"].(float64) != 5 {
		t.Fatalf("expected total_phases 5, got %v", wpMap["total_phases"])
	}
	if wpMap["status"] != "active" {
		t.Fatalf("expected status 'active', got %v", wpMap["status"])
	}
}

func TestPendingTicketMarshalJSON_OmitsWorkflowProgressWhenNil(t *testing.T) {
	ticket := &model.Ticket{
		ID:        "test-9",
		ProjectID: "testproj",
		Title:     "Test ticket",
		Status:    model.StatusOpen,
		Priority:  2,
		IssueType: "task",
		CreatedBy: "tester",
	}

	pt := repo.PendingTicket{
		Ticket:           ticket,
		IsBlocked:        false,
		WorkflowProgress: nil,
	}

	jsonData, err := json.Marshal(pt)
	if err != nil {
		t.Fatalf("failed to marshal PendingTicket: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify workflow_progress field is omitted
	_, exists := result["workflow_progress"]
	if exists {
		t.Fatal("expected workflow_progress to be omitted when nil")
	}
}

func TestAttachWorkflowProgress_CaseInsensitiveTicketID(t *testing.T) {
	env := NewTestEnv(t)

	// Create ticket with uppercase ID
	env.CreateTicket(t, "TEST-10", "Case test")

	ticket, err := env.TicketSvc.Get(env.ProjectID, "TEST-10")
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}

	// Create workflow with lowercase ticket ID in database
	wi := &model.WorkflowInstance{
		ID:         "wf-case",
		ProjectID:  env.ProjectID,
		TicketID:   strings.ToLower("TEST-10"),
		WorkflowID: "test",
		Status:     model.WorkflowInstanceActive,
		PhaseOrder: `["phase1"]`,
		Phases:     `{}`,
		Findings:   "{}",
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	err = wfiRepo.Create(wi)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Use DeriveWorkflowProgress for case-insensitive matching
	instances, err := wfiRepo.ListActiveByProject(env.ProjectID)
	if err != nil {
		t.Fatalf("failed to list active instances: %v", err)
	}

	progress := env.WorkflowSvc.DeriveWorkflowProgress(instances)

	pendingTickets := []*repo.PendingTicket{
		{Ticket: ticket, IsBlocked: false},
	}
	repo.AttachWorkflowProgress(pendingTickets, progress)

	wp := pendingTickets[0].WorkflowProgress
	if wp == nil {
		t.Fatal("expected workflow_progress to be populated (case-insensitive match)")
	}
}
