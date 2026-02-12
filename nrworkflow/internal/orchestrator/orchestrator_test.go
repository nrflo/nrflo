package orchestrator

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"nrworkflow/internal/db"
	"nrworkflow/internal/model"
	"nrworkflow/internal/repo"
	"nrworkflow/internal/service"
	"nrworkflow/internal/types"
	"nrworkflow/internal/ws"
)

// testEnv holds test infrastructure for orchestrator tests.
type testEnv struct {
	pool    *db.Pool
	hub     *ws.Hub
	orch    *Orchestrator
	dbPath  string
	project string
}

// newTestEnv creates an isolated test environment with a fresh DB and WS hub.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	hub := ws.NewHub()
	go hub.Run()

	orch := New(dbPath, hub)

	projectID := "test-project"

	// Seed project
	projectSvc := service.NewProjectService(pool)
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:     "Test Project",
		RootPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	// Seed test workflow definition
	workflowSvc := service.NewWorkflowService(pool)
	phasesJSON, _ := json.Marshal([]string{"analyzer", "builder"})
	_, err = workflowSvc.CreateWorkflowDef(projectID, &types.WorkflowDefCreateRequest{
		ID:          "test",
		Description: "Test workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
	})
	if err != nil {
		t.Fatalf("failed to seed workflow: %v", err)
	}

	t.Cleanup(func() {
		hub.Stop()
		pool.Close()
	})

	return &testEnv{
		pool:    pool,
		hub:     hub,
		orch:    orch,
		dbPath:  dbPath,
		project: projectID,
	}
}

// createTicket creates a ticket in the test DB.
func (e *testEnv) createTicket(t *testing.T, ticketID, title string) {
	t.Helper()
	ticketSvc := service.NewTicketService(e.pool)
	_, err := ticketSvc.Create(e.project, &types.TicketCreateRequest{
		ID:    ticketID,
		Title: title,
	})
	if err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}
}

// initWorkflow initializes the "test" workflow on a ticket and returns the instance ID.
func (e *testEnv) initWorkflow(t *testing.T, ticketID string) string {
	t.Helper()
	workflowSvc := service.NewWorkflowService(e.pool)
	err := workflowSvc.Init(e.project, ticketID, &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to init workflow: %v", err)
	}

	var id string
	err = e.pool.QueryRow(`
		SELECT id FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		e.project, ticketID, "test").Scan(&id)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}
	return id
}

// getTicket retrieves a ticket from the DB.
func (e *testEnv) getTicket(t *testing.T, ticketID string) *model.Ticket {
	t.Helper()
	ticketSvc := service.NewTicketService(e.pool)
	ticket, err := ticketSvc.Get(e.project, ticketID)
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}
	return ticket
}

// getWorkflowInstance retrieves a workflow instance from the DB.
func (e *testEnv) getWorkflowInstance(t *testing.T, wfiID string) *model.WorkflowInstance {
	t.Helper()
	wfiRepo := repo.NewWorkflowInstanceRepo(e.pool)
	wi, err := wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	return wi
}

// subscribeWSClient creates a test WS client subscribed to the given ticket.
func (e *testEnv) subscribeWSClient(t *testing.T, id, ticketID string) chan []byte {
	t.Helper()
	client, ch := ws.NewTestClient(e.hub, id)
	e.hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	e.hub.Subscribe(client, e.project, ticketID)
	time.Sleep(50 * time.Millisecond)
	return ch
}

// expectEvent waits for a specific WS event type on the channel.
func expectEvent(t *testing.T, ch chan []byte, eventType string, timeout time.Duration) ws.Event {
	t.Helper()
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				t.Fatalf("failed to unmarshal event: %v", err)
			}
			if event.Type == eventType {
				return event
			}
		case <-time.After(timeout):
			t.Fatalf("timeout waiting for event type '%s'", eventType)
		}
	}
}

func TestMarkCompletedClosesTicket(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MC-1", "Close on complete")
	wfiID := env.initWorkflow(t, "MC-1")

	// Verify ticket starts open
	ticket := env.getTicket(t, "MC-1")
	if ticket.Status != model.StatusOpen {
		t.Fatalf("expected ticket status 'open', got %v", ticket.Status)
	}

	// Call markCompleted
	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "MC-1",
		WorkflowName: "test",
	})

	// Verify ticket is now closed
	ticket = env.getTicket(t, "MC-1")
	if ticket.Status != model.StatusClosed {
		t.Fatalf("expected ticket status 'closed', got %v", ticket.Status)
	}

	// Verify close_reason is set
	if !ticket.CloseReason.Valid {
		t.Fatal("expected close_reason to be set")
	}
	expectedReason := "Workflow 'test' completed successfully"
	if ticket.CloseReason.String != expectedReason {
		t.Fatalf("expected close_reason %q, got %q", expectedReason, ticket.CloseReason.String)
	}

	// Verify closed_at is set
	if !ticket.ClosedAt.Valid {
		t.Fatal("expected closed_at to be set")
	}
}

func TestMarkCompletedUpdatesWorkflowStatus(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MC-2", "Status update")
	wfiID := env.initWorkflow(t, "MC-2")

	// Verify starts active
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceActive {
		t.Fatalf("expected workflow status 'active', got %v", wi.Status)
	}

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "MC-2",
		WorkflowName: "test",
	})

	// Verify workflow instance status is completed
	wi = env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceCompleted {
		t.Fatalf("expected workflow status 'completed', got %v", wi.Status)
	}
}

func TestMarkCompletedBroadcastsEvent(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MC-3", "WS event")
	wfiID := env.initWorkflow(t, "MC-3")

	// Subscribe WS client
	ch := env.subscribeWSClient(t, "ws-mc3", "MC-3")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "MC-3",
		WorkflowName: "test",
	})

	// Expect orchestration.completed event
	event := expectEvent(t, ch, ws.EventOrchestrationCompleted, 2*time.Second)
	if event.TicketID != "MC-3" {
		t.Fatalf("expected ticket_id 'MC-3', got %v", event.TicketID)
	}
	if event.Data["instance_id"] != wfiID {
		t.Fatalf("expected instance_id %q, got %v", wfiID, event.Data["instance_id"])
	}
}

func TestMarkCompletedTicketCloseFailureDoesNotBreakWorkflow(t *testing.T) {
	env := newTestEnv(t)

	// Create ticket and workflow, but use a wrong ticket ID in the RunRequest
	// so that the ticket close call fails (ticket not found).
	env.createTicket(t, "MC-4", "Real ticket")
	wfiID := env.initWorkflow(t, "MC-4")

	// Subscribe WS client to the non-existent ticket ID we'll pass
	ch := env.subscribeWSClient(t, "ws-mc4", "NONEXISTENT")

	// Call markCompleted with a ticket ID that doesn't exist in DB.
	// The ticket close will fail but workflow should still be marked completed.
	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "NONEXISTENT",
		WorkflowName: "test",
	})

	// Workflow instance should still be completed
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceCompleted {
		t.Fatalf("expected workflow status 'completed', got %v", wi.Status)
	}

	// WS event should still be broadcast
	event := expectEvent(t, ch, ws.EventOrchestrationCompleted, 2*time.Second)
	if event.Data["instance_id"] != wfiID {
		t.Fatalf("expected instance_id %q, got %v", wfiID, event.Data["instance_id"])
	}
}

func TestMarkCompletedAlreadyClosedTicket(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MC-5", "Already closed")
	wfiID := env.initWorkflow(t, "MC-5")

	// Close the ticket first
	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.Close(env.project, "MC-5", "manually closed")
	if err != nil {
		t.Fatalf("failed to pre-close ticket: %v", err)
	}

	// markCompleted should not fail - the ticket close is a no-op UPDATE (already closed)
	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "MC-5",
		WorkflowName: "test",
	})

	// Verify ticket is still closed
	ticket := env.getTicket(t, "MC-5")
	if ticket.Status != model.StatusClosed {
		t.Fatalf("expected ticket status 'closed', got %v", ticket.Status)
	}

	// The close reason should be updated to the new one (TicketService.Close does an UPDATE)
	expectedReason := "Workflow 'test' completed successfully"
	if !ticket.CloseReason.Valid || ticket.CloseReason.String != expectedReason {
		t.Fatalf("expected close_reason %q, got %v", expectedReason, ticket.CloseReason)
	}

	// Workflow instance should still be completed
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceCompleted {
		t.Fatalf("expected workflow status 'completed', got %v", wi.Status)
	}
}

func TestMarkCompletedUpdatesOrchestrationFindings(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MC-6", "Findings update")
	wfiID := env.initWorkflow(t, "MC-6")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "MC-6",
		WorkflowName: "test",
	})

	// Verify _orchestration findings are set to "completed"
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()

	orch, ok := findings["_orchestration"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected _orchestration in findings, got %v", findings)
	}
	if orch["status"] != "completed" {
		t.Fatalf("expected _orchestration status 'completed', got %v", orch["status"])
	}
}

func TestMarkFailedDoesNotCloseTicket(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MF-1", "Should stay open")
	wfiID := env.initWorkflow(t, "MF-1")

	env.orch.markFailed(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "MF-1",
		WorkflowName: "test",
	}, "phase analyzer failed")

	// Ticket should remain open
	ticket := env.getTicket(t, "MF-1")
	if ticket.Status != model.StatusOpen {
		t.Fatalf("expected ticket status 'open' after failure, got %v", ticket.Status)
	}
	if ticket.ClosedAt.Valid {
		t.Fatal("expected closed_at to be NULL after failure")
	}

	// Workflow instance should be failed
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceFailed {
		t.Fatalf("expected workflow status 'failed', got %v", wi.Status)
	}
}

func TestShouldSkipPhase(t *testing.T) {
	tests := []struct {
		name     string
		category string
		skipFor  []string
		want     bool
	}{
		{"empty category", "", []string{"simple"}, false},
		{"empty skipFor", "simple", nil, false},
		{"both empty", "", nil, false},
		{"matching category", "simple", []string{"simple", "docs"}, true},
		{"non-matching category", "full", []string{"simple", "docs"}, false},
		{"single match", "docs", []string{"docs"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipPhase(tt.category, tt.skipFor)
			if got != tt.want {
				t.Errorf("shouldSkipPhase(%q, %v) = %v, want %v",
					tt.category, tt.skipFor, got, tt.want)
			}
		})
	}
}

// Verify user instructions are stored as a direct string, not a nested map.
func TestUserInstructionsStoredAsDirectString(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "UI-1", "User instructions test")
	wfiID := env.initWorkflow(t, "UI-1")

	// Simulate what Start() does: store instructions as direct string
	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool)
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
	findings["user_instructions"] = "Fix the login bug"
	findingsJSON, _ := json.Marshal(findings)
	err := wfiRepo.UpdateFindings(wfiID, string(findingsJSON))
	if err != nil {
		t.Fatalf("failed to update findings: %v", err)
	}

	// Read back and verify it's a direct string, not a nested map
	wi = env.getWorkflowInstance(t, wfiID)
	readFindings := wi.GetFindings()

	instructions, ok := readFindings["user_instructions"]
	if !ok {
		t.Fatal("expected user_instructions in findings")
	}

	str, ok := instructions.(string)
	if !ok {
		t.Fatalf("expected user_instructions to be a string, got %T: %v", instructions, instructions)
	}
	if str != "Fix the login bug" {
		t.Fatalf("expected 'Fix the login bug', got %q", str)
	}
}

// Verify empty instructions are not stored in findings.
func TestEmptyInstructionsNotStored(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "UI-2", "Empty instructions test")
	wfiID := env.initWorkflow(t, "UI-2")

	// With empty instructions, Start() doesn't store anything (if req.Instructions != "")
	// Verify findings don't have user_instructions
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()

	if _, exists := findings["user_instructions"]; exists {
		t.Fatal("expected no user_instructions in findings for empty instructions")
	}
}

// Verify the close reason includes the workflow name.
func TestMarkCompletedCloseReasonIncludesWorkflowName(t *testing.T) {
	env := newTestEnv(t)

	// Create a second workflow definition for this test
	workflowSvc := service.NewWorkflowService(env.pool)
	phasesJSON, _ := json.Marshal([]string{"analyzer"})
	_, err := workflowSvc.CreateWorkflowDef(env.project, &types.WorkflowDefCreateRequest{
		ID:          "feature",
		Description: "Feature workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	env.createTicket(t, "MC-7", "Workflow name in reason")

	// Init the "feature" workflow
	err = workflowSvc.Init(env.project, "MC-7", &types.WorkflowInitRequest{
		Workflow: "feature",
	})
	if err != nil {
		t.Fatalf("failed to init feature workflow: %v", err)
	}

	var wfiID string
	err = env.pool.QueryRow(`
		SELECT id FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "MC-7", "feature").Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "MC-7",
		WorkflowName: "feature",
	})

	ticket := env.getTicket(t, "MC-7")
	expectedReason := "Workflow 'feature' completed successfully"
	if !ticket.CloseReason.Valid || ticket.CloseReason.String != expectedReason {
		t.Fatalf("expected close_reason %q, got %v", expectedReason, ticket.CloseReason)
	}
}

// --- SetInProgress tests ---

func TestSetInProgressOnOpenTicket(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "SIP-1", "Open ticket")

	// Verify ticket starts open
	ticket := env.getTicket(t, "SIP-1")
	if ticket.Status != model.StatusOpen {
		t.Fatalf("expected status 'open', got %v", ticket.Status)
	}

	// Call SetInProgress
	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.SetInProgress(env.project, "SIP-1")
	if err != nil {
		t.Fatalf("SetInProgress failed: %v", err)
	}

	// Verify status changed
	ticket = env.getTicket(t, "SIP-1")
	if ticket.Status != model.StatusInProgress {
		t.Fatalf("expected status 'in_progress', got %v", ticket.Status)
	}
}

func TestSetInProgressOnClosedTicket(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "SIP-2", "Closed ticket")

	// Close the ticket
	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.Close(env.project, "SIP-2", "done")
	if err != nil {
		t.Fatalf("failed to close ticket: %v", err)
	}

	// Call SetInProgress — should be a no-op
	err = ticketSvc.SetInProgress(env.project, "SIP-2")
	if err != nil {
		t.Fatalf("SetInProgress returned error: %v", err)
	}

	// Verify status stays closed
	ticket := env.getTicket(t, "SIP-2")
	if ticket.Status != model.StatusClosed {
		t.Fatalf("expected status 'closed', got %v", ticket.Status)
	}
}

func TestSetInProgressOnAlreadyInProgressTicket(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "SIP-3", "Already in progress")

	// Set to in_progress first time
	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.SetInProgress(env.project, "SIP-3")
	if err != nil {
		t.Fatalf("first SetInProgress failed: %v", err)
	}

	// Call again — should be a no-op (status is in_progress, not open)
	err = ticketSvc.SetInProgress(env.project, "SIP-3")
	if err != nil {
		t.Fatalf("second SetInProgress returned error: %v", err)
	}

	// Verify status stays in_progress
	ticket := env.getTicket(t, "SIP-3")
	if ticket.Status != model.StatusInProgress {
		t.Fatalf("expected status 'in_progress', got %v", ticket.Status)
	}
}

func TestSetInProgressUpdatesTimestamp(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "SIP-4", "Timestamp check")

	// Record original updated_at
	before := env.getTicket(t, "SIP-4")
	// RFC3339 has second precision, so we need to wait > 1s
	time.Sleep(1100 * time.Millisecond)

	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.SetInProgress(env.project, "SIP-4")
	if err != nil {
		t.Fatalf("SetInProgress failed: %v", err)
	}

	after := env.getTicket(t, "SIP-4")
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("expected updated_at to advance, before=%v after=%v", before.UpdatedAt, after.UpdatedAt)
	}
}

// --- Orchestrator Start sets ticket to in_progress ---

func TestStartSetsTicketToInProgress(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "START-1", "Start workflow test")

	// Verify ticket starts open
	ticket := env.getTicket(t, "START-1")
	if ticket.Status != model.StatusOpen {
		t.Fatalf("expected status 'open', got %v", ticket.Status)
	}

	// We can't call Start() fully (it needs spawner infra), but we can
	// simulate the SetInProgress call that Start() makes. The orchestrator
	// opens a new pool and calls SetInProgress. Replicate that path.
	pool, err := db.NewPool(env.dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	ticketSvc := service.NewTicketService(pool)
	err = ticketSvc.SetInProgress(env.project, "START-1")
	pool.Close()
	if err != nil {
		t.Fatalf("SetInProgress failed: %v", err)
	}

	// Verify ticket is now in_progress
	ticket = env.getTicket(t, "START-1")
	if ticket.Status != model.StatusInProgress {
		t.Fatalf("expected status 'in_progress', got %v", ticket.Status)
	}
}

func TestStartDoesNotChangeClosedTicketStatus(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "START-2", "Closed before start")

	// Close the ticket
	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.Close(env.project, "START-2", "pre-closed")
	if err != nil {
		t.Fatalf("failed to close ticket: %v", err)
	}

	// Simulate the SetInProgress call from Start()
	err = ticketSvc.SetInProgress(env.project, "START-2")
	if err != nil {
		t.Fatalf("SetInProgress returned error: %v", err)
	}

	// Status should remain closed
	ticket := env.getTicket(t, "START-2")
	if ticket.Status != model.StatusClosed {
		t.Fatalf("expected status 'closed', got %v", ticket.Status)
	}
}

func TestMarkFailedKeepsInProgressStatus(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MF-2", "In progress before fail")
	wfiID := env.initWorkflow(t, "MF-2")

	// Set ticket to in_progress (simulating what Start does)
	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.SetInProgress(env.project, "MF-2")
	if err != nil {
		t.Fatalf("SetInProgress failed: %v", err)
	}

	// markFailed should NOT change ticket status
	env.orch.markFailed(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "MF-2",
		WorkflowName: "test",
	}, "phase failed")

	// Ticket should remain in_progress (not closed, not reverted to open)
	ticket := env.getTicket(t, "MF-2")
	if ticket.Status != model.StatusInProgress {
		t.Fatalf("expected status 'in_progress' after failure, got %v", ticket.Status)
	}
}
