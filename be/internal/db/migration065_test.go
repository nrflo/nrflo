package db

import (
	"path/filepath"
	"testing"
)

// TestMigration065_NudgeCountColumnExists verifies that migration 000065 adds the
// nudge_count column to agent_sessions with INTEGER type and NOT NULL DEFAULT 0.
func TestMigration065_NudgeCountColumnExists(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Verify the column is queryable and returns the default value.
	// Insert a minimal agent_sessions row and read back nudge_count.
	_, err = pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'Test', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	_, err = pool.Exec(
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		 VALUES ('p1', 'wf1', '', 'ticket', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	_, err = pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, created_at, updated_at)
		 VALUES ('wfi-1', 'p1', 'TKT-1', 'wf1', 'ticket', 'active', '{}', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert workflow_instance: %v", err)
	}
	_, err = pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status,
			 nudge_count, config, created_at, updated_at)
		VALUES ('sess-1', 'p1', 'TKT-1', 'wfi-1', 'test', 'implementor', 'running',
		        0, '', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert agent_session: %v", err)
	}

	var nudgeCount int
	err = pool.QueryRow(`SELECT nudge_count FROM agent_sessions WHERE id = 'sess-1'`).Scan(&nudgeCount)
	if err != nil {
		t.Fatalf("SELECT nudge_count: %v (column may be missing)", err)
	}
	if nudgeCount != 0 {
		t.Errorf("nudge_count = %d, want 0 (default)", nudgeCount)
	}
}

// TestMigration065_NudgeCountDefaultZero verifies that existing rows (inserted without
// specifying nudge_count) default to 0 as required by the migration.
func TestMigration065_NudgeCountDefaultZero(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Set up dependencies.
	for _, stmt := range []string{
		`INSERT INTO projects (id, name, created_at, updated_at)
		 VALUES ('p2', 'T', datetime('now'), datetime('now'))`,
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		 VALUES ('p2', 'wf2', '', 'ticket', datetime('now'), datetime('now'))`,
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, created_at, updated_at)
		 VALUES ('wfi-2', 'p2', 'TKT-2', 'wf2', 'ticket', 'active', '{}', datetime('now'), datetime('now'))`,
	} {
		if _, err := pool.Exec(stmt); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	// Insert using only required columns, omitting nudge_count to test DEFAULT.
	_, err = pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status,
			 config, created_at, updated_at)
		VALUES ('sess-def', 'p2', 'TKT-2', 'wfi-2', 'test', 'setup-analyzer', 'running',
		        '', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert agent_session (no nudge_count): %v", err)
	}

	var nudgeCount int
	err = pool.QueryRow(`SELECT nudge_count FROM agent_sessions WHERE id = 'sess-def'`).Scan(&nudgeCount)
	if err != nil {
		t.Fatalf("SELECT nudge_count: %v", err)
	}
	if nudgeCount != 0 {
		t.Errorf("nudge_count = %d, want 0 (NOT NULL DEFAULT 0)", nudgeCount)
	}
}

// TestMigration065_NudgeCountIsUpdatable verifies nudge_count can be updated via SQL.
func TestMigration065_NudgeCountIsUpdatable(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, stmt := range []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p3', 'T', datetime('now'), datetime('now'))`,
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		 VALUES ('p3', 'wf3', '', 'ticket', datetime('now'), datetime('now'))`,
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, created_at, updated_at)
		 VALUES ('wfi-3', 'p3', 'TKT-3', 'wf3', 'ticket', 'active', '{}', datetime('now'), datetime('now'))`,
		`INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, config, created_at, updated_at)
		 VALUES ('sess-upd', 'p3', 'TKT-3', 'wfi-3', 'test', 'implementor', 'running', '', datetime('now'), datetime('now'))`,
	} {
		if _, err := pool.Exec(stmt); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	// Increment nudge_count three times.
	for i := 0; i < 3; i++ {
		if _, err := pool.Exec(`UPDATE agent_sessions SET nudge_count = nudge_count + 1 WHERE id = 'sess-upd'`); err != nil {
			t.Fatalf("increment %d: %v", i+1, err)
		}
	}

	var nudgeCount int
	if err := pool.QueryRow(`SELECT nudge_count FROM agent_sessions WHERE id = 'sess-upd'`).Scan(&nudgeCount); err != nil {
		t.Fatalf("SELECT nudge_count: %v", err)
	}
	if nudgeCount != 3 {
		t.Errorf("nudge_count = %d, want 3 after 3 increments", nudgeCount)
	}
}
