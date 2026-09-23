package integration

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigration245MigratesToOpus55Fable51GPT6 exercises migration 000245: the
// opus-5-5 / fable-5-1 / gpt-6-* catalog rows are seeded, and every
// def/workflow/config/tier reference to opus-5(-1m), fable-5 and gpt-5.6-*
// moves to the new generation. The old catalog rows survive.
func TestMigration245MigratesToOpus55Fable51GPT6(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate245.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := buildMigrator(t, sqlDB)
	migrateTo(t, m, 244)

	now := "2026-09-01T00:00:00Z"
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := sqlDB.Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj-245", "p245", "/tmp", now, now)
	mustExec(`INSERT INTO workflows (id, project_id, description, observer_model, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`, "wf-245", "proj-245", "d", "opus-5", now, now)
	mustExec(`INSERT OR REPLACE INTO config (project_id, key, value) VALUES ('', 'observer_model', 'gpt-5.6-terra')`)

	agents := []struct{ id, model, lcModel string }{
		{"builder", "opus-5", ""},
		{"planner", "opus-5-1m", "gpt-5.6-luna"},
		{"deep", "fable-5", "gpt-5.6-sol"},
		{"codex", "gpt-5.6-terra", ""},
		{"untouched", "sonnet-5", "opus-4-8"},
	}
	for _, a := range agents {
		mustExec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, low_consumption_model, layer, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.id, "proj-245", "wf-245", a.model, 60, "p", a.lcModel, 0, now, now)
	}
	mustExec(`INSERT INTO system_agent_definitions (id, role, model, timeout, prompt, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`, "sys-1", "sys-1", "gpt-5.6-sol", 60, "p", now, now)

	migrateTo(t, m, 245)

	for id, want := range map[string]string{
		"builder": "opus-5-5|", "planner": "opus-5-5-1m|gpt-6-luna", "deep": "fable-5-1|gpt-6-sol",
		"codex": "gpt-6-sol|", "untouched": "sonnet-5|opus-4-8",
	} {
		assertScalarSQL(t, sqlDB,
			`SELECT model || '|' || low_consumption_model FROM agent_definitions WHERE id = '`+id+`'`, want)
	}
	assertScalarSQL(t, sqlDB, `SELECT model FROM system_agent_definitions WHERE id = 'sys-1'`, "gpt-6-sol")
	assertScalarSQL(t, sqlDB, `SELECT observer_model FROM workflows WHERE id = 'wf-245'`, "opus-5-5")
	assertScalarSQL(t, sqlDB, `SELECT value FROM config WHERE project_id = '' AND key = 'observer_model'`, "gpt-6-sol")

	// Seeded tier chains (000244) follow the rewrite.
	assertScalarSQL(t, sqlDB,
		`SELECT group_concat(tier || ':' || model_id, ',') FROM (
		   SELECT tier, model_id FROM tier_models WHERE provider IN ('anthropic', 'openai') AND execution_mode = 'cli_interactive' ORDER BY tier, position)`,
		"1:haiku-4-5,1:gpt-6-luna,2:sonnet-5,2:gpt-6-sol,3:sonnet-5,3:gpt-6-sol,4:sonnet-5,4:gpt-6-sol,5:opus-5-5,5:gpt-6-sol")
	assertScalarSQL(t, sqlDB, `SELECT model_id FROM tier_models WHERE tier = 5 AND position = 0`, "opus-5-5")

	assertScalarSQL(t, sqlDB,
		`SELECT cli_model || '|' || api_model || '|' || cli_context || '|' || api_context || '|' || fallback_models || '|' || price_in || '|' || price_cache_read
		 FROM models WHERE id = 'opus-5-5'`,
		"claude-opus-5-5|claude-opus-5-5|200000|1000000||4.0|0.2")
	assertScalarSQL(t, sqlDB,
		`SELECT cli_model || '|' || cli_context || '|' || fallback_models FROM models WHERE id = 'opus-5-5-1m'`,
		"claude-opus-5-5[1m]|1000000|claude-opus-5-5")
	assertScalarSQL(t, sqlDB,
		`SELECT cli_model || '|' || cli_context || '|' || fallback_models || '|' || price_in || '|' || price_cache_read FROM models WHERE id = 'fable-5-1'`,
		"claude-fable-5-1|1000000|claude-opus-5-5|10.0|0.25")
	assertScalarSQL(t, sqlDB, `SELECT fallback_models FROM models WHERE id = 'fable-5'`, "claude-opus-5-5")
	for id, want := range map[string]string{
		"gpt-6-astra": `["low","medium","high","xhigh","max","ultra"]|low|10.0`,
		"gpt-6-sol":   `["low","medium","high","xhigh","max","ultra"]|medium|2.0`,
		"gpt-6-luna":  `["low","medium","high","xhigh","max"]|medium|0.1`,
	} {
		assertScalarSQL(t, sqlDB,
			`SELECT cli_efforts || '|' || default_effort || '|' || price_in FROM models WHERE id = '`+id+`' AND cli_context = 272000 AND api_context = 1050000`, want)
	}
	assertScalarSQL(t, sqlDB, `SELECT price_in || '|' || price_out FROM models WHERE id = 'gpt-5.6-sol'`, "4.0|20.0")

	for _, id := range []string{"opus-5", "opus-5-1m", "fable-5", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		assertScalarSQL(t, sqlDB, `SELECT enabled FROM models WHERE id = '`+id+`'`, "1")
	}
}
