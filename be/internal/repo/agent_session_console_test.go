package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// setupConsoleTestDB creates a DB with one project and returns a repo + insert helper.
func setupConsoleTestDB(t *testing.T) (*db.DB, *AgentSessionRepo) {
	t.Helper()
	database := newTestDB(t)
	if _, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("project: %v", err)
	}
	return database, NewAgentSessionRepo(database, clock.Real())
}

func insertConsoleSession(t *testing.T, database *db.DB, id, token string, status model.AgentSessionStatus, updatedAt time.Time) {
	t.Helper()
	updated := updatedAt.UTC().Format(time.RFC3339Nano)
	_, err := database.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, kind, spawn_token, created_at, updated_at)
		VALUES (?, 'proj', '', NULL, 'console', 'console', 'sonnet-5', ?, 'console', ?, ?, ?)`,
		id, status, token, updated, updated)
	if err != nil {
		t.Fatalf("insert console session %s: %v", id, err)
	}
}

func insertWorkflowAgentSession(t *testing.T, database *db.DB, id string, status model.AgentSessionStatus, updatedAt time.Time) {
	t.Helper()
	updated := updatedAt.UTC().Format(time.RFC3339Nano)
	if _, err := database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES ('proj', 'wf', '', 'ticket', datetime('now'), datetime('now'))
		ON CONFLICT(project_id, id) DO NOTHING`); err != nil {
		t.Fatalf("workflow: %v", err)
	}
	wfiID := "wfi-" + id
	if _, err := database.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES (?, 'proj', '', 'wf', 'active', 'ticket', datetime('now'), datetime('now'))`, wfiID); err != nil {
		t.Fatalf("wfi: %v", err)
	}
	_, err := database.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, kind, spawn_token, pid, created_at, updated_at)
		VALUES (?, 'proj', '', ?, 'p', 'a', 'sonnet-5', ?, 'workflow_agent', 'tok-wf-'||?, 123, ?, ?)`,
		id, wfiID, status, id, updated, updated)
	if err != nil {
		t.Fatalf("insert workflow agent session %s: %v", id, err)
	}
}

func TestConsoleSession_GetByTokenAndClose(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	insertConsoleSession(t, database, "console-1", "tok-console-1", model.AgentSessionUserInteractive, time.Now().UTC())

	got, err := r.GetByToken("tok-console-1")
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got == nil || got.ID != "console-1" {
		t.Fatalf("GetByToken = %+v, want console-1", got)
	}

	n, err := r.CloseConsole("console-1")
	if err != nil {
		t.Fatalf("CloseConsole: %v", err)
	}
	if n != 1 {
		t.Fatalf("CloseConsole rows = %d, want 1", n)
	}

	closed, err := r.GetConsole("console-1")
	if err != nil {
		t.Fatalf("GetConsole: %v", err)
	}
	if closed == nil {
		t.Fatalf("GetConsole after close = nil, want row")
	}
	if closed.Status != model.AgentSessionInteractiveCompleted {
		t.Errorf("Status = %q, want interactive_completed", closed.Status)
	}
	if !closed.EndedAt.Valid || closed.EndedAt.String == "" {
		t.Errorf("EndedAt not set after close")
	}

	after, err := r.GetByToken("tok-console-1")
	if err != nil {
		t.Fatalf("GetByToken after close: %v", err)
	}
	if after != nil {
		t.Errorf("GetByToken after close = %+v, want nil", after)
	}
}

func TestConsoleSession_CloseConsole_Idempotent(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	insertConsoleSession(t, database, "console-2", "tok-console-2", model.AgentSessionUserInteractive, time.Now().UTC())

	n1, err := r.CloseConsole("console-2")
	if err != nil {
		t.Fatalf("first CloseConsole: %v", err)
	}
	if n1 != 1 {
		t.Fatalf("first CloseConsole rows = %d, want 1", n1)
	}

	n2, err := r.CloseConsole("console-2")
	if err != nil {
		t.Fatalf("second CloseConsole: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second CloseConsole rows = %d, want 0", n2)
	}
}

func TestConsoleSession_CloseConsole_UnknownID(t *testing.T) {
	t.Parallel()
	_, r := setupConsoleTestDB(t)

	n, err := r.CloseConsole("nope")
	if err != nil {
		t.Fatalf("CloseConsole: %v", err)
	}
	if n != 0 {
		t.Errorf("CloseConsole rows = %d, want 0", n)
	}
}

func TestConsoleSession_CloseConsole_KindGuard_LeavesWorkflowAgentUntouched(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	insertWorkflowAgentSession(t, database, "wf-agent-1", model.AgentSessionUserInteractive, time.Now().UTC())

	n, err := r.CloseConsole("wf-agent-1")
	if err != nil {
		t.Fatalf("CloseConsole: %v", err)
	}
	if n != 0 {
		t.Fatalf("CloseConsole rows = %d, want 0 for workflow_agent kind", n)
	}

	got, err := r.GetByToken("tok-wf-wf-agent-1")
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got == nil {
		t.Errorf("workflow_agent session token was invalidated, want untouched")
	}
	if got != nil && got.Status != model.AgentSessionUserInteractive {
		t.Errorf("workflow_agent session Status = %q, want unchanged user_interactive", got.Status)
	}
}

func TestConsoleSession_GetConsole_RejectsNonConsoleKind(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	insertWorkflowAgentSession(t, database, "wf-agent-2", model.AgentSessionRunning, time.Now().UTC())

	got, err := r.GetConsole("wf-agent-2")
	if err != nil {
		t.Fatalf("GetConsole: %v", err)
	}
	if got != nil {
		t.Errorf("GetConsole(workflow_agent id) = %+v, want nil", got)
	}
}

func TestExpireIdleConsoles(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)

	now := time.Now().UTC()
	cutoff := now.Add(-1 * time.Hour)

	insertConsoleSession(t, database, "stale-console", "tok-stale", model.AgentSessionUserInteractive, cutoff.Add(-time.Minute))
	insertConsoleSession(t, database, "fresh-console", "tok-fresh", model.AgentSessionUserInteractive, cutoff.Add(time.Minute))
	insertConsoleSession(t, database, "closed-console", "tok-closed", model.AgentSessionInteractiveCompleted, cutoff.Add(-time.Minute))
	insertWorkflowAgentSession(t, database, "stale-wf-agent", model.AgentSessionUserInteractive, cutoff.Add(-time.Minute))

	n, err := r.ExpireIdleConsoles(cutoff.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("ExpireIdleConsoles: %v", err)
	}
	if n != 1 {
		t.Fatalf("ExpireIdleConsoles rows = %d, want 1 (only stale-console)", n)
	}

	stale, err := r.GetConsole("stale-console")
	if err != nil {
		t.Fatalf("GetConsole(stale): %v", err)
	}
	if stale == nil || stale.Status != model.AgentSessionInteractiveCompleted {
		t.Errorf("stale-console status = %+v, want interactive_completed", stale)
	}

	fresh, err := r.GetByToken("tok-fresh")
	if err != nil {
		t.Fatalf("GetByToken(fresh): %v", err)
	}
	if fresh == nil {
		t.Errorf("fresh-console token invalidated, want untouched")
	}

	staleWfAgent, err := r.GetByToken("tok-wf-stale-wf-agent")
	if err != nil {
		t.Fatalf("GetByToken(stale wf agent): %v", err)
	}
	if staleWfAgent == nil {
		t.Errorf("stale workflow_agent session was expired by ExpireIdleConsoles, want untouched")
	}
}

func TestConsoleSessions_ExcludedFromRunningQueries(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	insertConsoleSession(t, database, "console-x", "tok-x", model.AgentSessionUserInteractive, time.Now().UTC())

	count, err := r.CountRunning()
	if err != nil {
		t.Fatalf("CountRunning: %v", err)
	}
	if count != 0 {
		t.Errorf("CountRunning = %d, want 0 (console rows are user_interactive, not running)", count)
	}

	running, err := r.GetRunning(10)
	if err != nil {
		t.Fatalf("GetRunning: %v", err)
	}
	if len(running) != 0 {
		t.Errorf("GetRunning = %d rows, want 0 (console has no workflow_instance to JOIN)", len(running))
	}

	recent, err := r.GetRecent(10)
	if err != nil {
		t.Fatalf("GetRecent: %v", err)
	}
	for _, s := range recent {
		if s.ID == "console-x" {
			t.Errorf("GetRecent included console row, want excluded (no workflow_instance JOIN)")
		}
	}

	live, err := r.ListLiveByProject("proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	for _, s := range live {
		if s.SessionID == "console-x" {
			t.Errorf("ListLiveByProject included console row, want excluded")
		}
	}
}
