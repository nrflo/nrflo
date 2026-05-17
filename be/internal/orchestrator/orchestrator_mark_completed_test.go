package orchestrator

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

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
		ProjectID:             env.project,
		TicketID:              "MC-1",
		WorkflowName:          "test",
		CloseTicketOnComplete: true,
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

	env.createTicket(t, "MC-4", "Real ticket")
	wfiID := env.initWorkflow(t, "MC-4")

	ch := env.subscribeWSClient(t, "ws-mc4", "NONEXISTENT")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "NONEXISTENT",
		WorkflowName:          "test",
		CloseTicketOnComplete: true,
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

	// Close the ticket first with an explicit reason
	ticketSvc := service.NewTicketService(env.pool, clock.Real())
	err := ticketSvc.Close(env.project, "MC-5", "manually closed")
	if err != nil {
		t.Fatalf("failed to pre-close ticket: %v", err)
	}

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "MC-5",
		WorkflowName:          "test",
		CloseTicketOnComplete: true,
	})

	// Verify ticket is still closed and original close_reason is preserved
	// (markCompleted must skip Close() when ticket is already closed)
	ticket := env.getTicket(t, "MC-5")
	if ticket.Status != model.StatusClosed {
		t.Fatalf("expected ticket status 'closed', got %v", ticket.Status)
	}
	if !ticket.CloseReason.Valid || ticket.CloseReason.String != "manually closed" {
		t.Errorf("expected original close_reason %q preserved, got %v", "manually closed", ticket.CloseReason)
	}

	// Workflow instance should still be completed
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceCompleted {
		t.Fatalf("expected workflow status 'completed', got %v", wi.Status)
	}
}

// TestMarkCompleted_TicketAlreadyClosed_PreservesCloseMetadata is a regression
// test verifying that markCompleted does not clobber closed_at or close_reason
// when the ticket was already closed before workflow completion.
func TestMarkCompleted_TicketAlreadyClosed_PreservesCloseMetadata(t *testing.T) {
	env := newTestEnv(t)

	// Seed the ticket via direct INSERT with explicit closed_at and close_reason
	// so we can assert the exact values are preserved after markCompleted runs.
	originalClosedAt := "2024-01-15T10:00:00Z"
	originalReason := "closed by another process"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.pool.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority,
			closed_at, close_reason, created_at, updated_at, created_by)
		VALUES (?, ?, 'Pre-closed ticket', 'closed', 'task', 2, ?, ?, ?, ?, 'tester')`,
		"mc-pre-closed", env.project, originalClosedAt, originalReason, now, now)
	if err != nil {
		t.Fatalf("failed to seed pre-closed ticket: %v", err)
	}

	wfiID := env.initWorkflow(t, "mc-pre-closed")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "mc-pre-closed",
		WorkflowName:          "test",
		CloseTicketOnComplete: true,
	})

	// Read directly from DB to assert closed_at and close_reason are unchanged
	var gotClosedAt, gotCloseReason string
	err = env.pool.QueryRow(`
		SELECT COALESCE(closed_at, ''), COALESCE(close_reason, '')
		FROM tickets WHERE LOWER(id) = LOWER(?) AND LOWER(project_id) = LOWER(?)`,
		"mc-pre-closed", env.project).Scan(&gotClosedAt, &gotCloseReason)
	if err != nil {
		t.Fatalf("failed to read ticket from DB: %v", err)
	}

	if gotClosedAt != originalClosedAt {
		t.Errorf("closed_at = %q, want original %q (must not be clobbered)", gotClosedAt, originalClosedAt)
	}
	if gotCloseReason != originalReason {
		t.Errorf("close_reason = %q, want original %q (must not be clobbered)", gotCloseReason, originalReason)
	}

	// Workflow instance should be completed
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceCompleted {
		t.Errorf("expected workflow status 'completed', got %v", wi.Status)
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
	findings := getWFIFindings(t, env, wfiID)

	orch, ok := findings["_orchestration"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected _orchestration in findings, got %v", findings)
	}
	if orch["status"] != "completed" {
		t.Fatalf("expected _orchestration status 'completed', got %v", orch["status"])
	}
}

func TestMarkCompletedCloseReasonIncludesWorkflowName(t *testing.T) {
	env := newTestEnv(t)

	// Create a second workflow definition for this test
	workflowSvc := service.NewWorkflowService(env.pool, clock.Real())
	_, err := workflowSvc.CreateWorkflowDef(env.project, &types.WorkflowDefCreateRequest{
		ID:          "feature",
		Description: "Feature workflow",
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	// Create agent definition with layer info
	now := clock.Real().Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	_, err = env.pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, created_at, updated_at)
		VALUES (?, ?, ?, 'sonnet', 20, 'test prompt', 0, ?, ?)`,
		"analyzer", env.project, "feature", now, now)
	if err != nil {
		t.Fatalf("failed to create agent definition: %v", err)
	}

	env.createTicket(t, "MC-7", "Workflow name in reason")

	// Init the "feature" workflow
	_, err = workflowSvc.Init(env.project, "MC-7", &types.WorkflowInitRequest{
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
		ProjectID:             env.project,
		TicketID:              "MC-7",
		WorkflowName:          "feature",
		CloseTicketOnComplete: true,
	})

	ticket := env.getTicket(t, "MC-7")
	expectedReason := "Workflow 'feature' completed successfully"
	if !ticket.CloseReason.Valid || ticket.CloseReason.String != expectedReason {
		t.Fatalf("expected close_reason %q, got %v", expectedReason, ticket.CloseReason)
	}
}

func TestMarkCompletedProjectScopeUsesProjectCompleted(t *testing.T) {
	env := newTestEnv(t)

	wfiID := env.initProjectWorkflow(t, "test")

	// Verify starts active
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceActive {
		t.Fatalf("expected workflow status 'active', got %v", wi.Status)
	}

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ScopeType:    "project",
	})

	// Verify workflow instance status is project_completed
	wi = env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceProjectCompleted {
		t.Fatalf("expected workflow status 'project_completed', got %v", wi.Status)
	}
}

func TestMarkCompletedProjectScopeUpdatesAgentSessions(t *testing.T) {
	env := newTestEnv(t)

	wfiID := env.initProjectWorkflow(t, "test")

	// Insert agent sessions with various statuses
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sessions := []struct {
		id     string
		status model.AgentSessionStatus
	}{
		{"session-completed", model.AgentSessionCompleted},
		{"session-failed", model.AgentSessionFailed},
		{"session-timeout", model.AgentSessionTimeout},
	}

	for _, s := range sessions {
		_, err = database.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, created_at, updated_at)
			VALUES (?, ?, '', ?, 'test-phase', 'test-agent', ?, ?, ?)`,
			s.id, env.project, wfiID, s.status, now, now)
		if err != nil {
			t.Fatalf("failed to insert session %s: %v", s.id, err)
		}
	}

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ScopeType:    "project",
	})

	// Verify all sessions are now project_completed
	asRepo := repo.NewAgentSessionRepo(database, clock.Real())
	for _, s := range sessions {
		session, err := asRepo.Get(s.id)
		if err != nil {
			t.Fatalf("failed to get session %s: %v", s.id, err)
		}
		if session.Status != model.AgentSessionProjectCompleted {
			t.Fatalf("expected session %s status 'project_completed', got %v", s.id, session.Status)
		}
	}
}

func TestMarkCompletedProjectScopeDoesNotUpdateRunningOrContinued(t *testing.T) {
	env := newTestEnv(t)

	wfiID := env.initProjectWorkflow(t, "test")

	// Insert agent sessions with running and continued statuses
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sessions := []struct {
		id     string
		status model.AgentSessionStatus
	}{
		{"session-running", model.AgentSessionRunning},
		{"session-continued", model.AgentSessionContinued},
	}

	for _, s := range sessions {
		_, err = database.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, created_at, updated_at)
			VALUES (?, ?, '', ?, 'test-phase', 'test-agent', ?, ?, ?)`,
			s.id, env.project, wfiID, s.status, now, now)
		if err != nil {
			t.Fatalf("failed to insert session %s: %v", s.id, err)
		}
	}

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ScopeType:    "project",
	})

	// Verify sessions are NOT changed
	asRepo := repo.NewAgentSessionRepo(database, clock.Real())
	for _, s := range sessions {
		session, err := asRepo.Get(s.id)
		if err != nil {
			t.Fatalf("failed to get session %s: %v", s.id, err)
		}
		if session.Status != s.status {
			t.Fatalf("expected session %s status %v, got %v", s.id, s.status, session.Status)
		}
	}
}

func TestMarkCompletedTicketScopeStillUsesCompleted(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MC-TICKET", "Ticket scope test")
	wfiID := env.initWorkflow(t, "MC-TICKET")

	// Insert agent session with completed status
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'test-phase', 'test-agent', ?, ?, ?)`,
		"session-ticket", env.project, "MC-TICKET", wfiID, model.AgentSessionCompleted, now, now)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "MC-TICKET",
		WorkflowName:          "test",
		ScopeType:             "ticket",
		CloseTicketOnComplete: true,
	})

	// Verify workflow instance status is completed (not project_completed)
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceCompleted {
		t.Fatalf("expected workflow status 'completed', got %v", wi.Status)
	}

	// Verify agent session status is still completed (not project_completed)
	asRepo := repo.NewAgentSessionRepo(database, clock.Real())
	session, err := asRepo.Get("session-ticket")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.Status != model.AgentSessionCompleted {
		t.Fatalf("expected session status 'completed', got %v", session.Status)
	}
}

func TestMarkCompletedBroadcastsTicketUpdatedEvent(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MC-WS1", "Ticket update WS event")
	wfiID := env.initWorkflow(t, "MC-WS1")

	// Subscribe WS client
	ch := env.subscribeWSClient(t, "ws-mcws1", "MC-WS1")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "MC-WS1",
		WorkflowName:          "test",
		CloseTicketOnComplete: true,
	})

	// Expect ticket.updated event with status=closed
	event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	if event.TicketID != "MC-WS1" {
		t.Fatalf("expected ticket_id 'MC-WS1', got %v", event.TicketID)
	}
	status, ok := event.Data["status"]
	if !ok {
		t.Fatal("expected 'status' in event data")
	}
	if status != "closed" {
		t.Fatalf("expected status 'closed' in event, got %v", status)
	}
}

func TestMarkCompletedProjectScopeDoesNotBroadcastTicketUpdated(t *testing.T) {
	env := newTestEnv(t)

	wfiID := env.initProjectWorkflow(t, "test")

	// Subscribe WS client (to project, empty ticket)
	ch := env.subscribeWSClient(t, "ws-proj", "")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ScopeType:    "project",
	})

	// Expect orchestration.completed event but NOT ticket.updated
	event := expectEvent(t, ch, ws.EventOrchestrationCompleted, 2*time.Second)
	if event.Data["instance_id"] != wfiID {
		t.Fatalf("expected instance_id %q, got %v", wfiID, event.Data["instance_id"])
	}

	// Ensure no ticket.updated event arrives
	select {
	case msg := <-ch:
		var evt ws.Event
		if err := json.Unmarshal(msg, &evt); err == nil && evt.Type == ws.EventTicketUpdated {
			t.Fatalf("unexpected ticket.updated event for project-scoped workflow")
		}
	case <-time.After(500 * time.Millisecond):
		// Good - no ticket.updated event
	}
}

func TestMarkCompletedBroadcastsWorkflowFinalResult(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MC-FR1", "Final result test")
	wfiID := env.initWorkflow(t, "MC-FR1")

	// Insert an agent_session with workflow_final_result finding and a non-NULL ended_at
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			status, result, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'test-phase', 'analyzer', 'completed', 'pass', ?, ?, ?)`,
		"sess-fr1", env.project, "MC-FR1", wfiID,
		now, now, now)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}
	// Seed workflow_final_result finding via FindingRepo
	findingRepo := repo.NewFindingRepo(env.pool, clock.Real())
	if err := findingRepo.Upsert("session", "sess-fr1", "workflow_final_result", []byte(`"hello"`),
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "analyzer"}, repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("failed to seed finding: %v", err)
	}

	ch := env.subscribeWSClient(t, "ws-fr1", "MC-FR1")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "MC-FR1",
		WorkflowName: "test",
	})

	event := expectEvent(t, ch, ws.EventOrchestrationCompleted, 2*time.Second)
	if event.Data["workflow_final_result"] != "hello" {
		t.Errorf("expected workflow_final_result 'hello', got %v", event.Data["workflow_final_result"])
	}
}

func TestMarkCompletedOmitsWorkflowFinalResultWhenAbsent(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MC-FR2", "No final result")
	wfiID := env.initWorkflow(t, "MC-FR2")

	// Insert an agent_session with a different finding key (no workflow_final_result)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			status, result, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'test-phase', 'analyzer', 'completed', 'pass', ?, ?, ?)`,
		"sess-fr2", env.project, "MC-FR2", wfiID,
		now, now, now)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}
	// Seed a different finding (no workflow_final_result) via FindingRepo
	findingRepo2 := repo.NewFindingRepo(env.pool, clock.Real())
	if err := findingRepo2.Upsert("session", "sess-fr2", "some_other_key", []byte(`"value"`),
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "analyzer"}, repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("failed to seed finding: %v", err)
	}

	ch := env.subscribeWSClient(t, "ws-fr2", "MC-FR2")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "MC-FR2",
		WorkflowName: "test",
	})

	event := expectEvent(t, ch, ws.EventOrchestrationCompleted, 2*time.Second)
	if _, ok := event.Data["workflow_final_result"]; ok {
		t.Errorf("expected workflow_final_result to be absent, but it was present: %v", event.Data["workflow_final_result"])
	}
	if event.Data["instance_id"] != wfiID {
		t.Errorf("expected instance_id %q, got %v", wfiID, event.Data["instance_id"])
	}
}
