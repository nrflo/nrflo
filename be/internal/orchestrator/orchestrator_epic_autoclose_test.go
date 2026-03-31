package orchestrator

import (
	"testing"
	"time"

	"be/internal/model"
	"be/internal/ws"
)

// insertEpicWithChildrenOrch inserts an epic + child tickets into the orchestrator test DB.
func insertEpicWithChildrenOrch(t *testing.T, env *testEnv, epicID string, childIDs []string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := env.pool.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, ?, 'open', 'epic', 2, ?, ?, 'test')`,
		epicID, env.project, epicID+" epic", now, now)
	if err != nil {
		t.Fatalf("failed to create epic %s: %v", epicID, err)
	}

	for i, childID := range childIDs {
		ts := time.Now().UTC().Add(time.Duration(i+1) * time.Millisecond).Format(time.RFC3339Nano)
		_, err = env.pool.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, parent_ticket_id, created_at, updated_at, created_by)
			VALUES (?, ?, ?, 'open', 'task', 2, ?, ?, ?, 'test')`,
			childID, env.project, childID+" task", epicID, ts, ts)
		if err != nil {
			t.Fatalf("failed to create child %s: %v", childID, err)
		}
	}
}

// TestMarkCompleted_AutoClosesParentEpic verifies that when the last open child's workflow
// completes, the parent epic is auto-closed with the correct reason.
func TestMarkCompleted_AutoClosesParentEpic(t *testing.T) {
	env := newTestEnv(t)
	insertEpicWithChildrenOrch(t, env, "orch-epic-mc1", []string{"orch-mc1-a", "orch-mc1-b"})

	wfiA := env.initWorkflow(t, "orch-mc1-a")
	wfiB := env.initWorkflow(t, "orch-mc1-b")

	// Complete first child — epic should remain open
	env.orch.markCompleted(wfiA, RunRequest{
		ProjectID:             env.project,
		TicketID:              "orch-mc1-a",
		WorkflowName:          "test",
		CloseTicketOnComplete: true,
	})

	epicAfterFirst := env.getTicket(t, "orch-epic-mc1")
	if epicAfterFirst.Status != model.StatusOpen {
		t.Errorf("expected epic open after first child workflow complete, got %q", epicAfterFirst.Status)
	}

	// Complete second (last) child — epic should auto-close
	env.orch.markCompleted(wfiB, RunRequest{
		ProjectID:             env.project,
		TicketID:              "orch-mc1-b",
		WorkflowName:          "test",
		CloseTicketOnComplete: true,
	})

	epicAfterAll := env.getTicket(t, "orch-epic-mc1")
	if epicAfterAll.Status != model.StatusClosed {
		t.Errorf("expected epic closed after last child workflow complete, got %q", epicAfterAll.Status)
	}
	if !epicAfterAll.CloseReason.Valid || epicAfterAll.CloseReason.String != "All child tickets closed" {
		t.Errorf("expected close_reason 'All child tickets closed', got %q (valid=%v)",
			epicAfterAll.CloseReason.String, epicAfterAll.CloseReason.Valid)
	}
}

// TestMarkCompleted_AutoClosesParentEpic_BroadcastsWSEvent verifies that when workflow
// completion auto-closes the parent epic, a EventTicketUpdated WS event is broadcast for it.
func TestMarkCompleted_AutoClosesParentEpic_BroadcastsWSEvent(t *testing.T) {
	env := newTestEnv(t)
	insertEpicWithChildrenOrch(t, env, "orch-epic-ws1", []string{"orch-ws1-child"})

	wfiID := env.initWorkflow(t, "orch-ws1-child")

	// Subscribe to epic-specific events
	ch := env.subscribeWSClient(t, "ws-epic-mc", "orch-epic-ws1")

	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "orch-ws1-child",
		WorkflowName:          "test",
		CloseTicketOnComplete: true,
	})

	// Expect ticket.updated for the epic with status=closed
	event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	if event.TicketID != "orch-epic-ws1" {
		t.Errorf("expected epic ticket_id 'orch-epic-ws1', got %q", event.TicketID)
	}
	if event.Data["status"] != "closed" {
		t.Errorf("expected epic status 'closed' in WS event, got %v", event.Data["status"])
	}
}

// TestMarkCompleted_EpicWithOpenSiblings verifies the epic stays open when only one
// of several children finishes its workflow.
func TestMarkCompleted_EpicWithOpenSiblings(t *testing.T) {
	env := newTestEnv(t)
	insertEpicWithChildrenOrch(t, env, "orch-epic-sib1", []string{"orch-sib-a", "orch-sib-b"})

	wfiA := env.initWorkflow(t, "orch-sib-a")
	env.initWorkflow(t, "orch-sib-b") // workflow exists but NOT completed

	// Complete only sibling-A
	env.orch.markCompleted(wfiA, RunRequest{
		ProjectID:             env.project,
		TicketID:              "orch-sib-a",
		WorkflowName:          "test",
		CloseTicketOnComplete: true,
	})

	// Epic must remain open (orch-sib-b is still open)
	epic := env.getTicket(t, "orch-epic-sib1")
	if epic.Status != model.StatusOpen {
		t.Errorf("expected epic open while sibling-b still open, got %q", epic.Status)
	}
}

// TestMarkCompleted_NoParentEpic verifies that when a ticket with no parent completes,
// markCompleted still succeeds and does not error.
func TestMarkCompleted_NoParentEpic(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "orch-noparent-1", "No parent ticket")
	wfiID := env.initWorkflow(t, "orch-noparent-1")

	// Should not panic or error
	env.orch.markCompleted(wfiID, RunRequest{
		ProjectID:             env.project,
		TicketID:              "orch-noparent-1",
		WorkflowName:          "test",
		CloseTicketOnComplete: true,
	})

	ticket := env.getTicket(t, "orch-noparent-1")
	if ticket.Status != model.StatusClosed {
		t.Errorf("expected ticket closed after markCompleted, got %q", ticket.Status)
	}
}
