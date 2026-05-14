package db

import (
	"path/filepath"
	"strings"
	"testing"
)

func seedProject085(t *testing.T, pool *Pool, projectID string) {
	t.Helper()
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project %s: %v", projectID, err)
	}
}

func seedWorkflow085(t *testing.T, pool *Pool, projectID, workflowID string) {
	t.Helper()
	if _, err := pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'ticket', datetime('now'), datetime('now'))`, projectID, workflowID); err != nil {
		t.Fatalf("seed workflow %s: %v", workflowID, err)
	}
}

func TestMigration085_PythonScriptsColumns(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "python_scripts")
	wantCols := []string{"id", "project_id", "name", "description", "code", "created_at", "updated_at"}
	for _, col := range wantCols {
		if _, ok := cols[col]; !ok {
			t.Errorf("python_scripts missing column %q", col)
		}
	}
	if cols["project_id"].notNull != 1 {
		t.Errorf("project_id notNull = %d, want 1", cols["project_id"].notNull)
	}
	if cols["name"].notNull != 1 {
		t.Errorf("name notNull = %d, want 1", cols["name"].notNull)
	}
}

func TestMigration085_AgentDefinitionsPythonScriptID(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "agent_definitions")
	psCol, ok := cols["python_script_id"]
	if !ok {
		t.Fatal("agent_definitions missing python_script_id column")
	}
	if psCol.notNull != 0 {
		t.Errorf("python_script_id notNull = %d, want 0 (nullable)", psCol.notNull)
	}
}

func TestMigration085_ExecutionModeCheckIncludesScript(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProject085(t, pool, "p1")
	seedWorkflow085(t, pool, "p1", "wf1")

	cases := []struct {
		name   string
		value  string
		wantOK bool
	}{
		{"cli rejected", "cli", false},
		{"api accepted", "api", true},
		{"script accepted", "script", true},
		{"foo rejected", "foo", false},
		{"empty rejected", "", false},
		{"batch rejected", "batch", false},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, created_at, updated_at)
				VALUES (?, 'p1', 'wf1', 'sonnet', 20, '', ?, datetime('now'), datetime('now'))`,
				"a"+string(rune('A'+i)), tc.value)
			if tc.wantOK && err != nil {
				t.Errorf("insert execution_mode=%q: unexpected error: %v", tc.value, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("insert execution_mode=%q: expected CHECK constraint error", tc.value)
			}
		})
	}
}

func TestMigration085_PythonScriptIDNullableOnAgentDef(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProject085(t, pool, "p1")
	seedWorkflow085(t, pool, "p1", "wf1")

	// Insert agent_definition without python_script_id (should be NULL).
	if _, err := pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, created_at, updated_at)
		VALUES ('a1', 'p1', 'wf1', 'sonnet', 20, '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert agent_def without python_script_id: %v", err)
	}

	var psID interface{}
	if err := pool.QueryRow(`SELECT python_script_id FROM agent_definitions WHERE id = 'a1'`).Scan(&psID); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if psID != nil {
		t.Errorf("python_script_id = %v, want NULL for row inserted without it", psID)
	}
}

func TestMigration085_CascadeDeleteOnProject(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProject085(t, pool, "proj-cascade")
	if _, err := pool.Exec(`INSERT INTO python_scripts (id, project_id, name, description, code, created_at, updated_at)
		VALUES ('ps-aaa', 'proj-cascade', 'Script', '', 'print(1)', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert python_script: %v", err)
	}

	if _, err := pool.Exec(`DELETE FROM projects WHERE id = 'proj-cascade'`); err != nil {
		t.Fatalf("delete project: %v", err)
	}

	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM python_scripts WHERE project_id = 'proj-cascade'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("python_scripts count = %d after CASCADE project delete, want 0", count)
	}
}

func TestMigration085_UniqueIndexOnProjectIDAndID(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProject085(t, pool, "p1")
	if _, err := pool.Exec(`INSERT INTO python_scripts (id, project_id, name, description, code, created_at, updated_at)
		VALUES ('ps-dup', 'p1', 'Script', '', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	_, err = pool.Exec(`INSERT INTO python_scripts (id, project_id, name, description, code, created_at, updated_at)
		VALUES ('ps-dup', 'p1', 'Script2', '', '', datetime('now'), datetime('now'))`)
	if err == nil {
		t.Error("expected UNIQUE constraint error on duplicate (project_id, id)")
	} else if !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("error = %v, want UNIQUE constraint", err)
	}
}

func TestMigration085_DifferentProjectsDifferentIDs(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProject085(t, pool, "p1")
	seedProject085(t, pool, "p2")

	// Two different projects can each have a script (different IDs, same name is fine).
	if _, err := pool.Exec(`INSERT INTO python_scripts (id, project_id, name, description, code, created_at, updated_at)
		VALUES ('ps-p1script', 'p1', 'Script', '', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert p1: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO python_scripts (id, project_id, name, description, code, created_at, updated_at)
		VALUES ('ps-p2script', 'p2', 'Script', '', '', datetime('now'), datetime('now'))`); err != nil {
		t.Errorf("different project with different ID: unexpected error: %v", err)
	}
}
