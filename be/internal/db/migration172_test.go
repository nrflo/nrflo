package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

func TestMigration172_DelistsGPT52CLI(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gpt52-delist.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	m := newMigrateInstance(t, sqlDB)
	if err := m.Migrate(171); err != nil {
		t.Fatalf("migrate to 171: %v", err)
	}

	seedMigration172References(t, sqlDB)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate remaining: %v", err)
	}

	assertScalar(t, sqlDB, `SELECT cli_model FROM models WHERE id = 'gpt-5.2'`, "")
	assertScalar(t, sqlDB, `SELECT cli_efforts FROM models WHERE id = 'gpt-5.2'`, "[]")
	assertScalar(t, sqlDB, `SELECT api_model FROM models WHERE id = 'gpt-5.2'`, "gpt-5.2")

	assertScalar(t, sqlDB, `SELECT model FROM agent_definitions WHERE id = 'cli-52'`, "gpt-5.4")
	assertScalar(t, sqlDB, `SELECT low_consumption_model FROM agent_definitions WHERE id = 'cli-52'`, "gpt-5.4")
	assertScalar(t, sqlDB, `SELECT model FROM agent_definitions WHERE id = 'api-52'`, "gpt-5.2")
	assertScalar(t, sqlDB, `SELECT model FROM system_agent_definitions WHERE id = 'sys-cli-52'`, "gpt-5.4")
	assertScalar(t, sqlDB, `SELECT model FROM system_agent_definitions WHERE id = 'sys-api-52'`, "gpt-5.2")
	assertScalar(t, sqlDB, `SELECT observer_model FROM workflows WHERE id = 'wf-52'`, "gpt-5.4")
}

func seedMigration172References(t *testing.T, db *sql.DB) {
	t.Helper()
	now := "2026-07-16T00:00:00Z"
	statements := []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('delist', 'Delist', ?, ?)`,
		`INSERT INTO workflows (project_id, id, description, scope_type, observer_provider, observer_model, created_at, updated_at) VALUES ('delist', 'wf-52', '', 'ticket', 'codex', 'gpt-5.2', ?, ?)`,
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, low_consumption_model, prompt, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('cli-52', 'delist', 'wf-52', 'gpt-5.2', 'gpt-5.2', '', 'cli_interactive', NULL, ?, ?)`,
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, prompt, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('api-52', 'delist', 'wf-52', 'gpt-5.2', '', 'api', NULL, ?, ?)`,
		`INSERT INTO system_agent_definitions (id, model, role, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('sys-cli-52', 'gpt-5.2', 'delist-cli', 'cli_interactive', NULL, ?, ?)`,
		`INSERT INTO system_agent_definitions (id, model, role, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('sys-api-52', 'gpt-5.2', 'delist-api', 'api', NULL, ?, ?)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement, now, now); err != nil {
			t.Fatalf("seed migration 172 reference: %v", err)
		}
	}
}
