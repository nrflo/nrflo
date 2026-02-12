package orchestrator

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/model"
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

	env.createTicket(t, "MC-4", "Real ticket")
	wfiID := env.initWorkflow(t, "MC-4")

	ch := env.subscribeWSClient(t, "ws-mc4", "NONEXISTENT")

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

func TestMarkCompletedCloseReasonIncludesWorkflowName(t *testing.T) {
	env := newTestEnv(t)

	// Create a second workflow definition for this test
	workflowSvc := service.NewWorkflowService(env.pool)
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "analyzer", "layer": 0},
	})
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
