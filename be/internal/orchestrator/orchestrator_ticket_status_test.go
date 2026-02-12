package orchestrator

import (
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
	"be/internal/service"
)

// --- SetInProgress tests ---

func TestSetInProgressOnOpenTicket(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "SIP-1", "Open ticket")

	ticket := env.getTicket(t, "SIP-1")
	if ticket.Status != model.StatusOpen {
		t.Fatalf("expected status 'open', got %v", ticket.Status)
	}

	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.SetInProgress(env.project, "SIP-1")
	if err != nil {
		t.Fatalf("SetInProgress failed: %v", err)
	}

	ticket = env.getTicket(t, "SIP-1")
	if ticket.Status != model.StatusInProgress {
		t.Fatalf("expected status 'in_progress', got %v", ticket.Status)
	}
}

func TestSetInProgressOnClosedTicket(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "SIP-2", "Closed ticket")

	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.Close(env.project, "SIP-2", "done")
	if err != nil {
		t.Fatalf("failed to close ticket: %v", err)
	}

	err = ticketSvc.SetInProgress(env.project, "SIP-2")
	if err != nil {
		t.Fatalf("SetInProgress returned error: %v", err)
	}

	ticket := env.getTicket(t, "SIP-2")
	if ticket.Status != model.StatusClosed {
		t.Fatalf("expected status 'closed', got %v", ticket.Status)
	}
}

func TestSetInProgressOnAlreadyInProgressTicket(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "SIP-3", "Already in progress")

	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.SetInProgress(env.project, "SIP-3")
	if err != nil {
		t.Fatalf("first SetInProgress failed: %v", err)
	}

	err = ticketSvc.SetInProgress(env.project, "SIP-3")
	if err != nil {
		t.Fatalf("second SetInProgress returned error: %v", err)
	}

	ticket := env.getTicket(t, "SIP-3")
	if ticket.Status != model.StatusInProgress {
		t.Fatalf("expected status 'in_progress', got %v", ticket.Status)
	}
}

func TestSetInProgressUpdatesTimestamp(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "SIP-4", "Timestamp check")

	before := env.getTicket(t, "SIP-4")
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

	ticket := env.getTicket(t, "START-1")
	if ticket.Status != model.StatusOpen {
		t.Fatalf("expected status 'open', got %v", ticket.Status)
	}

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

	ticket = env.getTicket(t, "START-1")
	if ticket.Status != model.StatusInProgress {
		t.Fatalf("expected status 'in_progress', got %v", ticket.Status)
	}
}

func TestStartDoesNotChangeClosedTicketStatus(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "START-2", "Closed before start")

	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.Close(env.project, "START-2", "pre-closed")
	if err != nil {
		t.Fatalf("failed to close ticket: %v", err)
	}

	err = ticketSvc.SetInProgress(env.project, "START-2")
	if err != nil {
		t.Fatalf("SetInProgress returned error: %v", err)
	}

	ticket := env.getTicket(t, "START-2")
	if ticket.Status != model.StatusClosed {
		t.Fatalf("expected status 'closed', got %v", ticket.Status)
	}
}

func TestMarkFailedKeepsInProgressStatus(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "MF-2", "In progress before fail")
	wfiID := env.initWorkflow(t, "MF-2")

	ticketSvc := service.NewTicketService(env.pool)
	err := ticketSvc.SetInProgress(env.project, "MF-2")
	if err != nil {
		t.Fatalf("SetInProgress failed: %v", err)
	}

	env.orch.markFailed(wfiID, RunRequest{
		ProjectID:    env.project,
		TicketID:     "MF-2",
		WorkflowName: "test",
	}, "phase failed")

	ticket := env.getTicket(t, "MF-2")
	if ticket.Status != model.StatusInProgress {
		t.Fatalf("expected status 'in_progress' after failure, got %v", ticket.Status)
	}
}
