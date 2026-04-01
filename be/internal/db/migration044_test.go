package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMigration044_ConfigTableHasCompositePK verifies the config table schema
// has project_id as part of the primary key.
func TestMigration044_ConfigTableHasCompositePK(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Verify we can insert a row with explicit project_id.
	_, err = pool.Exec("INSERT INTO config (project_id, key, value) VALUES ('proj1', 'mykey', 'myval')")
	if err != nil {
		t.Fatalf("insert with project_id: %v", err)
	}

	// Duplicate PK must fail.
	_, err = pool.Exec("INSERT INTO config (project_id, key, value) VALUES ('proj1', 'mykey', 'other')")
	if err == nil {
		t.Fatal("expected UNIQUE constraint error on duplicate (project_id,key), got nil")
	}
	if !strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") && !strings.Contains(err.Error(), "constraint") {
		t.Errorf("expected UNIQUE constraint error, got: %v", err)
	}
}

// TestMigration044_GlobalConfigRowsPreserved verifies that existing global config
// rows (project_id='') survive the migration intact.
func TestMigration044_GlobalConfigRowsPreserved(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Insert a global config entry (as migration preserves existing rows with project_id='').
	if err := pool.SetConfig("low_consumption_mode", "true"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	// Verify it's readable as a global config.
	val, err := pool.GetConfig("low_consumption_mode")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "true" {
		t.Errorf("GetConfig(%q) = %q, want %q", "low_consumption_mode", val, "true")
	}

	// Verify it lives at project_id=''.
	var projectID string
	err = pool.QueryRow("SELECT project_id FROM config WHERE key = 'low_consumption_mode'").Scan(&projectID)
	if err != nil {
		t.Fatalf("query project_id: %v", err)
	}
	if projectID != "" {
		t.Errorf("project_id = %q, want empty string", projectID)
	}
}

// TestMigration044_ProjectIDColumnExists verifies project_id column exists in config.
func TestMigration044_ProjectIDColumnExists(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// If project_id didn't exist this would fail.
	var projectID string
	err = pool.QueryRow("SELECT project_id FROM config LIMIT 1").Scan(&projectID)
	// ErrNoRows is fine (empty table), any other error means column is missing.
	if err != nil && err.Error() != "sql: no rows in result set" {
		t.Fatalf("project_id column missing or unreadable: %v", err)
	}
}

// TestMigration044_AgentSessionsConfigColumn verifies the config column
// exists in agent_sessions and defaults to empty string.
func TestMigration044_AgentSessionsConfigColumn(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Insert a minimal project and workflow_instance so we can create a session.
	_, err = pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'P', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	_, err = pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, phases, created_at, updated_at) VALUES ('p1', 'wf1', 'W', 'ticket', '[]', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	_, err = pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at) VALUES ('wfi1', 'p1', 't1', 'wf1', 'active', 'ticket', '{}', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert workflow_instance: %v", err)
	}

	// Insert a session WITHOUT specifying config — should get the DEFAULT ''.
	_, err = pool.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES ('sess1', 'p1', 't1', 'wfi1', 'phase0', 'test-agent', 'sonnet', 'running', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert session without config: %v", err)
	}

	var configVal string
	err = pool.QueryRow("SELECT config FROM agent_sessions WHERE id = 'sess1'").Scan(&configVal)
	if err != nil {
		t.Fatalf("SELECT config: %v", err)
	}
	if configVal != "" {
		t.Errorf("config default = %q, want empty string", configVal)
	}
}

// TestMigration044_SameKeyDifferentProjectsAllowed verifies that two projects
// can store the same key independently.
func TestMigration044_SameKeyDifferentProjectsAllowed(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := pool.SetProjectConfig("projA", "hook", "valA"); err != nil {
		t.Fatalf("SetProjectConfig projA: %v", err)
	}
	if err := pool.SetProjectConfig("projB", "hook", "valB"); err != nil {
		t.Fatalf("SetProjectConfig projB: %v", err)
	}

	valA, err := pool.GetProjectConfig("projA", "hook")
	if err != nil {
		t.Fatalf("GetProjectConfig projA: %v", err)
	}
	if valA != "valA" {
		t.Errorf("projA hook = %q, want %q", valA, "valA")
	}

	valB, err := pool.GetProjectConfig("projB", "hook")
	if err != nil {
		t.Fatalf("GetProjectConfig projB: %v", err)
	}
	if valB != "valB" {
		t.Errorf("projB hook = %q, want %q", valB, "valB")
	}
}
