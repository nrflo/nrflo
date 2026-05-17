package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func setupTokenTestDB(t *testing.T) (*db.DB, *AgentSessionRepo, string) {
	t.Helper()
	database := newTestDB(t)
	if _, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("project: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES ('proj', 'wf', '', 'ticket', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("workflow: %v", err)
	}
	wfiID := "wfi-token-test"
	if _, err := database.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES (?, 'proj', 'TKT-1', 'wf', 'active', 'ticket', datetime('now'), datetime('now'))`, wfiID); err != nil {
		t.Fatalf("wfi: %v", err)
	}
	return database, NewAgentSessionRepo(database, clock.Real()), wfiID
}

func insertSessionWithToken(t *testing.T, database *db.DB, id, wfiID, token string, status model.AgentSessionStatus) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := database.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, spawn_token, created_at, updated_at)
		VALUES (?, 'proj', 'TKT-1', ?, 'p', 'a', 'sonnet', ?, ?, ?, ?)`,
		id, wfiID, status, token, now, now)
	if err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

func TestGetByToken_RunningSession(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	insertSessionWithToken(t, database, "sess-1", wfiID, "tok-running", model.AgentSessionRunning)

	got, err := r.GetByToken("tok-running")
	if err != nil {
		t.Fatalf("GetByToken err: %v", err)
	}
	if got == nil || got.ID != "sess-1" {
		t.Fatalf("GetByToken = %+v, want sess-1", got)
	}
}

func TestGetByToken_UserInteractive(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	insertSessionWithToken(t, database, "sess-ui", wfiID, "tok-ui", model.AgentSessionUserInteractive)

	got, err := r.GetByToken("tok-ui")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil || got.ID != "sess-ui" {
		t.Fatalf("user_interactive token rejected, got %+v", got)
	}
}

func TestGetByToken_TerminalStatusRejects(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	for _, st := range []model.AgentSessionStatus{
		model.AgentSessionCompleted,
		model.AgentSessionFailed,
		model.AgentSessionTimeout,
		model.AgentSessionContinued,
		model.AgentSessionInteractiveCompleted,
		model.AgentSessionSkipped,
	} {
		token := "tok-" + string(st)
		insertSessionWithToken(t, database, "sess-"+string(st), wfiID, token, st)
		got, err := r.GetByToken(token)
		if err != nil {
			t.Fatalf("status %s: err %v", st, err)
		}
		if got != nil {
			t.Errorf("status %s: GetByToken = %+v, want nil", st, got)
		}
	}
}

func TestGetByToken_UnknownToken(t *testing.T) {
	t.Parallel()
	database, r, _ := setupTokenTestDB(t)
	defer database.Close()
	got, err := r.GetByToken("does-not-exist")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestGetByToken_EmptyToken(t *testing.T) {
	t.Parallel()
	database, r, _ := setupTokenTestDB(t)
	defer database.Close()
	got, err := r.GetByToken("")
	if err != nil || got != nil {
		t.Errorf("GetByToken(\"\") = %+v, %v; want nil, nil", got, err)
	}
}

func TestUpdateSpawnToken(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	insertSessionWithToken(t, database, "sess-x", wfiID, "old-token", model.AgentSessionRunning)
	if err := r.UpdateSpawnToken("sess-x", "new-token"); err != nil {
		t.Fatalf("UpdateSpawnToken err: %v", err)
	}
	got, _ := r.GetByToken("new-token")
	if got == nil || got.ID != "sess-x" {
		t.Fatalf("after update, GetByToken(new) = %+v", got)
	}
	old, _ := r.GetByToken("old-token")
	if old != nil {
		t.Errorf("old token still valid: %+v", old)
	}
}

func TestUpdateSpawnToken_NotFound(t *testing.T) {
	t.Parallel()
	database, r, _ := setupTokenTestDB(t)
	defer database.Close()
	if err := r.UpdateSpawnToken("nope", "tok"); err == nil {
		t.Errorf("expected error for missing session")
	}
}

func TestCreate_PersistsSpawnToken(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	sess := &model.AgentSession{
		ID:                 "sess-create",
		ProjectID:          "proj",
		TicketID:           "TKT-1",
		WorkflowInstanceID: wfiID,
		Phase:              "p",
		AgentType:          "a",
		Status:             model.AgentSessionRunning,
	}
	sess.SpawnToken.String = "tok-from-create"
	sess.SpawnToken.Valid = true
	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := r.GetByToken("tok-from-create")
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got == nil || got.ID != "sess-create" {
		t.Fatalf("GetByToken = %+v", got)
	}
}
