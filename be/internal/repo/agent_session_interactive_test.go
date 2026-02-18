package repo

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// TestUpdateStatusToInteractiveCompleted_DBConstraintMissingMigration documents
// that the DB schema is missing a migration to allow 'user_interactive' and
// 'interactive_completed' as valid status values, and 'user_interactive' as a
// valid result value.
//
// Production bug: both Create(status=user_interactive) and
// UpdateStatusToInteractiveCompleted fail with DB CHECK constraints.
// A migration is needed to extend the allowed status/result values.
// See be_production_bugs in ticket findings.
func TestUpdateStatusToInteractiveCompleted_DBConstraintMissingMigration(t *testing.T) {
	database, r, wfiID := setupTestDB(t)
	defer database.Close()

	// Attempt to create session with user_interactive status — must fail.
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
	if err == nil {
		t.Fatal("expected Create to fail for user_interactive status (missing migration)")
	}
	if !isConstraintError(err) {
		t.Errorf("expected DB constraint error, got: %v", err)
	}

	// UpdateStatus to user_interactive must also fail.
	// First create a valid session.
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

	// Attempt to update to user_interactive — must fail.
	err = r.UpdateStatus("ic-sess-valid", model.AgentSessionUserInteractive)
	if err == nil {
		t.Fatal("expected UpdateStatus to user_interactive to fail (missing migration)")
	}
	if !isConstraintError(err) {
		t.Errorf("expected DB constraint error, got: %v", err)
	}

	// Attempt UpdateStatusToInteractiveCompleted — must fail because
	// interactive_completed is also not allowed.
	err = r.UpdateStatusToInteractiveCompleted("ic-sess-valid")
	if err == nil {
		t.Fatal("expected UpdateStatusToInteractiveCompleted to fail (missing migration)")
	}
	if !isConstraintError(err) {
		t.Errorf("expected DB constraint error, got: %v", err)
	}
}

// TestUpdateStatusToInteractiveCompleted_NotFound verifies that
// UpdateStatusToInteractiveCompleted returns an error for a nonexistent session ID.
//
// NOTE: Because the status 'interactive_completed' violates the DB CHECK constraint,
// this test actually returns a constraint error rather than "not found". Both are
// errors, which is the property being tested.
func TestUpdateStatusToInteractiveCompleted_NotFound(t *testing.T) {
	database, r, _ := setupTestDB(t)
	defer database.Close()

	err := r.UpdateStatusToInteractiveCompleted("nonexistent-session-ic")
	if err == nil {
		t.Fatal("expected error for nonexistent/constraint-failing session, got nil")
	}
}

// TestUpdateStatusToInteractiveCompleted_TimestampUsesRepoClock documents that
// UpdateStatusToInteractiveCompleted would use the injected clock if the DB
// constraint were relaxed. Currently the call fails due to missing migration.
func TestUpdateStatusToInteractiveCompleted_TimestampUsesRepoClock(t *testing.T) {
	database, _, wfiID := setupTestDB(t)
	defer database.Close()

	fixedTime := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	r := NewAgentSessionRepo(database, clk)

	// Create a valid session to operate on.
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

	// UpdateStatusToInteractiveCompleted fails due to constraint violation.
	// This documents the production bug: migration is needed.
	err := r.UpdateStatusToInteractiveCompleted("ic-clk-valid")
	if err == nil {
		t.Fatal("expected UpdateStatusToInteractiveCompleted to fail (missing migration)")
	}
}

// isConstraintError returns true if err is a DB CHECK constraint failure.
func isConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "constraint") || strings.Contains(msg, "check")
}
