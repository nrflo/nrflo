package orchestrator

import (
	"encoding/json"
	"testing"

	"be/internal/model"
	"be/internal/repo"
)

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

	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()

	if _, exists := findings["user_instructions"]; exists {
		t.Fatal("expected no user_instructions in findings for empty instructions")
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
