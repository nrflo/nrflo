package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMigration053_LayerColumnExists verifies agent_definitions has a layer column.
func TestMigration053_LayerColumnExists(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var layerFound bool
	rows, err := pool.Query("PRAGMA table_info(agent_definitions)")
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == "layer" {
			layerFound = true
			if colType != "INTEGER" {
				t.Errorf("layer type = %q, want INTEGER", colType)
			}
			if notNull != 1 {
				t.Errorf("layer NOT NULL = %d, want 1", notNull)
			}
		}
	}
	if !layerFound {
		t.Fatal("layer column not found in agent_definitions")
	}
}

// TestMigration053_PhasesColumnDropped verifies workflows table has no phases column.
func TestMigration053_PhasesColumnDropped(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows, err := pool.Query("PRAGMA table_info(workflows)")
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == "phases" {
			t.Fatal("phases column still exists in workflows table after migration 053")
		}
	}
}

// TestMigration053_LayerDefaultZero verifies that new agent_definitions rows default to layer=0.
func TestMigration053_LayerDefaultZero(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Seed project and workflow
	_, err = pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'P', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	_, err = pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('p1', 'wf1', 'W', 'ticket', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	// Insert agent_definition without specifying layer (should default to 0)
	_, err = pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, created_at, updated_at)
		VALUES ('test-agent', 'p1', 'wf1', 'sonnet', 20, 'do things', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert agent_definition: %v", err)
	}

	var layer int
	err = pool.QueryRow("SELECT layer FROM agent_definitions WHERE id = 'test-agent'").Scan(&layer)
	if err != nil {
		t.Fatalf("query layer: %v", err)
	}
	if layer != 0 {
		t.Errorf("default layer = %d, want 0", layer)
	}
}

// TestMigration053_WorkflowInsertWithoutPhases verifies workflows can be inserted without phases column.
func TestMigration053_WorkflowInsertWithoutPhases(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	_, err = pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'P', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	// Insert workflow — no phases column should be available
	_, err = pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('p1', 'wf-new', 'Test', 'ticket', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert workflow without phases: %v", err)
	}

	// Trying to insert with a phases column should fail
	_, err = pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, phases, created_at, updated_at) VALUES ('p1', 'wf-bad', 'Test', 'ticket', '[]', datetime('now'), datetime('now'))`)
	if err == nil {
		t.Fatal("expected error inserting with phases column, got nil")
	}
	if !strings.Contains(err.Error(), "phases") {
		t.Errorf("expected error mentioning 'phases', got: %v", err)
	}
}

// TestMigration053_AgentDefLayerExplicitValue verifies explicit layer values are stored.
func TestMigration053_AgentDefLayerExplicitValue(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	_, err = pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'P', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	_, err = pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('p1', 'wf1', 'W', 'ticket', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	// Insert with explicit layer
	_, err = pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, created_at, updated_at)
		VALUES ('agent-l5', 'p1', 'wf1', 'sonnet', 20, 'stuff', 5, datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert with layer=5: %v", err)
	}

	var layer int
	err = pool.QueryRow("SELECT layer FROM agent_definitions WHERE id = 'agent-l5'").Scan(&layer)
	if err != nil {
		t.Fatalf("query layer: %v", err)
	}
	if layer != 5 {
		t.Errorf("layer = %d, want 5", layer)
	}
}
