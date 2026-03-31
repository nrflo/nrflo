package orchestrator

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/ws"
)

// TestMarkCompleted_CloseTicketFalse_DoesNotCloseTicket verifies that
// when CloseTicketOnComplete=false, the ticket is NOT closed after workflow completion.
func TestMarkCompleted_CloseTicketFalse_DoesNotCloseTicket(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CTF-1", "No close on complete")
	wfiID := env.initWorkflow(t, "CTF-1")

	// Verify ticket starts open
	ticket := env.getTicket(t, "CTF-1")
	if ticket.Status != model.StatusOpen {
		t.Fatalf("expected ticket status 'open', got %v", ticket.Status)
	}

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "CTF-1",
		WorkflowName:          "test",
		CloseTicketOnComplete: false,
	})

	// Ticket should remain open
	ticket = env.getTicket(t, "CTF-1")
	if ticket.Status != model.StatusOpen {
		t.Fatalf("expected ticket status 'open' after markCompleted(false), got %v", ticket.Status)
	}

	// close_reason must not be set
	if ticket.CloseReason.Valid {
		t.Fatalf("expected close_reason to be unset, got %q", ticket.CloseReason.String)
	}
}

// TestMarkCompleted_CloseTicketFalse_WorkflowInstanceCompleted verifies that
// when CloseTicketOnComplete=false the workflow instance is still marked completed.
func TestMarkCompleted_CloseTicketFalse_WorkflowInstanceCompleted(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CTF-2", "Instance completed regardless")
	wfiID := env.initWorkflow(t, "CTF-2")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "CTF-2",
		WorkflowName:          "test",
		CloseTicketOnComplete: false,
	})

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceCompleted {
		t.Fatalf("expected workflow status 'completed', got %v", wi.Status)
	}
}

// TestMarkCompleted_CloseTicketFalse_BroadcastsOrchestrationCompleted verifies that
// EventOrchestrationCompleted is still broadcast even when CloseTicketOnComplete=false.
func TestMarkCompleted_CloseTicketFalse_BroadcastsOrchestrationCompleted(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CTF-3", "Orchestration event still fires")
	wfiID := env.initWorkflow(t, "CTF-3")

	ch := env.subscribeWSClient(t, "ws-ctf3", "CTF-3")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "CTF-3",
		WorkflowName:          "test",
		CloseTicketOnComplete: false,
	})

	// EventOrchestrationCompleted must fire unconditionally
	event := expectEvent(t, ch, ws.EventOrchestrationCompleted, 2*time.Second)
	if event.TicketID != "CTF-3" {
		t.Fatalf("expected ticket_id 'CTF-3', got %v", event.TicketID)
	}
	if event.Data["instance_id"] != wfiID {
		t.Fatalf("expected instance_id %q, got %v", wfiID, event.Data["instance_id"])
	}
}

// TestMarkCompleted_CloseTicketFalse_DoesNotBroadcastTicketUpdated verifies that
// EventTicketUpdated is NOT broadcast when CloseTicketOnComplete=false.
//
// Uses EventOrchestrationCompleted as a sentinel: it fires after the ticket-close
// code path, so any ticket.updated would have arrived before it.
func TestMarkCompleted_CloseTicketFalse_DoesNotBroadcastTicketUpdated(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CTF-4", "No ticket.updated event")
	wfiID := env.initWorkflow(t, "CTF-4")

	ch := env.subscribeWSClient(t, "ws-ctf4", "CTF-4")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "CTF-4",
		WorkflowName:          "test",
		CloseTicketOnComplete: false,
	})

	// Drain until orchestration.completed (sentinel). ticket.updated would have
	// been emitted before it if ticket closing had been attempted.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var evt ws.Event
			if err := json.Unmarshal(msg, &evt); err != nil {
				t.Fatalf("failed to unmarshal event: %v", err)
			}
			if evt.Type == ws.EventTicketUpdated {
				t.Fatalf("unexpected ticket.updated event when CloseTicketOnComplete=false")
			}
			if evt.Type == ws.EventOrchestrationCompleted {
				return // sentinel reached, no ticket.updated seen — pass
			}
		case <-deadline:
			t.Fatal("timeout waiting for orchestration.completed sentinel")
		}
	}
}

// TestMarkCompleted_CloseTicketTrue_DefaultBehaviorUnchanged is an explicit
// regression guard: CloseTicketOnComplete=true must still close the ticket.
func TestMarkCompleted_CloseTicketTrue_DefaultBehaviorUnchanged(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CTF-5", "Explicit true closes ticket")
	wfiID := env.initWorkflow(t, "CTF-5")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "CTF-5",
		WorkflowName:          "test",
		CloseTicketOnComplete: true,
	})

	ticket := env.getTicket(t, "CTF-5")
	if ticket.Status != model.StatusClosed {
		t.Fatalf("expected ticket status 'closed' with CloseTicketOnComplete=true, got %v", ticket.Status)
	}
	if !ticket.CloseReason.Valid {
		t.Fatal("expected close_reason to be set")
	}
}
