package integration

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigration210MigratesOpus48To5 exercises migration 000210: the opus-5 /
// opus-5-1m catalog rows are seeded, fable-5's overload fallback moves to
// claude-opus-5, and every def/workflow/tier reference pinned to
// opus-4-8/opus-4-8-1m is rewritten to opus-5/opus-5-1m. The 4.8 catalog rows
// survive — the provider still serves them (only references migrate).
func TestMigration210MigratesOpus48To5(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate210.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := buildMigrator(t, sqlDB)
	migrateTo(t, m, 209)

	now := "2026-07-01T00:00:00Z"
	if _, err := sqlDB.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj-210", "p210", "/tmp", now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := sqlDB.Exec(
		`INSERT INTO workflows (id, project_id, description, observer_model, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"wf-210", "proj-210", "d", "opus-4-8", now, now); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	agents := []struct{ id, model, lcModel string }{
		{"builder", "opus-4-8", ""},
		{"planner", "opus-4-8-1m", ""},
		{"writer", "sonnet-5", "opus-4-8"},
		{"analyst", "haiku-4-5", "opus-4-8-1m"},
		{"untouched", "opus-4-7", "sonnet-5"},
	}
	for _, a := range agents {
		if _, err := sqlDB.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, low_consumption_model, layer, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.id, "proj-210", "wf-210", a.model, 60, "p", a.lcModel, 0, now, now); err != nil {
			t.Fatalf("insert agent_def %q: %v", a.id, err)
		}
	}

	sysAgents := []struct{ id, model string }{
		{"sys-1", "opus-4-8"},
		{"sys-2", "opus-4-8-1m"},
		{"sys-3", "sonnet-5"},
	}
	for _, s := range sysAgents {
		if _, err := sqlDB.Exec(
			`INSERT INTO system_agent_definitions (id, role, model, timeout, prompt, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			s.id, s.id, s.model, 60, "p", now, now); err != nil {
			t.Fatalf("insert system_agent_def %q: %v", s.id, err)
		}
	}

	if _, err := sqlDB.Exec(
		`INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		5, 0, "anthropic", "api", "opus-4-8", "xhigh"); err != nil {
		t.Fatalf("insert tier_model: %v", err)
	}

	migrateTo(t, m, 210)

	wantModel := map[string]string{
		"builder":   "opus-5",
		"planner":   "opus-5-1m",
		"writer":    "sonnet-5",
		"analyst":   "haiku-4-5",
		"untouched": "opus-4-7",
	}
	for id, want := range wantModel {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT model FROM agent_definitions WHERE id = ? AND project_id = ? AND workflow_id = ?`,
			id, "proj-210", "wf-210").Scan(&got); err != nil {
			t.Fatalf("scan agent_def %q: %v", id, err)
		}
		if got != want {
			t.Errorf("agent_def %q: model = %q, want %q", id, got, want)
		}
	}

	wantLC := map[string]string{
		"builder":   "",
		"planner":   "",
		"writer":    "opus-5",
		"analyst":   "opus-5-1m",
		"untouched": "sonnet-5",
	}
	for id, want := range wantLC {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT low_consumption_model FROM agent_definitions WHERE id = ? AND project_id = ? AND workflow_id = ?`,
			id, "proj-210", "wf-210").Scan(&got); err != nil {
			t.Fatalf("scan agent_def lc %q: %v", id, err)
		}
		if got != want {
			t.Errorf("agent_def %q: low_consumption_model = %q, want %q", id, got, want)
		}
	}

	wantSys := map[string]string{"sys-1": "opus-5", "sys-2": "opus-5-1m", "sys-3": "sonnet-5"}
	for id, want := range wantSys {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT model FROM system_agent_definitions WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("scan system_agent_def %q: %v", id, err)
		}
		if got != want {
			t.Errorf("system_agent_def %q: model = %q, want %q", id, got, want)
		}
	}

	assertScalarSQL(t, sqlDB, `SELECT observer_model FROM workflows WHERE id = 'wf-210'`, "opus-5")
	assertScalarSQL(t, sqlDB, `SELECT model_id FROM tier_models WHERE tier = 5 AND position = 0`, "opus-5")
	assertScalarSQL(t, sqlDB, `SELECT fallback_models FROM models WHERE id = 'fable-5'`, "claude-opus-5")

	// New Opus 5 catalog rows: mode-specific mapping, contexts, efforts, price.
	assertScalarSQL(t, sqlDB,
		`SELECT cli_model || '|' || api_model || '|' || cli_context || '|' || api_context || '|' || fallback_models
		 FROM models WHERE id = 'opus-5'`,
		"claude-opus-5|claude-opus-5|200000|1000000|")
	assertScalarSQL(t, sqlDB,
		`SELECT cli_model || '|' || api_model || '|' || cli_context || '|' || api_context || '|' || fallback_models
		 FROM models WHERE id = 'opus-5-1m'`,
		"claude-opus-5[1m]|claude-opus-5[1m]|1000000|1000000|claude-opus-5")
	for _, id := range []string{"opus-5", "opus-5-1m"} {
		assertScalarSQL(t, sqlDB,
			`SELECT cli_efforts || '|' || api_efforts FROM models WHERE id = '`+id+`'`,
			`["low","medium","high","xhigh","max"]|["low","medium","high","xhigh","max"]`)
		assertScalarSQL(t, sqlDB,
			`SELECT price_in || '|' || price_out || '|' || release_date FROM models WHERE id = '`+id+`'`,
			"5.0|25.0|2026-07-24")
	}

	// 4.8 rows must NOT be delisted — they remain selectable built-ins.
	for _, id := range []string{"opus-4-8", "opus-4-8-1m"} {
		assertScalarSQL(t, sqlDB,
			`SELECT enabled FROM models WHERE id = '`+id+`'`, "1")
	}
}

func assertScalarSQL(t *testing.T, sqlDB *sql.DB, query, want string) {
	t.Helper()
	var got string
	if err := sqlDB.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if got != want {
		t.Errorf("query %q = %q, want %q", query, got, want)
	}
}
