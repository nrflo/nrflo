package repo

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
)

func setupCleanupTestDB(t *testing.T) (*db.DB, *AgentSessionRepo, string) {
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

	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "test-agent", "layer": 0},
	})
	_, err = database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, phases, created_at, updated_at)
		VALUES ('proj', 'test-workflow', 'Test Workflow', 'ticket', ?, datetime('now'), datetime('now'))`, string(phasesJSON))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	wfiID := "wfi-cleanup-test"
	_, err = database.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, phase_order, phases, findings, created_at, updated_at)
		VALUES (?, 'proj', 'TKT-1', 'test-workflow', 'active', 'ticket', '[]', '{}', '{}', datetime('now'), datetime('now'))`, wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	repo := NewAgentSessionRepo(database)
	return database, repo, wfiID
}

func TestAgentSessionCleanupKeepLatest(t *testing.T) {
	database, repo, wfiID := setupCleanupTestDB(t)
	defer database.Close()

	// Insert 5 sessions: 3 completed, 2 running
	now := time.Now().UTC()
	sessions := []struct {
		id     string
		status model.AgentSessionStatus
		offset time.Duration
	}{
		{"sess-completed-1", model.AgentSessionCompleted, -4 * time.Minute},
		{"sess-completed-2", model.AgentSessionCompleted, -3 * time.Minute},
		{"sess-completed-3", model.AgentSessionCompleted, -2 * time.Minute},
		{"sess-running-1", model.AgentSessionRunning, -1 * time.Minute},
		{"sess-running-2", model.AgentSessionRunning, 0},
	}

	for _, s := range sessions {
		updatedAt := now.Add(s.offset).Format(time.RFC3339Nano)
		_, err := database.Exec(`
			INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
			VALUES (?, 'proj', 'TKT-1', ?, 'test-phase', 'test-agent', 'sonnet', ?, ?, ?)`,
			s.id, wfiID, s.status, updatedAt, updatedAt)
		if err != nil {
			t.Fatalf("failed to create session %s: %v", s.id, err)
		}
	}

	// Call CleanupKeepLatest(2) - keep only 2 latest completed + all running
	deleted, err := repo.CleanupKeepLatest(2)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}

	// Should delete 1 completed session (oldest one)
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Verify remaining sessions
	var count int
	err = database.QueryRow(`SELECT COUNT(*) FROM agent_sessions`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count sessions: %v", err)
	}
	// Should have 4 remaining: 2 completed + 2 running
	if count != 4 {
		t.Errorf("expected 4 remaining sessions, got %d", count)
	}

	// Verify oldest completed session was deleted
	var exists bool
	err = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_sessions WHERE id = ?)`, "sess-completed-1").Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check existence: %v", err)
	}
	if exists {
		t.Errorf("expected sess-completed-1 to be deleted")
	}

	// Verify latest 2 completed sessions remain
	for _, id := range []string{"sess-completed-2", "sess-completed-3"} {
		err = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_sessions WHERE id = ?)`, id).Scan(&exists)
		if err != nil {
			t.Fatalf("failed to check existence: %v", err)
		}
		if !exists {
			t.Errorf("expected %s to remain", id)
		}
	}

	// Verify all running sessions remain
	for _, id := range []string{"sess-running-1", "sess-running-2"} {
		err = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_sessions WHERE id = ?)`, id).Scan(&exists)
		if err != nil {
			t.Fatalf("failed to check existence: %v", err)
		}
		if !exists {
			t.Errorf("expected %s to remain (running)", id)
		}
	}
}

func TestAgentSessionCleanupKeepLatest_ZeroKeep(t *testing.T) {
	database, repo, wfiID := setupCleanupTestDB(t)
	defer database.Close()

	// Insert 3 completed sessions
	for i := 1; i <= 3; i++ {
		sessID := "sess-" + string(rune(i+'0'))
		s := &model.AgentSession{
			ID:                 sessID,
			ProjectID:          "proj",
			TicketID:           "TKT-1",
			WorkflowInstanceID: wfiID,
			Phase:              "test-phase",
			AgentType:          "test-agent",
			ModelID:            sql.NullString{String: "sonnet", Valid: true},
			Status:             model.AgentSessionCompleted,
		}
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// CleanupKeepLatest(0) should delete all non-running sessions
	deleted, err := repo.CleanupKeepLatest(0)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}

	if deleted != 3 {
		t.Errorf("expected 3 deleted, got %d", deleted)
	}

	var count int
	err = database.QueryRow(`SELECT COUNT(*) FROM agent_sessions`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count sessions: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 remaining sessions, got %d", count)
	}
}

func TestAgentSessionCleanupKeepLatest_KeepExceedsTotal(t *testing.T) {
	database, repo, wfiID := setupCleanupTestDB(t)
	defer database.Close()

	// Insert only 2 completed sessions
	for i := 1; i <= 2; i++ {
		sessID := "sess-" + string(rune(i+'0'))
		s := &model.AgentSession{
			ID:                 sessID,
			ProjectID:          "proj",
			TicketID:           "TKT-1",
			WorkflowInstanceID: wfiID,
			Phase:              "test-phase",
			AgentType:          "test-agent",
			ModelID:            sql.NullString{String: "sonnet", Valid: true},
			Status:             model.AgentSessionCompleted,
		}
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// CleanupKeepLatest(100) should delete nothing (keep > total)
	deleted, err := repo.CleanupKeepLatest(100)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}

	var count int
	err = database.QueryRow(`SELECT COUNT(*) FROM agent_sessions`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count sessions: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 remaining sessions, got %d", count)
	}
}

func TestAgentSessionCleanupKeepLatest_EmptyTable(t *testing.T) {
	database, repo, _ := setupCleanupTestDB(t)
	defer database.Close()

	// Call cleanup on empty table
	deleted, err := repo.CleanupKeepLatest(10)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("expected 0 deleted from empty table, got %d", deleted)
	}
}

func TestAgentSessionCleanupKeepLatest_OnlyRunningSessions(t *testing.T) {
	database, repo, wfiID := setupCleanupTestDB(t)
	defer database.Close()

	// Insert only running sessions
	for i := 1; i <= 3; i++ {
		sessID := "sess-running-" + string(rune(i+'0'))
		s := &model.AgentSession{
			ID:                 sessID,
			ProjectID:          "proj",
			TicketID:           "TKT-1",
			WorkflowInstanceID: wfiID,
			Phase:              "test-phase",
			AgentType:          "test-agent",
			ModelID:            sql.NullString{String: "sonnet", Valid: true},
			Status:             model.AgentSessionRunning,
		}
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
	}

	// Cleanup should not delete any running sessions
	deleted, err := repo.CleanupKeepLatest(1)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("expected 0 deleted (all running), got %d", deleted)
	}

	var count int
	err = database.QueryRow(`SELECT COUNT(*) FROM agent_sessions`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count sessions: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 remaining sessions, got %d", count)
	}
}

func TestAgentSessionCleanupKeepLatest_MixedStatuses(t *testing.T) {
	database, repo, wfiID := setupCleanupTestDB(t)
	defer database.Close()

	// Insert sessions with various non-running statuses
	now := time.Now().UTC()
	sessions := []struct {
		id     string
		status model.AgentSessionStatus
		offset time.Duration
	}{
		{"sess-completed-1", model.AgentSessionCompleted, -5 * time.Minute},
		{"sess-failed-1", model.AgentSessionFailed, -4 * time.Minute},
		{"sess-completed-2", model.AgentSessionCompleted, -3 * time.Minute},
		{"sess-timeout-1", model.AgentSessionTimeout, -2 * time.Minute},
		{"sess-continued-1", model.AgentSessionContinued, -1 * time.Minute},
	}

	for _, s := range sessions {
		updatedAt := now.Add(s.offset).Format(time.RFC3339Nano)
		_, err := database.Exec(`
			INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
			VALUES (?, 'proj', 'TKT-1', ?, 'test-phase', 'test-agent', 'sonnet', ?, ?, ?)`,
			s.id, wfiID, s.status, updatedAt, updatedAt)
		if err != nil {
			t.Fatalf("failed to create session %s: %v", s.id, err)
		}
	}

	// Call CleanupKeepLatest(2) - keep only 2 latest non-running
	deleted, err := repo.CleanupKeepLatest(2)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}

	// Should delete 3 oldest sessions (completed-1, failed-1, completed-2)
	if deleted != 3 {
		t.Errorf("expected 3 deleted, got %d", deleted)
	}

	// Verify remaining sessions (2 latest: timeout-1, continued-1)
	var count int
	err = database.QueryRow(`SELECT COUNT(*) FROM agent_sessions`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count sessions: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 remaining sessions, got %d", count)
	}

	// Verify the 2 most recent sessions remain
	for _, id := range []string{"sess-timeout-1", "sess-continued-1"} {
		var exists bool
		err = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_sessions WHERE id = ?)`, id).Scan(&exists)
		if err != nil {
			t.Fatalf("failed to check existence: %v", err)
		}
		if !exists {
			t.Errorf("expected %s to remain (most recent)", id)
		}
	}
}
