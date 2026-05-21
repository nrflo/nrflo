package repo

import (
	"fmt"
	"strings"
	"testing"

	"be/internal/model"
)

// TestWorkflowInstanceWaiting_Const verifies the constant value.
func TestWorkflowInstanceWaiting_Const(t *testing.T) {
	t.Parallel()
	if model.WorkflowInstanceWaiting != "waiting" {
		t.Errorf("WorkflowInstanceWaiting = %q, want \"waiting\"", model.WorkflowInstanceWaiting)
	}
}

// TestWorkflowInstance_StatusWaiting_CheckConstraint verifies that the 'waiting' status
// value passes the CHECK constraint in the workflow_instances table (migration 000124).
func TestWorkflowInstance_StatusWaiting_CheckConstraint(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('p-wait', 'P', '/tmp', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES ('wf-wait', 'p-wait', '', 'ticket', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	// Insert with status='waiting' directly to verify CHECK constraint.
	_, err := pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, created_at, updated_at)
		 VALUES ('wfi-wait-check', 'p-wait', '', 'wf-wait', 'ticket', 'waiting', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("INSERT with status='waiting' failed: %v (CHECK constraint may not accept 'waiting')", err)
	}

	var status string
	if err := pool.QueryRow(`SELECT status FROM workflow_instances WHERE id = 'wfi-wait-check'`).Scan(&status); err != nil {
		t.Fatalf("SELECT status: %v", err)
	}
	if status != "waiting" {
		t.Errorf("status = %q, want \"waiting\"", status)
	}
}

// TestWorkflowInstance_InvalidStatus_Rejected verifies that unknown status values are rejected.
func TestWorkflowInstance_InvalidStatus_Rejected(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('p-inv', 'P', '/tmp', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES ('wf-inv', 'p-inv', '', 'ticket', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	_, err := pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, created_at, updated_at)
		 VALUES ('wfi-bad', 'p-inv', '', 'wf-inv', 'ticket', 'bogus_status', datetime('now'), datetime('now'))`,
	)
	if err == nil {
		t.Error("INSERT with invalid status expected error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "CHECK") && !strings.Contains(err.Error(), "constraint") {
		t.Errorf("error = %q, want CHECK constraint violation", err.Error())
	}
}

// TestWorkflowInstanceWaiting_AllValidStatuses verifies all valid status values pass the CHECK constraint.
func TestWorkflowInstanceWaiting_AllValidStatuses(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('p-allst', 'P', '/tmp', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES ('wf-allst', 'p-allst', '', 'ticket', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	statuses := []string{"active", "completed", "failed", "project_completed", "waiting"}
	for i, st := range statuses {
		id := "wfi-st-" + strings.ReplaceAll(st, "_", "-")
		ticketID := fmt.Sprintf("ticket-%d", i)
		if _, err := pool.Exec(
			`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, created_at, updated_at)
			 VALUES (?, 'p-allst', ?, 'wf-allst', 'ticket', ?, datetime('now'), datetime('now'))`,
			id, ticketID, st,
		); err != nil {
			t.Errorf("INSERT with status=%q failed: %v", st, err)
		}
	}
}
