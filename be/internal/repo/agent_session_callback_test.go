package repo

import (
	"database/sql"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// setupTestDB creates a test database with required dependencies
func setupTestDB(t *testing.T) (*db.DB, *AgentSessionRepo, string) {
	t.Helper()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	// Create required FK dependencies
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj', 'Test Project', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	_, err = database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES ('proj', 'test-workflow', 'Test Workflow', 'ticket', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	wfiID := "wfi-test-123"
	_, err = database.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
		VALUES (?, 'proj', '', 'test-workflow', 'active', 'ticket', '{}', datetime('now'), datetime('now'))`, wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	repo := NewAgentSessionRepo(database, clock.Real())
	return database, repo, wfiID
}

// TestResetSessionsForCallback tests that ResetSessionsForCallback correctly
// marks sessions as callback, clears findings, and sets ended_at.
func TestResetSessionsForCallback(t *testing.T) {
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	// Create test sessions in different phases
	sessions := []*model.AgentSession{
		{
			ID:                 "sess-1",
			ProjectID:          "proj",
			TicketID:           "TKT-1",
			WorkflowInstanceID: wfiID,
			Phase:              "analyzer",
			AgentType:          "analyzer",
			Status:             model.AgentSessionCompleted,
			Findings:           sql.NullString{String: `{"key1":"value1"}`, Valid: true},
		},
		{
			ID:                 "sess-2",
			ProjectID:          "proj",
			TicketID:           "TKT-1",
			WorkflowInstanceID: wfiID,
			Phase:              "builder",
			AgentType:          "builder",
			Status:             model.AgentSessionCompleted,
			Findings:           sql.NullString{String: `{"key2":"value2"}`, Valid: true},
		},
		{
			ID:                 "sess-3",
			ProjectID:          "proj",
			TicketID:           "TKT-1",
			WorkflowInstanceID: wfiID,
			Phase:              "verifier",
			AgentType:          "verifier",
			Status:             model.AgentSessionFailed,
			Findings:           sql.NullString{String: `{"error":"test"}`, Valid: true},
		},
	}

	for _, s := range sessions {
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
	}

	// Reset sessions for analyzer and builder phases
	phases := []string{"analyzer", "builder"}
	err := repo.ResetSessionsForCallback(wfiID, phases)
	if err != nil {
		t.Fatalf("ResetSessionsForCallback failed: %v", err)
	}

	// Verify sess-1 (analyzer) was reset
	sess1, err := repo.Get("sess-1")
	if err != nil {
		t.Fatalf("failed to get sess-1: %v", err)
	}
	if sess1.Status != model.AgentSessionCallback {
		t.Errorf("expected sess-1 status=callback, got %s", sess1.Status)
	}
	if sess1.Findings.String != "{}" {
		t.Errorf("expected sess-1 findings cleared, got %s", sess1.Findings.String)
	}
	if !sess1.EndedAt.Valid {
		t.Error("expected sess-1 ended_at to be set")
	}

	// Verify sess-2 (builder) was reset
	sess2, err := repo.Get("sess-2")
	if err != nil {
		t.Fatalf("failed to get sess-2: %v", err)
	}
	if sess2.Status != model.AgentSessionCallback {
		t.Errorf("expected sess-2 status=callback, got %s", sess2.Status)
	}
	if sess2.Findings.String != "{}" {
		t.Errorf("expected sess-2 findings cleared, got %s", sess2.Findings.String)
	}
	if !sess2.EndedAt.Valid {
		t.Error("expected sess-2 ended_at to be set")
	}

	// Verify sess-3 (verifier) was NOT reset (not in phases list)
	sess3, err := repo.Get("sess-3")
	if err != nil {
		t.Fatalf("failed to get sess-3: %v", err)
	}
	if sess3.Status != model.AgentSessionFailed {
		t.Errorf("expected sess-3 status unchanged (failed), got %s", sess3.Status)
	}
	if sess3.Findings.String != `{"error":"test"}` {
		t.Errorf("expected sess-3 findings unchanged, got %s", sess3.Findings.String)
	}
}

// TestResetSessionsForCallback_ExcludesRunningAndContinued tests that
// running and continued sessions are excluded from reset.
func TestResetSessionsForCallback_ExcludesRunningAndContinued(t *testing.T) {
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	// Create sessions with various statuses
	sessions := []*model.AgentSession{
		{
			ID:                 "sess-running",
			ProjectID:          "proj",
			TicketID:           "TKT-2",
			WorkflowInstanceID: wfiID,
			Phase:              "analyzer",
			AgentType:          "analyzer",
			Status:             model.AgentSessionRunning,
			Findings:           sql.NullString{String: `{"running":"data"}`, Valid: true},
		},
		{
			ID:                 "sess-continued",
			ProjectID:          "proj",
			TicketID:           "TKT-2",
			WorkflowInstanceID: wfiID,
			Phase:              "analyzer",
			AgentType:          "analyzer",
			Status:             model.AgentSessionContinued,
			Findings:           sql.NullString{String: `{"continued":"data"}`, Valid: true},
		},
		{
			ID:                 "sess-completed",
			ProjectID:          "proj",
			TicketID:           "TKT-2",
			WorkflowInstanceID: wfiID,
			Phase:              "analyzer",
			AgentType:          "analyzer",
			Status:             model.AgentSessionCompleted,
			Findings:           sql.NullString{String: `{"completed":"data"}`, Valid: true},
		},
	}

	for _, s := range sessions {
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
	}

	// Reset all analyzer phase sessions
	err := repo.ResetSessionsForCallback(wfiID, []string{"analyzer"})
	if err != nil {
		t.Fatalf("ResetSessionsForCallback failed: %v", err)
	}

	// Verify running session was NOT reset
	running, _ := repo.Get("sess-running")
	if running.Status != model.AgentSessionRunning {
		t.Errorf("expected running session to remain running, got %s", running.Status)
	}
	if running.Findings.String != `{"running":"data"}` {
		t.Errorf("expected running session findings unchanged, got %s", running.Findings.String)
	}

	// Verify continued session was NOT reset
	continued, _ := repo.Get("sess-continued")
	if continued.Status != model.AgentSessionContinued {
		t.Errorf("expected continued session to remain continued, got %s", continued.Status)
	}
	if continued.Findings.String != `{"continued":"data"}` {
		t.Errorf("expected continued session findings unchanged, got %s", continued.Findings.String)
	}

	// Verify completed session WAS reset
	completed, _ := repo.Get("sess-completed")
	if completed.Status != model.AgentSessionCallback {
		t.Errorf("expected completed session status=callback, got %s", completed.Status)
	}
	if completed.Findings.String != "{}" {
		t.Errorf("expected completed session findings cleared, got %s", completed.Findings.String)
	}
}

// TestResetSessionsForCallback_EmptyPhases tests that empty phases list is a no-op.
func TestResetSessionsForCallback_EmptyPhases(t *testing.T) {
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	// Create a session
	session := &model.AgentSession{
		ID:                 "sess-empty",
		ProjectID:          "proj",
		TicketID:           "TKT-3",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionCompleted,
		Findings:           sql.NullString{String: `{"data":"test"}`, Valid: true},
	}
	repo.Create(session)

	// Call with empty phases list
	err := repo.ResetSessionsForCallback(wfiID, []string{})
	if err != nil {
		t.Fatalf("ResetSessionsForCallback with empty phases failed: %v", err)
	}

	// Verify session was NOT modified
	updated, _ := repo.Get("sess-empty")
	if updated.Status != model.AgentSessionCompleted {
		t.Errorf("expected status unchanged, got %s", updated.Status)
	}
	if updated.Findings.String != `{"data":"test"}` {
		t.Errorf("expected findings unchanged, got %s", updated.Findings.String)
	}
}
