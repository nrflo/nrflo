package integration

import (
	"testing"
	"time"

	"be/internal/model"
)

// insertEpicWithChildren inserts an epic ticket and child tickets into the DB.
// Children are created with parent_ticket_id pointing to the epic.
func insertEpicWithChildren(t *testing.T, env *TestEnv, epicID string, childIDs []string) {
	t.Helper()
	base := time.Now().UTC()

	_, err := env.Pool.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, ?, 'open', 'epic', 2, ?, ?, 'test')`,
		epicID, env.ProjectID, epicID+" epic", base.Format(time.RFC3339Nano), base.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to create epic %s: %v", epicID, err)
	}

	for i, childID := range childIDs {
		ts := base.Add(time.Duration(i+1) * time.Millisecond)
		_, err = env.Pool.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, parent_ticket_id, created_at, updated_at, created_by)
			VALUES (?, ?, ?, 'open', 'task', 2, ?, ?, ?, 'test')`,
			childID, env.ProjectID, childID+" task", epicID, ts.Format(time.RFC3339Nano), ts.Format(time.RFC3339Nano))
		if err != nil {
			t.Fatalf("failed to create child %s: %v", childID, err)
		}
	}
}

// TestTryCloseParentEpic_NoParent verifies that a ticket with no parent returns nil, no error.
func TestTryCloseParentEpic_NoParent(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "epica-orphan", "Orphan ticket")

	epic, err := env.TicketSvc.TryCloseParentEpic(env.ProjectID, "epica-orphan")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if epic != nil {
		t.Fatalf("expected nil for ticket with no parent, got %+v", epic)
	}
}

// TestTryCloseParentEpic_ParentNotEpic verifies no auto-close when parent is not an epic.
func TestTryCloseParentEpic_ParentNotEpic(t *testing.T) {
	env := NewTestEnv(t)
	base := time.Now().UTC()

	// Insert a "task" parent (not epic)
	_, err := env.Pool.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, ?, 'open', 'task', 2, ?, ?, 'test')`,
		"epica-parent-task", env.ProjectID, "Parent task",
		base.Format(time.RFC3339Nano), base.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to create parent task: %v", err)
	}

	// Child with parent_ticket_id pointing to a task
	_, err = env.Pool.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, parent_ticket_id, created_at, updated_at, created_by)
		VALUES (?, ?, ?, 'open', 'task', 2, ?, ?, ?, 'test')`,
		"epica-child-1", env.ProjectID, "Child task", "epica-parent-task",
		base.Add(time.Millisecond).Format(time.RFC3339Nano),
		base.Add(time.Millisecond).Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	epic, err := env.TicketSvc.TryCloseParentEpic(env.ProjectID, "epica-child-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if epic != nil {
		t.Fatalf("expected nil for non-epic parent, got %+v", epic)
	}

	// Verify parent is still open
	parent, err := env.TicketSvc.Get(env.ProjectID, "epica-parent-task")
	if err != nil {
		t.Fatalf("failed to get parent: %v", err)
	}
	if parent.Status != model.StatusOpen {
		t.Errorf("expected parent (task) to remain open, got %q", parent.Status)
	}
}

// TestTryCloseParentEpic_ParentEpicAlreadyClosed verifies idempotence: no error, no re-close.
func TestTryCloseParentEpic_ParentEpicAlreadyClosed(t *testing.T) {
	env := NewTestEnv(t)
	insertEpicWithChildren(t, env, "epica-epic-closed", []string{"epica-child-cc1"})

	// Pre-close the epic with a specific reason
	if err := env.TicketSvc.Close(env.ProjectID, "epica-epic-closed", "Pre-closed manually"); err != nil {
		t.Fatalf("failed to close epic: %v", err)
	}
	// Close the child too
	if err := env.TicketSvc.Close(env.ProjectID, "epica-child-cc1", "done"); err != nil {
		t.Fatalf("failed to close child: %v", err)
	}

	// TryCloseParentEpic should return nil (epic already closed)
	result, err := env.TicketSvc.TryCloseParentEpic(env.ProjectID, "epica-child-cc1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil for already-closed epic, got %+v", result)
	}

	// Verify the original close_reason was NOT overwritten
	epic, err := env.TicketSvc.Get(env.ProjectID, "epica-epic-closed")
	if err != nil {
		t.Fatalf("failed to get epic: %v", err)
	}
	if !epic.CloseReason.Valid || epic.CloseReason.String != "Pre-closed manually" {
		t.Errorf("expected close_reason 'Pre-closed manually', got %q (valid=%v)",
			epic.CloseReason.String, epic.CloseReason.Valid)
	}
}

// TestTryCloseParentEpic_SiblingsStillOpen verifies epic stays open when siblings remain open.
func TestTryCloseParentEpic_SiblingsStillOpen(t *testing.T) {
	env := NewTestEnv(t)
	childIDs := []string{"epica-sib-a", "epica-sib-b", "epica-sib-c"}
	insertEpicWithChildren(t, env, "epica-epic-sib", childIDs)

	// Close first two children — epic must remain open each time
	for _, childID := range childIDs[:2] {
		if err := env.TicketSvc.Close(env.ProjectID, childID, "done"); err != nil {
			t.Fatalf("failed to close %s: %v", childID, err)
		}
		result, err := env.TicketSvc.TryCloseParentEpic(env.ProjectID, childID)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", childID, err)
		}
		if result != nil {
			t.Errorf("expected epic to remain open after closing %s (sibling still open)", childID)
		}
	}

	// Verify epic is still open
	epic, err := env.TicketSvc.Get(env.ProjectID, "epica-epic-sib")
	if err != nil {
		t.Fatalf("failed to get epic: %v", err)
	}
	if epic.Status != model.StatusOpen {
		t.Errorf("expected epic status 'open', got %q", epic.Status)
	}
}

// TestTryCloseParentEpic_LastChildAutoClosesEpic verifies auto-close on last open child.
func TestTryCloseParentEpic_LastChildAutoClosesEpic(t *testing.T) {
	env := NewTestEnv(t)
	childIDs := []string{"epica-last-a", "epica-last-b", "epica-last-c"}
	insertEpicWithChildren(t, env, "epica-epic-last", childIDs)

	// Close first two silently
	for _, childID := range childIDs[:2] {
		if err := env.TicketSvc.Close(env.ProjectID, childID, "done"); err != nil {
			t.Fatalf("failed to close %s: %v", childID, err)
		}
		env.TicketSvc.TryCloseParentEpic(env.ProjectID, childID) //nolint — result ignored intentionally
	}

	// Close last child
	if err := env.TicketSvc.Close(env.ProjectID, "epica-last-c", "done"); err != nil {
		t.Fatalf("failed to close last child: %v", err)
	}

	result, err := env.TicketSvc.TryCloseParentEpic(env.ProjectID, "epica-last-c")
	if err != nil {
		t.Fatalf("unexpected error closing last child: %v", err)
	}
	if result == nil {
		t.Fatal("expected closed epic to be returned, got nil")
	}
	if result.Status != model.StatusClosed {
		t.Errorf("returned epic: expected status 'closed', got %q", result.Status)
	}
	if !result.CloseReason.Valid || result.CloseReason.String != "All child tickets closed" {
		t.Errorf("expected close_reason 'All child tickets closed', got %q (valid=%v)",
			result.CloseReason.String, result.CloseReason.Valid)
	}

	// Verify via Get
	epicAfter, err := env.TicketSvc.Get(env.ProjectID, "epica-epic-last")
	if err != nil {
		t.Fatalf("failed to get epic after auto-close: %v", err)
	}
	if epicAfter.Status != model.StatusClosed {
		t.Errorf("Get after auto-close: expected status 'closed', got %q", epicAfter.Status)
	}
}
