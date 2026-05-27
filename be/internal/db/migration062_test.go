package db

import (
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
	pool, err := newMigratedTestPool(t)
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
	pool, err := newMigratedTestPool(t)
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
	pool, err := newMigratedTestPool(t)
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

// Note: the tool_definitions table and its indexes added by migration 062 were
// removed by migration 134 (HTTP tools feature deleted), so they are no longer
// asserted here.
