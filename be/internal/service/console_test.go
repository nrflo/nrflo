package service

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func setupConsoleServiceTestEnv(t *testing.T) (*db.Pool, *ConsoleService, *clock.TestClock) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "console_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?,?,?,?,?)`,
		"proj1", "Test Project", "/tmp", now, now,
	); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	clk := clock.NewTest(time.Now().UTC())
	svc := NewConsoleService(pool, clk)
	return pool, svc, clk
}

// backdateSessionUpdatedAt directly rewrites updated_at, simulating an idle session
// without sleeping in the test.
func backdateSessionUpdatedAt(t *testing.T, pool *db.Pool, sessionID string, ts time.Time) {
	t.Helper()
	if _, err := pool.Exec(`UPDATE agent_sessions SET updated_at = ? WHERE id = ?`,
		ts.UTC().Format(time.RFC3339Nano), sessionID); err != nil {
		t.Fatalf("backdate updated_at: %v", err)
	}
}

func TestConsoleCreateSession_RowShape(t *testing.T) {
	t.Parallel()
	pool, svc, _ := setupConsoleServiceTestEnv(t)

	sessionID, token, err := svc.CreateSession("proj1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sessionID == "" || token == "" {
		t.Fatalf("CreateSession returned empty id/token: %q %q", sessionID, token)
	}

	var kind, status, phase, nodeID, agentType, ticketID string
	var wfi sql.NullString
	var spawnToken sql.NullString
	err = pool.QueryRow(`SELECT kind, status, phase, node_id, agent_type, ticket_id, workflow_instance_id, spawn_token
		FROM agent_sessions WHERE id = ?`, sessionID).Scan(
		&kind, &status, &phase, &nodeID, &agentType, &ticketID, &wfi, &spawnToken)
	if err != nil {
		t.Fatalf("query session row: %v", err)
	}

	if kind != model.AgentSessionKindConsole {
		t.Errorf("kind = %q, want %q", kind, model.AgentSessionKindConsole)
	}
	if status != string(model.AgentSessionUserInteractive) {
		t.Errorf("status = %q, want %q", status, model.AgentSessionUserInteractive)
	}
	if agentType != "console" {
		t.Errorf("agent_type = %q, want console", agentType)
	}
	if ticketID != "" {
		t.Errorf("ticket_id = %q, want empty", ticketID)
	}
	if wfi.Valid {
		t.Errorf("workflow_instance_id = %+v, want NULL", wfi)
	}
	if !spawnToken.Valid || spawnToken.String != token {
		t.Errorf("spawn_token = %+v, want %q", spawnToken, token)
	}
}

func TestConsoleCreateSession_TokensDistinctPerCall(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupConsoleServiceTestEnv(t)

	_, tok1, err := svc.CreateSession("proj1")
	if err != nil {
		t.Fatalf("CreateSession 1: %v", err)
	}
	_, tok2, err := svc.CreateSession("proj1")
	if err != nil {
		t.Fatalf("CreateSession 2: %v", err)
	}
	if tok1 == tok2 {
		t.Errorf("CreateSession returned duplicate tokens: %q", tok1)
	}
}

func TestConsoleCreateSession_UnknownProjectErrors(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupConsoleServiceTestEnv(t)

	_, _, err := svc.CreateSession("no-such-project")
	if err == nil {
		t.Fatalf("expected error for unknown project")
	}
}

func TestConsoleCloseSession_FlipsStatusAndIsIdempotent(t *testing.T) {
	t.Parallel()
	pool, svc, _ := setupConsoleServiceTestEnv(t)

	sessionID, _, err := svc.CreateSession("proj1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := svc.CloseSession(sessionID); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	var status string
	var endedAt sql.NullString
	if err := pool.QueryRow(`SELECT status, ended_at FROM agent_sessions WHERE id = ?`, sessionID).
		Scan(&status, &endedAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != string(model.AgentSessionInteractiveCompleted) {
		t.Errorf("status = %q, want interactive_completed", status)
	}
	if !endedAt.Valid || endedAt.String == "" {
		t.Errorf("ended_at not set")
	}

	if err := svc.CloseSession(sessionID); err != nil {
		t.Fatalf("second CloseSession should be idempotent, got err: %v", err)
	}
}

func TestConsoleCloseSession_WorkflowAgentSessionErrors(t *testing.T) {
	t.Parallel()
	pool, svc, _ := setupConsoleServiceTestEnv(t)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES ('proj1', 'wf1', '', 'ticket', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES ('wfi-1', 'proj1', '', 'wf1', 'active', 'ticket', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert wfi: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, kind, created_at, updated_at)
		VALUES ('wf-agent-1', 'proj1', '', 'wfi-1', 'p', 'a', 'sonnet', 'user_interactive', 'workflow_agent', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert workflow agent session: %v", err)
	}

	err := svc.CloseSession("wf-agent-1")
	if !errors.Is(err, ErrConsoleSessionNotFound) {
		t.Fatalf("CloseSession(workflow_agent id) err = %v, want ErrConsoleSessionNotFound", err)
	}

	var status string
	if err := pool.QueryRow(`SELECT status FROM agent_sessions WHERE id = 'wf-agent-1'`).Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != string(model.AgentSessionUserInteractive) {
		t.Errorf("workflow_agent status = %q, want unchanged user_interactive", status)
	}
}

func TestConsoleCloseSession_UnknownIDErrors(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupConsoleServiceTestEnv(t)

	err := svc.CloseSession("does-not-exist")
	if !errors.Is(err, ErrConsoleSessionNotFound) {
		t.Fatalf("CloseSession err = %v, want ErrConsoleSessionNotFound", err)
	}
}
