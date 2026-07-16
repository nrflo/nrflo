package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigration170_RewritesRemovedCLIModelReferences(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "model-audit.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	m := newMigrateInstance(t, sqlDB)
	if err := m.Migrate(169); err != nil {
		t.Fatalf("migrate to 169: %v", err)
	}

	seedMigration170References(t, sqlDB)
	// Pinned below 000172 (which rewrites gpt-5.2 CLI refs onward) so these
	// assertions keep checking 000170's own rewrites.
	if err := m.Migrate(171); err != nil {
		t.Fatalf("migrate to 171: %v", err)
	}

	assertDefinitionRewrite(t, sqlDB, "cli-53", "gpt-5.2", "high", "gpt-5.2")
	assertDefinitionRewrite(t, sqlDB, "api-53", "gpt-5.3-codex", "", "")
	assertDefinitionRewrite(t, sqlDB, "mini", "gpt-5.6-luna", "low", "gpt-5.6-luna")
	assertScalar(t, sqlDB, `SELECT model FROM system_agent_definitions WHERE id = 'sys-cli-53'`, "gpt-5.2")
	assertScalar(t, sqlDB, `SELECT reasoning_effort FROM system_agent_definitions WHERE id = 'sys-cli-53'`, "high")
	assertScalar(t, sqlDB, `SELECT model FROM system_agent_definitions WHERE id = 'sys-api-53'`, "gpt-5.3-codex")
	assertScalar(t, sqlDB, `SELECT model FROM system_agent_definitions WHERE id = 'sys-mini'`, "gpt-5.6-luna")
	assertScalar(t, sqlDB, `SELECT observer_model FROM workflows WHERE id = 'wf-53'`, "gpt-5.2")
	assertScalar(t, sqlDB, `SELECT observer_model FROM workflows WHERE id = 'wf-mini'`, "gpt-5.6-luna")
	assertScalar(t, sqlDB, `SELECT model_id FROM agent_sessions WHERE id = 'historical-mini'`, "gpt-5.5-mini")
}

func seedMigration170References(t *testing.T, db *sql.DB) {
	t.Helper()
	now := "2026-07-15T00:00:00Z"
	statements := []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('audit', 'Audit', ?, ?)`,
		`INSERT INTO workflows (project_id, id, description, scope_type, observer_provider, observer_model, created_at, updated_at) VALUES ('audit', 'wf-53', '', 'ticket', 'codex', 'gpt-5.3-codex', ?, ?)`,
		`INSERT INTO workflows (project_id, id, description, scope_type, observer_provider, observer_model, created_at, updated_at) VALUES ('audit', 'wf-mini', '', 'ticket', 'codex', 'gpt-5.5-mini', ?, ?)`,
		`INSERT INTO workflow_instances (id, project_id, def_project_id, workflow_id, status, created_at, updated_at) VALUES ('wfi-audit', 'audit', 'audit', 'wf-53', 'completed', ?, ?)`,
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, low_consumption_model, prompt, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('cli-53', 'audit', 'wf-53', 'gpt-5.3-codex', 'gpt-5.3-codex', '', 'cli_interactive', NULL, ?, ?)`,
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, prompt, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('api-53', 'audit', 'wf-53', 'gpt-5.3-codex', '', 'api', NULL, ?, ?)`,
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, low_consumption_model, prompt, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('mini', 'audit', 'wf-mini', 'gpt-5.5-mini', 'gpt-5.5-mini', '', 'cli_interactive', NULL, ?, ?)`,
		`INSERT INTO system_agent_definitions (id, model, role, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('sys-cli-53', 'gpt-5.3-codex', 'audit-cli', 'cli_interactive', NULL, ?, ?)`,
		`INSERT INTO system_agent_definitions (id, model, role, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('sys-api-53', 'gpt-5.3-codex', 'audit-api', 'api', NULL, ?, ?)`,
		`INSERT INTO system_agent_definitions (id, model, role, execution_mode, reasoning_effort, created_at, updated_at) VALUES ('sys-mini', 'gpt-5.5-mini', 'audit-mini', 'cli_interactive', NULL, ?, ?)`,
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at) VALUES ('historical-mini', 'audit', '', 'wfi-audit', 'audit', 'audit', 'gpt-5.5-mini', 'completed', ?, ?)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement, now, now); err != nil {
			t.Fatalf("seed migration 170 reference: %v", err)
		}
	}
}
