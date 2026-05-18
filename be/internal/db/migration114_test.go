package db

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func seedProject114(t *testing.T, pool *Pool, projectID string) {
	t.Helper()
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, 'P', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project %s: %v", projectID, err)
	}
}

func TestMigration114_NewColumns(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "python_scripts")

	checks := []struct {
		col     string
		notNull int
		dfltPat string
	}{
		{"kind", 1, "agent"},
		{"tool_description", 1, ""},
		{"input_schema", 1, "{}"},
		{"timeout_sec", 1, "30"},
	}
	for _, c := range checks {
		info, ok := cols[c.col]
		if !ok {
			t.Errorf("python_scripts missing column %q", c.col)
			continue
		}
		if info.notNull != c.notNull {
			t.Errorf("column %q notNull = %d, want %d", c.col, info.notNull, c.notNull)
		}
		if info.dflt == nil {
			t.Errorf("column %q dflt = nil, want default value containing %q", c.col, c.dfltPat)
		} else if c.dfltPat != "" {
			s := fmt.Sprintf("%v", info.dflt)
			if !strings.Contains(s, c.dfltPat) {
				t.Errorf("column %q dflt = %v, want to contain %q", c.col, info.dflt, c.dfltPat)
			}
		}
	}
}

func TestMigration114_KindCheckConstraint(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	seedProject114(t, pool, "p1")

	cases := []struct {
		name   string
		kind   string
		wantOK bool
	}{
		{"agent accepted", "agent", true},
		{"tool accepted", "tool", true},
		{"empty rejected", "", false},
		{"foo rejected", "foo", false},
		{"Agent rejected", "Agent", false},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pool.Exec(
				`INSERT INTO python_scripts (id, project_id, name, description, code, kind, created_at, updated_at)
				 VALUES (?, 'p1', ?, '', '', ?, datetime('now'), datetime('now'))`,
				fmt.Sprintf("ps-k%d", i), fmt.Sprintf("name%d", i), tc.kind,
			)
			if tc.wantOK && err != nil {
				t.Errorf("insert kind=%q: unexpected error: %v", tc.kind, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("insert kind=%q: expected CHECK constraint error, got nil", tc.kind)
			}
		})
	}
}

func TestMigration114_KindDefault(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	seedProject114(t, pool, "p1")

	if _, err := pool.Exec(
		`INSERT INTO python_scripts (id, project_id, name, description, code, created_at, updated_at)
		 VALUES ('ps-def', 'p1', 'Script', '', '', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("insert without kind: %v", err)
	}

	var kind string
	if err := pool.QueryRow(`SELECT kind FROM python_scripts WHERE id='ps-def'`).Scan(&kind); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if kind != "agent" {
		t.Errorf("kind = %q, want default %q", kind, "agent")
	}
}

func TestMigration114_PartialUniqueIndex(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	seedProject114(t, pool, "p1")
	seedProject114(t, pool, "p2")

	insert := func(id, proj, name, kind string) error {
		_, e := pool.Exec(
			`INSERT INTO python_scripts (id, project_id, name, description, code, kind, created_at, updated_at)
			 VALUES (?, ?, ?, '', '', ?, datetime('now'), datetime('now'))`,
			id, proj, name, kind,
		)
		return e
	}

	// Two tool rows same name+project → UNIQUE error
	if err := insert("ps-t1", "p1", "foo", "tool"); err != nil {
		t.Fatalf("first tool insert: %v", err)
	}
	err2 := insert("ps-t2", "p1", "foo", "tool")
	if err2 == nil {
		t.Error("second tool insert same name+project: expected UNIQUE error, got nil")
	} else if !strings.Contains(err2.Error(), "UNIQUE") {
		t.Errorf("second tool insert: error = %v, want UNIQUE constraint", err2)
	}

	// Agent + tool same name+project → both succeed
	if err := insert("ps-a1", "p1", "bar", "agent"); err != nil {
		t.Fatalf("agent insert: %v", err)
	}
	if err := insert("ps-t3", "p1", "bar", "tool"); err != nil {
		t.Errorf("tool after agent same name+project: unexpected error: %v", err)
	}

	// Two tool rows same name, different projects → both succeed
	if err := insert("ps-t4", "p1", "baz", "tool"); err != nil {
		t.Fatalf("tool p1 insert: %v", err)
	}
	if err := insert("ps-t5", "p2", "baz", "tool"); err != nil {
		t.Errorf("tool p2 same name: unexpected error: %v", err)
	}
}

func TestMigration114_ReviewItemsGone(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='review_items'`,
	).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 0 {
		t.Error("review_items table still exists after migration 114")
	}
}

func TestMigration114_CustomerConfigVersionsGone(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='customer_config_versions'`,
	).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 0 {
		t.Error("customer_config_versions table still exists after migration 114")
	}
}

func TestMigration114_ToolDispatchesCleanup(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	seedProject114(t, pool, "p-td")

	// Seed a tool_definition so we can reference it by name.
	if _, err := pool.Exec(
		`INSERT INTO tool_definitions (id, name, description, input_schema, endpoint, created_at, updated_at)
		 VALUES ('td-1', 'my_custom_tool', '', '{}', 'http://x/tool', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("insert tool_definition: %v", err)
	}

	ins := func(id, toolName string) {
		t.Helper()
		if _, err := pool.Exec(
			`INSERT INTO tool_dispatches (id, project_id, tool_name, input, status, duration_ms, created_at)
			 VALUES (?, 'p-td', ?, '{}', 'success', 1, datetime('now'))`,
			id, toolName,
		); err != nil {
			t.Fatalf("insert tool_dispatch %s: %v", id, err)
		}
	}

	ins("d-builtin", "findings_add")    // builtin — keep
	ins("d-tooldef", "my_custom_tool")  // in tool_definitions — keep
	ins("d-orphan", "legacy_dead_tool") // orphan — delete

	// Execute the same cleanup DELETE from migration 114.
	if _, err := pool.Exec(`
		DELETE FROM tool_dispatches
		WHERE tool_name NOT IN (SELECT name FROM tool_definitions)
		  AND tool_name NOT IN (
		      'findings_add', 'findings_add_bulk', 'findings_append', 'findings_append_bulk',
		      'findings_get', 'findings_delete',
		      'project_findings_add', 'project_findings_add_bulk', 'project_findings_append',
		      'project_findings_append_bulk', 'project_findings_get', 'project_findings_delete',
		      'agent_fail', 'agent_finished', 'agent_continue', 'agent_callback', 'agent_context_update',
		      'workflow_skip',
		      'artifact_add', 'artifact_list', 'artifact_get'
		  )`); err != nil {
		t.Fatalf("cleanup DELETE: %v", err)
	}

	var remaining int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM tool_dispatches WHERE project_id='p-td'`).Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 2 {
		t.Errorf("tool_dispatches count = %d after cleanup, want 2 (builtin + tool_def)", remaining)
	}

	var orphanCount int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM tool_dispatches WHERE tool_name='legacy_dead_tool'`).Scan(&orphanCount); err != nil {
		t.Fatalf("count orphan: %v", err)
	}
	if orphanCount != 0 {
		t.Error("orphan tool_dispatch still present after cleanup")
	}
}
