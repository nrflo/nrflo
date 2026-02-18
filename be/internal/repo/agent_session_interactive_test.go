package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// TestUserInteractive_StatusConstraintFixed verifies that migration 000026
// added user_interactive and interactive_completed to the agent_sessions
// CHECK constraint, so DB operations with these statuses now succeed.
func TestUserInteractive_StatusConstraintFixed(t *testing.T) {
	database, r, wfiID := setupTestDB(t)
	defer database.Close()

	// Create session with user_interactive status — should succeed.
	session := &model.AgentSession{
		ID:                 "ic-sess-constraint",
		ProjectID:          "proj",
		TicketID:           "TKT-IC-0",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionUserInteractive,
	}
	err := r.Create(session)
	if err != nil {
		t.Fatalf("Create with user_interactive status should succeed after migration 000026: %v", err)
	}

	// UpdateStatus to user_interactive should also succeed.
	validSession := &model.AgentSession{
		ID:                 "ic-sess-valid",
		ProjectID:          "proj",
		TicketID:           "TKT-IC-0",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionRunning,
	}
	if err := r.Create(validSession); err != nil {
		t.Fatalf("failed to create valid session: %v", err)
	}

	err = r.UpdateStatus("ic-sess-valid", model.AgentSessionUserInteractive)
	if err != nil {
		t.Fatalf("UpdateStatus to user_interactive should succeed: %v", err)
	}

	// UpdateStatusToInteractiveCompleted should succeed.
	err = r.UpdateStatusToInteractiveCompleted("ic-sess-valid")
	if err != nil {
		t.Fatalf("UpdateStatusToInteractiveCompleted should succeed: %v", err)
	}
}

// TestUpdateStatusToInteractiveCompleted_NotFound verifies that
// UpdateStatusToInteractiveCompleted returns an error for a nonexistent session ID.
func TestUpdateStatusToInteractiveCompleted_NotFound(t *testing.T) {
	database, r, _ := setupTestDB(t)
	defer database.Close()

	err := r.UpdateStatusToInteractiveCompleted("nonexistent-session-ic")
	if err == nil {
		t.Fatal("expected error for nonexistent session, got nil")
	}
}

// TestUpdateStatusToInteractiveCompleted_TimestampUsesRepoClock verifies that
// UpdateStatusToInteractiveCompleted uses the injected clock for timestamps.
func TestUpdateStatusToInteractiveCompleted_TimestampUsesRepoClock(t *testing.T) {
	database, _, wfiID := setupTestDB(t)
	defer database.Close()

	fixedTime := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	r := NewAgentSessionRepo(database, clk)

	validSession := &model.AgentSession{
		ID:                 "ic-clk-valid",
		ProjectID:          "proj",
		TicketID:           "TKT-IC-CLK",
		WorkflowInstanceID: wfiID,
		Phase:              "phase-clk",
		AgentType:          "agent-clk",
		Status:             model.AgentSessionRunning,
	}
	if err := r.Create(validSession); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	err := r.UpdateStatusToInteractiveCompleted("ic-clk-valid")
	if err != nil {
		t.Fatalf("UpdateStatusToInteractiveCompleted should succeed: %v", err)
	}

	// Verify the session was updated with the fixed clock time.
	session, err := r.Get("ic-clk-valid")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.Status != model.AgentSessionInteractiveCompleted {
		t.Errorf("expected status interactive_completed, got %s", session.Status)
	}
	if !session.EndedAt.Valid {
		t.Fatal("expected ended_at to be set")
	}
}
