package orchestrator

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/ws"
)

// TestMarkCompleted_CloseTicketFalse verifies all behaviors when CloseTicketOnComplete=false:
// ticket stays open, workflow instance is completed, orchestration.completed fires, ticket.updated does not fire.
func TestMarkCompleted_CloseTicketFalse(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CTF-1", "No close on complete")
	wfiID := env.initWorkflow(t, "CTF-1")

	// Verify ticket starts open
	ticket := env.getTicket(t, "CTF-1")
	if ticket.Status != model.StatusOpen {
		t.Fatalf("expected ticket status 'open', got %v", ticket.Status)
	}

	ch := env.subscribeWSClient(t, "ws-ctf1", "CTF-1")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "CTF-1",
		WorkflowName:          "test",
		CloseTicketOnComplete: false,
	})

	// Ticket must remain open with no close_reason
	ticket = env.getTicket(t, "CTF-1")
	if ticket.Status != model.StatusOpen {
		t.Fatalf("expected ticket status 'open' after markCompleted(false), got %v", ticket.Status)
	}
	if ticket.CloseReason.Valid {
		t.Fatalf("expected close_reason to be unset, got %q", ticket.CloseReason.String)
	}

	// Workflow instance must be completed
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceCompleted {
		t.Fatalf("expected workflow status 'completed', got %v", wi.Status)
	}

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
				if evt.TicketID != "CTF-1" {
					t.Fatalf("expected ticket_id 'CTF-1', got %v", evt.TicketID)
				}
				if evt.Data["instance_id"] != wfiID {
					t.Fatalf("expected instance_id %q, got %v", wfiID, evt.Data["instance_id"])
				}
				return // all assertions passed
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
