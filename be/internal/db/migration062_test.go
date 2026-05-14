package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// columnInfo summarizes a row from PRAGMA table_info.
type columnInfo struct {
	name    string
	colType string
	notNull int
	dflt    interface{}
}

func tableColumns(t *testing.T, p *Pool, table string) map[string]columnInfo {
	t.Helper()
	rows, err := p.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer rows.Close()
	out := map[string]columnInfo{}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[name] = columnInfo{name: name, colType: colType, notNull: notNull, dflt: dflt}
	}
	return out
}

// TestMigration062_AgentDefinitionsColumns verifies new columns exist.
func TestMigration062_AgentDefinitionsColumns(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "agent_definitions")
	em, ok := cols["execution_mode"]
	if !ok {
		t.Fatal("execution_mode column missing")
	}
	if em.notNull != 1 {
		t.Errorf("execution_mode notNull = %d, want 1", em.notNull)
	}
	if em.colType != "TEXT" {
		t.Errorf("execution_mode type = %q, want TEXT", em.colType)
	}
	tools, ok := cols["tools"]
	if !ok {
		t.Fatal("tools column missing")
	}
	if tools.notNull != 1 {
		t.Errorf("tools notNull = %d, want 1", tools.notNull)
	}
	if _, ok := cols["api_max_iterations"]; !ok {
		t.Fatal("api_max_iterations column missing")
	}
	if got := cols["api_max_iterations"].notNull; got != 0 {
		t.Errorf("api_max_iterations notNull = %d, want 0 (nullable)", got)
	}
}

// TestMigration062_AgentDefinitionsDefaults verifies legacy rows get cli_interactive/empty/null.
// Note: migration 105 coerced the original 'cli' default to 'cli_interactive'.
func TestMigration062_AgentDefinitionsDefaults(t *testing.T) {
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
	if _, err := pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, created_at, updated_at)
		VALUES ('a1', 'p1', 'wf1', 'sonnet', 20, '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed agent_def: %v", err)
	}

	var execMode, tools string
	var apiMax interface{}
	row := pool.QueryRow(`SELECT execution_mode, tools, api_max_iterations FROM agent_definitions WHERE id = 'a1'`)
	if err := row.Scan(&execMode, &tools, &apiMax); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if execMode != "cli_interactive" {
		t.Errorf("execution_mode = %q, want %q", execMode, "cli_interactive")
	}
	if tools != "" {
		t.Errorf("tools = %q, want empty", tools)
	}
	if apiMax != nil {
		t.Errorf("api_max_iterations = %v, want NULL", apiMax)
	}
}

// TestMigration062_ExecutionModeCheck rejects values outside cli/api.
func TestMigration062_ExecutionModeCheck(t *testing.T) {
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

	cases := []struct {
		name   string
		value  string
		wantOK bool
	}{
		{"cli rejected", "cli", false},
		{"api accepted", "api", true},
		{"foo rejected", "foo", false},
		{"empty rejected", "", false},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := "agent-" + tc.value + "-" + tc.name
			_ = id
			_, err := pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, created_at, updated_at)
				VALUES (?, 'p1', 'wf1', 'sonnet', 20, '', ?, datetime('now'), datetime('now'))`,
				"a"+string(rune('A'+i)), tc.value)
			if tc.wantOK && err != nil {
				t.Errorf("insert %q: unexpected error: %v", tc.value, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("insert %q: expected error, got nil", tc.value)
			}
		})
	}
}

// TestMigration062_ToolDefinitionsTable verifies CRUD on the new tool_definitions table.
func TestMigration062_ToolDefinitionsTable(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if _, err := pool.Exec(`INSERT INTO tool_definitions (id, name, description, input_schema, endpoint, created_at, updated_at)
		VALUES ('t1', 'echo', 'Echoes input', '{"type":"object"}', 'http://x/echo', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert tool: %v", err)
	}
	var authMethod string
	var timeoutSec int
	if err := pool.QueryRow(`SELECT auth_method, timeout_sec FROM tool_definitions WHERE id = 't1'`).Scan(&authMethod, &timeoutSec); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if authMethod != "none" {
		t.Errorf("auth_method default = %q, want none", authMethod)
	}
	if timeoutSec != 30 {
		t.Errorf("timeout_sec default = %d, want 30", timeoutSec)
	}

	// UNIQUE on name
	_, err = pool.Exec(`INSERT INTO tool_definitions (id, name, description, input_schema, endpoint, created_at, updated_at)
		VALUES ('t2', 'echo', '', '{}', 'http://x', datetime('now'), datetime('now'))`)
	if err == nil {
		t.Error("expected UNIQUE constraint error on duplicate name")
	} else if !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("error = %v, want UNIQUE constraint", err)
	}

	// auth_method CHECK
	_, err = pool.Exec(`INSERT INTO tool_definitions (id, name, description, input_schema, endpoint, auth_method, created_at, updated_at)
		VALUES ('t3', 'other', '', '{}', 'http://x', 'oauth', datetime('now'), datetime('now'))`)
	if err == nil {
		t.Error("expected CHECK error on unknown auth_method")
	}
}

// TestMigration062_APICredentialsTable verifies the new api_credentials table.
func TestMigration062_APICredentialsTable(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// global (project_id NULL)
	if _, err := pool.Exec(`INSERT INTO api_credentials (id, provider, project_id, secret_ref, created_at, updated_at)
		VALUES ('c1', 'anthropic', NULL, 'env:ANTHROPIC_API_KEY', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert global: %v", err)
	}
	// per-project
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'P', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO api_credentials (id, provider, project_id, secret_ref, created_at, updated_at)
		VALUES ('c2', 'anthropic', 'p1', 'literal:sk-test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert per-project: %v", err)
	}
	var n int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM api_credentials WHERE provider = 'anthropic'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}
}

// TestMigration062_Indexes verifies the expected indexes were created.
func TestMigration062_Indexes(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	want := map[string]bool{
		"idx_api_credentials_provider_project": false,
		"idx_tool_definitions_project":         false,
		"idx_tool_definitions_workflow":        false,
	}
	rows, err := pool.Query(`SELECT name FROM sqlite_master WHERE type='index'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for n, ok := range want {
		if !ok {
			t.Errorf("index %s not created by migration 062", n)
		}
	}
}
