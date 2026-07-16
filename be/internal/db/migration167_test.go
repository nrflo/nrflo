package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

func TestMigration167_CanonicalModels(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	want := map[string]string{
		"sonnet-5":      "anthropic|claude-sonnet-5|claude-sonnet-5|1000000|1000000||",
		"haiku-4-5":     "anthropic|claude-haiku-4-5|claude-haiku-4-5|200000|200000||",
		"opus-4-6":      "anthropic|claude-opus-4-6|claude-opus-4-6|200000|1000000||",
		"opus-4-6-1m":   "anthropic|claude-opus-4-6[1m]|claude-opus-4-6[1m]|1000000|1000000|claude-opus-4-6|",
		"opus-4-7":      "anthropic|claude-opus-4-7|claude-opus-4-7|200000|1000000||",
		"opus-4-7-1m":   "anthropic|claude-opus-4-7[1m]|claude-opus-4-7[1m]|1000000|1000000|claude-opus-4-7|",
		"opus-4-8":      "anthropic|claude-opus-4-8|claude-opus-4-8|200000|1000000||",
		"opus-4-8-1m":   "anthropic|claude-opus-4-8[1m]|claude-opus-4-8[1m]|1000000|1000000|claude-opus-4-8|",
		"gpt-5.3-codex": "openai|gpt-5.3-codex|gpt-5.3-codex|200000|200000||high",
		"gpt-5.4":       "openai|gpt-5.4|gpt-5.4|200000|200000||medium",
		"gpt-5.4-mini":  "openai|gpt-5.4-mini||200000|200000||low",
		"gpt-5.5":       "openai|gpt-5.5|gpt-5.5|200000|200000||medium",
		"gpt-5.5-mini":  "openai|gpt-5.5-mini||200000|200000||low",
		"gpt-5.6-sol":   "openai|gpt-5.6-sol|gpt-5.6-sol|372000|372000||medium",
		"gpt-5.6-terra": "openai|gpt-5.6-terra||372000|200000||medium",
		"gpt-5.6-luna":  "openai|gpt-5.6-luna||372000|200000||low",
	}

	rows, err := pool.Query(`SELECT id, provider, cli_model, api_model, cli_context,
		api_context, fallback_models, default_effort FROM models WHERE read_only = 1`)
	if err != nil {
		t.Fatalf("query canonical models: %v", err)
	}
	defer rows.Close()
	got := make(map[string]string)
	for rows.Next() {
		var id, provider, cliModel, apiModel, fallback, effort string
		var cliContext, apiContext int
		if err := rows.Scan(&id, &provider, &cliModel, &apiModel, &cliContext,
			&apiContext, &fallback, &effort); err != nil {
			t.Fatalf("scan canonical model: %v", err)
		}
		got[id] = fmt.Sprintf("%s|%s|%s|%d|%d|%s|%s", provider, cliModel,
			apiModel, cliContext, apiContext, fallback, effort)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("canonical model rows: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("canonical model count = %d, want %d", len(got), len(want))
	}
	for id, expected := range want {
		if got[id] != expected {
			t.Errorf("models[%q] = %q, want %q", id, got[id], expected)
		}
	}

	assertEfforts(t, pool, "gpt-5.6-sol", "[\"low\",\"medium\",\"high\",\"xhigh\",\"max\",\"ultra\"]", "[\"low\",\"medium\",\"high\",\"xhigh\",\"max\"]")
	assertEfforts(t, pool, "opus-4-6", "[\"low\",\"medium\",\"high\",\"max\"]", "[\"low\",\"medium\",\"high\",\"max\"]")
}

func assertEfforts(t *testing.T, pool *Pool, id, wantCLI, wantAPI string) {
	t.Helper()
	var cli, api string
	if err := pool.QueryRow(`SELECT cli_efforts, api_efforts FROM models WHERE id = ?`, id).Scan(&cli, &api); err != nil {
		t.Fatalf("query efforts for %s: %v", id, err)
	}
	if cli != wantCLI || api != wantAPI {
		t.Errorf("models[%q] efforts = (%s, %s), want (%s, %s)", id, cli, api, wantCLI, wantAPI)
	}
}

func TestMigration167_CustomMergeAndReferenceRewrite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "model-migration.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	m := newMigrateInstance(t, sqlDB)
	if err := m.Migrate(166); err != nil {
		t.Fatalf("migrate to 166: %v", err)
	}

	seedPre167Models(t, sqlDB)
	seedPre168References(t, sqlDB)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate remaining: %v", err)
	}

	var provider, cliModel, apiModel, cliEfforts, apiEfforts, fallback, effort string
	var cliContext, apiContext int
	err = sqlDB.QueryRow(`SELECT provider, cli_model, api_model, cli_efforts, api_efforts,
		cli_context, api_context, fallback_models, default_effort FROM models WHERE id = 'custom-shared'`).
		Scan(&provider, &cliModel, &apiModel, &cliEfforts, &apiEfforts,
			&cliContext, &apiContext, &fallback, &effort)
	if err != nil {
		t.Fatalf("query merged custom model: %v", err)
	}
	if got := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d|%s|%s", provider, cliModel,
		apiModel, cliEfforts, apiEfforts, cliContext, apiContext, fallback, effort); got != "openai|cli-custom|api-custom|[\"high\"]|[\"low\"]|123000|456000|cli-fallback|high" {
		t.Errorf("merged custom model = %s", got)
	}

	var collisionModel, collisionEffort string
	if err := sqlDB.QueryRow(`SELECT cli_model, default_effort FROM models WHERE id = 'gpt-5.4'`).Scan(&collisionModel, &collisionEffort); err != nil {
		t.Fatalf("query canonical collision: %v", err)
	}
	if collisionModel != "gpt-5.4" || collisionEffort != "medium" {
		t.Errorf("custom collision overwrote canonical row: model=%q effort=%q", collisionModel, collisionEffort)
	}

	assertDefinitionRewrite(t, sqlDB, "def-inherit", "gpt-5.6-sol", "high", "gpt-5.6-luna")
	assertDefinitionRewrite(t, sqlDB, "def-override", "gpt-5.4", "xhigh", "")
	assertScalar(t, sqlDB, `SELECT model FROM system_agent_definitions WHERE id = 'sys-rewrite'`, "gpt-5.3-codex")
	assertScalar(t, sqlDB, `SELECT reasoning_effort FROM system_agent_definitions WHERE id = 'sys-rewrite'`, "medium")
	assertScalar(t, sqlDB, `SELECT model_id FROM agent_sessions WHERE id = 'session-rewrite'`, "opus-4-8-1m")

	for _, table := range []string{"cli_models", "api_models"} {
		var count int
		if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatalf("check dropped table %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("old table %s still exists", table)
		}
	}

	var orphans int
	if err := sqlDB.QueryRow(`SELECT COUNT(DISTINCT model) FROM agent_definitions
		WHERE model NOT IN (SELECT id FROM models) AND model != 'script'`).Scan(&orphans); err != nil {
		t.Fatalf("query definition orphans: %v", err)
	}
	if orphans != 0 {
		t.Errorf("agent definition model orphans = %d, want 0", orphans)
	}
}

func seedPre167Models(t *testing.T, db *sql.DB) {
	t.Helper()
	now := "2026-07-15T00:00:00Z"
	_, err := db.Exec(`INSERT INTO cli_models (id, cli_type, display_name, mapped_model,
		reasoning_effort, context_length, read_only, enabled, created_at, updated_at,
		fallback_models, supported_efforts) VALUES
		('custom-shared', 'codex', 'CLI Custom', 'cli-custom', 'high', 123000, 0, 0, ?, ?, 'cli-fallback', '["high"]'),
		('gpt-5.4', 'codex', 'Collision', 'wrong-model', 'ultra', 1, 0, 0, ?, ?, '', '["ultra"]')`, now, now, now, now)
	if err != nil {
		t.Fatalf("seed custom CLI models: %v", err)
	}
	_, err = db.Exec(`INSERT INTO api_models (id, provider, display_name, mapped_model,
		reasoning_effort, context_length, read_only, enabled, created_at, updated_at,
		supported_efforts) VALUES
		('custom-shared', 'openai', 'API Custom', 'api-custom', 'low', 456000, 0, 1, ?, ?, '["low"]')`, now, now)
	if err != nil {
		t.Fatalf("seed custom API model: %v", err)
	}
}

func seedPre168References(t *testing.T, db *sql.DB) {
	t.Helper()
	now := "2026-07-15T00:00:00Z"
	statements := []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('model-migration', 'Migration', ?, ?)`,
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('model-migration', 'wf', '', 'ticket', ?, ?)`,
		`INSERT INTO workflow_instances (id, project_id, def_project_id, workflow_id, status, created_at, updated_at) VALUES ('wfi-model', 'model-migration', 'model-migration', 'wf', 'active', ?, ?)`,
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, low_consumption_model, prompt, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('def-inherit', 'model-migration', 'wf', 'codex_gpt56_sol_high', 'codex_gpt56_luna_low', '', 'cli_interactive', NULL, ?, ?)`,
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, prompt, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('def-override', 'model-migration', 'wf', 'gpt54_high', '', 'api', 'xhigh', ?, ?)`,
		`INSERT INTO system_agent_definitions (id, model, role, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('sys-rewrite', 'gpt53_codex_medium', 'migration-test', 'api', NULL, ?, ?)`,
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, model_id, status, created_at, updated_at) VALUES ('session-rewrite', 'model-migration', '', 'wfi-model', 'test', 'test', 'test', 'opus_4_8_1m', 'completed', ?, ?)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement, now, now); err != nil {
			t.Fatalf("seed pre-rewrite reference: %v", err)
		}
	}
}

func assertDefinitionRewrite(t *testing.T, db *sql.DB, id, wantModel, wantEffort, wantLow string) {
	t.Helper()
	var model, effort, low string
	if err := db.QueryRow(`SELECT model, COALESCE(reasoning_effort, ''), low_consumption_model
		FROM agent_definitions WHERE id = ?`, id).Scan(&model, &effort, &low); err != nil {
		t.Fatalf("query definition %s: %v", id, err)
	}
	if model != wantModel || effort != wantEffort || low != wantLow {
		t.Errorf("definition %s = (%q, %q, %q), want (%q, %q, %q)", id, model, effort, low, wantModel, wantEffort, wantLow)
	}
}

func assertScalar(t *testing.T, db *sql.DB, query, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query scalar: %v", err)
	}
	if got != want {
		t.Errorf("scalar = %q, want %q", got, want)
	}
}
