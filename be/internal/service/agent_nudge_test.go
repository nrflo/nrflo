package service

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// setupAgentNudgeEnv creates a minimal DB + AgentService for IncrementNudgeCount tests.
// Returns (svc, sessionID, cleanup).
func setupAgentNudgeEnv(t *testing.T) (*AgentService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "agent_nudge_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Seed minimum required rows (FK not enforced in SQLite by default).
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, stmt := range []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'T', '` + now + `', '` + now + `')`,
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		 VALUES ('p1', 'wf1', '', 'ticket', '` + now + `', '` + now + `')`,
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, created_at, updated_at)
		 VALUES ('wfi-1', 'p1', 'TKT-1', 'wf1', 'ticket', 'active', '{}', '` + now + `', '` + now + `')`,
	} {
		if _, err := pool.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	const sessionID = "sess-nudge-svc-1"
	_, err = pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status,
			 nudge_count, config, started_at, created_at, updated_at)
		VALUES (?, 'p1', 'TKT-1', 'wfi-1', 'test', 'implementor', 'running',
		        0, '', ?, ?, ?)`,
		sessionID, now, now, now,
	)
	if err != nil {
		t.Fatalf("insert agent_session: %v", err)
	}

	svc := NewAgentService(pool, clock.Real())
	return svc, sessionID
}

// TestIncrementNudgeCount_FirstCallReturnsOne verifies the first call returns 1.
func TestIncrementNudgeCount_FirstCallReturnsOne(t *testing.T) {
	t.Parallel()
	svc, sessionID := setupAgentNudgeEnv(t)

	count, err := svc.IncrementNudgeCount(sessionID)
	if err != nil {
		t.Fatalf("IncrementNudgeCount: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (first increment)", count)
	}
}

// TestIncrementNudgeCount_SecondCallReturnsTwo verifies the second call returns 2.
func TestIncrementNudgeCount_SecondCallReturnsTwo(t *testing.T) {
	t.Parallel()
	svc, sessionID := setupAgentNudgeEnv(t)

	if _, err := svc.IncrementNudgeCount(sessionID); err != nil {
		t.Fatalf("first IncrementNudgeCount: %v", err)
	}
	count, err := svc.IncrementNudgeCount(sessionID)
	if err != nil {
		t.Fatalf("second IncrementNudgeCount: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (second increment)", count)
	}
}

// TestIncrementNudgeCount_DBReflectsCount verifies nudge_count in the DB reflects the
// incremented value after multiple calls.
func TestIncrementNudgeCount_DBReflectsCount(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "reflect_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, stmt := range []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p2', 'T', '` + now + `', '` + now + `')`,
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		 VALUES ('p2', 'wf2', '', 'ticket', '` + now + `', '` + now + `')`,
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, created_at, updated_at)
		 VALUES ('wfi-2', 'p2', 'TKT-2', 'wf2', 'ticket', 'active', '{}', '` + now + `', '` + now + `')`,
	} {
		if _, err := pool.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	const sessionID = "sess-reflect"
	_, err = pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status,
			 nudge_count, config, created_at, updated_at)
		VALUES (?, 'p2', 'TKT-2', 'wfi-2', 'test', 'qa-verifier', 'running',
		        0, '', ?, ?)`,
		sessionID, now, now,
	)
	if err != nil {
		t.Fatalf("insert agent_session: %v", err)
	}

	svc := NewAgentService(pool, clock.Real())

	for i := 0; i < 4; i++ {
		if _, err := svc.IncrementNudgeCount(sessionID); err != nil {
			t.Fatalf("increment %d: %v", i+1, err)
		}
	}

	var dbCount int
	if err := pool.QueryRow(`SELECT nudge_count FROM agent_sessions WHERE id = ?`, sessionID).Scan(&dbCount); err != nil {
		t.Fatalf("SELECT nudge_count: %v", err)
	}
	if dbCount != 4 {
		t.Errorf("DB nudge_count = %d, want 4 after 4 increments", dbCount)
	}
}

// TestIncrementNudgeCount_UnknownSession_ReturnsError verifies an unknown session ID
// causes an error (sql.ErrNoRows from the SELECT after the no-op UPDATE).
func TestIncrementNudgeCount_UnknownSession_ReturnsError(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "unknown_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	svc := NewAgentService(pool, clock.Real())

	_, err = svc.IncrementNudgeCount("no-such-session")
	if err == nil {
		t.Error("IncrementNudgeCount for unknown session should return error, got nil")
	}
}

// TestIncrementNudgeCount_UpdatedAt_SetFromClock verifies updated_at is refreshed by the clock.
func TestIncrementNudgeCount_UpdatedAt_SetFromClock(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "ts_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	past := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	pastStr := past.UTC().Format(time.RFC3339Nano)
	for _, stmt := range []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p3', 'T', '` + pastStr + `', '` + pastStr + `')`,
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		 VALUES ('p3', 'wf3', '', 'ticket', '` + pastStr + `', '` + pastStr + `')`,
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, created_at, updated_at)
		 VALUES ('wfi-3', 'p3', 'TKT-3', 'wf3', 'ticket', 'active', '{}', '` + pastStr + `', '` + pastStr + `')`,
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status,
			nudge_count, config, created_at, updated_at)
		 VALUES ('sess-ts', 'p3', 'TKT-3', 'wfi-3', 'test', 'implementor', 'running',
		         0, '', '` + pastStr + `', '` + pastStr + `')`,
	} {
		if _, err := pool.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	fixed := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixed)
	svc := NewAgentService(pool, clk)

	if _, err := svc.IncrementNudgeCount("sess-ts"); err != nil {
		t.Fatalf("IncrementNudgeCount: %v", err)
	}

	var updatedAt sql.NullString
	if err := pool.QueryRow(`SELECT updated_at FROM agent_sessions WHERE id = 'sess-ts'`).Scan(&updatedAt); err != nil {
		t.Fatalf("SELECT updated_at: %v", err)
	}
	want := fixed.UTC().Format(time.RFC3339Nano)
	if !updatedAt.Valid || updatedAt.String != want {
		t.Errorf("updated_at = %v, want %q", updatedAt, want)
	}
}
