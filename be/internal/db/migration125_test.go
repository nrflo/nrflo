package db

import (
	"fmt"
	"path/filepath"
	"testing"
)

// TestMigration125_ConsultantColumnSchema verifies migration 000125 adds the
// consultant INTEGER NOT NULL DEFAULT 0 column to agent_definitions.
func TestMigration125_ConsultantColumnSchema(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "agent_definitions")
	col, ok := cols["consultant"]
	if !ok {
		t.Fatal("consultant column missing from agent_definitions; migration 000125 may not have run")
	}
	if col.colType != "INTEGER" {
		t.Errorf("consultant column type = %q, want INTEGER", col.colType)
	}
	if col.notNull != 1 {
		t.Errorf("consultant column notNull = %d, want 1", col.notNull)
	}
	dflt := fmt.Sprintf("%v", col.dflt)
	if dflt != "0" {
		t.Errorf("consultant column default = %q, want \"0\"", dflt)
	}
}

// TestMigration125_ExistingRowsDefaultToZero verifies that a row inserted
// without specifying consultant receives the DEFAULT 0 value.
func TestMigration125_ExistingRowsDefaultToZero(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'P', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('p1', 'wf1', '', 'ticket', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	// Insert agent_definition without specifying consultant — should default to 0.
	_, err = pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, created_at, updated_at)
		VALUES ('ag1', 'p1', 'wf1', 'sonnet', 20, 'do stuff', 'cli_interactive', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("INSERT agent_definitions without consultant: %v", err)
	}

	var consultant int
	if err := pool.QueryRow(`SELECT consultant FROM agent_definitions WHERE id = 'ag1'`).Scan(&consultant); err != nil {
		t.Fatalf("SELECT consultant: %v", err)
	}
	if consultant != 0 {
		t.Errorf("consultant default = %d, want 0", consultant)
	}
}
