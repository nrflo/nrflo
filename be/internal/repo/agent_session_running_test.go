package repo

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func setupRunningTestDB(t *testing.T) (*db.DB, *AgentSessionRepo, string) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "running_test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj', 'Test Project', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	_, err = database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, phases, created_at, updated_at)
		VALUES ('proj', 'test-workflow', 'Test Workflow', 'ticket', '[]', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	wfiID := "wfi-running-test"
	_, err = database.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
		VALUES (?, 'proj', 'TKT-1', 'test-workflow', 'active', 'ticket', '{}', datetime('now'), datetime('now'))`, wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	r := NewAgentSessionRepo(database, clock.Real())
	return database, r, wfiID
}

func insertRunningSession(t *testing.T, database *db.DB, id, wfiID string, status model.AgentSessionStatus, startedAt time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	startedAtStr := startedAt.UTC().Format(time.RFC3339Nano)
	_, err := database.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, started_at, created_at, updated_at)
		VALUES (?, 'proj', 'TKT-1', ?, 'test-phase', 'test-agent', 'sonnet', ?, ?, ?, ?)`,
		id, wfiID, status, startedAtStr, now, now)
	if err != nil {
		t.Fatalf("insertRunningSession(%s): %v", id, err)
	}
}

func TestGetRunning_OnlyRunningReturned(t *testing.T) {
	database, r, wfiID := setupRunningTestDB(t)
	defer database.Close()

	now := time.Now().UTC()
	insertRunningSession(t, database, "sess-running-1", wfiID, model.AgentSessionRunning, now.Add(-2*time.Minute))
	insertRunningSession(t, database, "sess-completed-1", wfiID, model.AgentSessionCompleted, now.Add(-3*time.Minute))
	insertRunningSession(t, database, "sess-failed-1", wfiID, model.AgentSessionFailed, now.Add(-4*time.Minute))
	insertRunningSession(t, database, "sess-running-2", wfiID, model.AgentSessionRunning, now.Add(-1*time.Minute))
	insertRunningSession(t, database, "sess-timeout-1", wfiID, model.AgentSessionTimeout, now.Add(-5*time.Minute))

	sessions, err := r.GetRunning(50)
	if err != nil {
		t.Fatalf("GetRunning() error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("GetRunning() = %d sessions, want 2", len(sessions))
	}
	for _, s := range sessions {
		if s.Status != model.AgentSessionRunning {
			t.Errorf("GetRunning() returned session %s with status %s, want running", s.ID, s.Status)
		}
	}
}

func TestGetRunning_OrderedByStartedAtASC(t *testing.T) {
	database, r, wfiID := setupRunningTestDB(t)
	defer database.Close()

	now := time.Now().UTC()
	// Insert in newest-first order; expect oldest-first result (ASC).
	insertRunningSession(t, database, "sess-newest", wfiID, model.AgentSessionRunning, now.Add(-1*time.Minute))
	insertRunningSession(t, database, "sess-middle", wfiID, model.AgentSessionRunning, now.Add(-3*time.Minute))
	insertRunningSession(t, database, "sess-oldest", wfiID, model.AgentSessionRunning, now.Add(-5*time.Minute))

	sessions, err := r.GetRunning(50)
	if err != nil {
		t.Fatalf("GetRunning() error: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("GetRunning() = %d sessions, want 3", len(sessions))
	}
	if sessions[0].ID != "sess-oldest" {
		t.Errorf("sessions[0].ID = %q, want sess-oldest (ASC order)", sessions[0].ID)
	}
	if sessions[1].ID != "sess-middle" {
		t.Errorf("sessions[1].ID = %q, want sess-middle", sessions[1].ID)
	}
	if sessions[2].ID != "sess-newest" {
		t.Errorf("sessions[2].ID = %q, want sess-newest", sessions[2].ID)
	}
}

func TestGetRunning_RespectsLimit(t *testing.T) {
	database, r, wfiID := setupRunningTestDB(t)
	defer database.Close()

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		insertRunningSession(t, database, fmt.Sprintf("sess-%d", i), wfiID, model.AgentSessionRunning, now.Add(time.Duration(-i)*time.Minute))
	}

	sessions, err := r.GetRunning(3)
	if err != nil {
		t.Fatalf("GetRunning(3) error: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("GetRunning(3) = %d sessions, want 3", len(sessions))
	}
}

func TestGetRunning_EmptyTable(t *testing.T) {
	_, r, _ := setupRunningTestDB(t)

	sessions, err := r.GetRunning(50)
	if err != nil {
		t.Fatalf("GetRunning() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("GetRunning() on empty table = %d sessions, want 0", len(sessions))
	}
}

func TestGetRunning_WorkflowIDPopulated(t *testing.T) {
	database, r, wfiID := setupRunningTestDB(t)
	defer database.Close()

	insertRunningSession(t, database, "sess-1", wfiID, model.AgentSessionRunning, time.Now().UTC())

	sessions, err := r.GetRunning(50)
	if err != nil {
		t.Fatalf("GetRunning() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("GetRunning() = %d sessions, want 1", len(sessions))
	}
	if sessions[0].Workflow != "test-workflow" {
		t.Errorf("sessions[0].Workflow = %q, want %q", sessions[0].Workflow, "test-workflow")
	}
}

func TestGetRunning_AllNonRunningStatuses(t *testing.T) {
	database, r, wfiID := setupRunningTestDB(t)
	defer database.Close()

	now := time.Now().UTC()
	nonRunning := []model.AgentSessionStatus{
		model.AgentSessionCompleted,
		model.AgentSessionFailed,
		model.AgentSessionTimeout,
		model.AgentSessionContinued,
	}
	for i, status := range nonRunning {
		insertRunningSession(t, database, fmt.Sprintf("sess-%d", i), wfiID, status, now.Add(time.Duration(-i)*time.Minute))
	}

	sessions, err := r.GetRunning(50)
	if err != nil {
		t.Fatalf("GetRunning() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("GetRunning() = %d sessions, want 0 (none running)", len(sessions))
	}
}
