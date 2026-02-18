package spawner

import (
	"path/filepath"
	"testing"

	"be/internal/db"
)

func setupTestDB(t *testing.T) *db.Pool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open test pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Seed parent rows for FK constraints
	now := "2025-01-01T00:00:00Z"
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj', 'Test', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, phases, created_at, updated_at) VALUES ('feature', 'proj', '[]', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO tickets (id, project_id, title, created_at, updated_at, created_by) VALUES ('T-1', 'proj', 'Test', ?, ?, 'test')`, now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, current_phase, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-1', 'proj', 'T-1', 'feature', 'active', '', '[]', '{}', '{}', 0, ?, ?)`, now, now)

	return pool
}

func mustExec(t *testing.T, pool *db.Pool, query string, args ...interface{}) {
	t.Helper()
	if _, err := pool.Exec(query, args...); err != nil {
		t.Fatalf("mustExec failed: %v\n  query: %s", err, query)
	}
}

func insertSession(t *testing.T, pool *db.Pool, id string, contextLeft int) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, context_left, created_at, updated_at)
		VALUES (?, 'proj', 'T-1', 'wfi-1', 'phase1', 'analyzer', 'running', ?, datetime('now'), datetime('now'))`,
		id, contextLeft)
}

func TestReadContextLeftFromDB_UpdatesProcs(t *testing.T) {
	pool := setupTestDB(t)
	insertSession(t, pool, "sess-1", 60)
	insertSession(t, pool, "sess-2", 30)

	procs := []*processInfo{
		{sessionID: "sess-1", contextLeft: 0},
		{sessionID: "sess-2", contextLeft: 0},
	}

	readContextLeftFromDB(pool, procs)

	if procs[0].contextLeft != 60 {
		t.Errorf("sess-1 contextLeft = %d, want 60", procs[0].contextLeft)
	}
	if procs[1].contextLeft != 30 {
		t.Errorf("sess-2 contextLeft = %d, want 30", procs[1].contextLeft)
	}
}

func TestReadContextLeftFromDB_NilPool(t *testing.T) {
	proc := &processInfo{sessionID: "sess-1", contextLeft: 75}
	readContextLeftFromDB(nil, []*processInfo{proc})

	if proc.contextLeft != 75 {
		t.Errorf("contextLeft = %d, want 75 (unchanged)", proc.contextLeft)
	}
}

func TestReadContextLeftFromDB_EmptyProcs(t *testing.T) {
	pool := setupTestDB(t)
	readContextLeftFromDB(pool, nil)
	readContextLeftFromDB(pool, []*processInfo{})
}

func TestReadContextLeftFromDB_SessionNotInDB(t *testing.T) {
	pool := setupTestDB(t)

	proc := &processInfo{sessionID: "nonexistent", contextLeft: 50}
	readContextLeftFromDB(pool, []*processInfo{proc})

	if proc.contextLeft != 50 {
		t.Errorf("contextLeft = %d, want 50 (unchanged)", proc.contextLeft)
	}
}
