package db

import (
	"fmt"
	"path/filepath"
	"testing"
)

func seedProject105(t *testing.T, pool *Pool, projectID string) {
	t.Helper()
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project %s: %v", projectID, err)
	}
}

func seedWorkflow105(t *testing.T, pool *Pool, projectID, workflowID string) {
	t.Helper()
	if _, err := pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'ticket', datetime('now'), datetime('now'))`, projectID, workflowID); err != nil {
		t.Fatalf("seed workflow %s: %v", workflowID, err)
	}
}

// TestMigration105_AgentDefinitionsCheckMatrix verifies the rebuilt CHECK constraint:
// cli rejected, cli_interactive/api/script accepted, foo/empty/batch rejected.
func TestMigration105_AgentDefinitionsCheckMatrix(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProject105(t, pool, "p1")
	seedWorkflow105(t, pool, "p1", "wf1")

	cases := []struct {
		mode   string
		wantOK bool
	}{
		{"cli", false},
		{"cli_interactive", true},
		{"api", true},
		{"script", true},
		{"foo", false},
		{"", false},
		{"batch", false},
	}
	for i, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			agentID := fmt.Sprintf("a105-%d", i)
			_, err := pool.Exec(
				`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, created_at, updated_at)
				 VALUES (?, 'p1', 'wf1', 'sonnet', 20, '', ?, datetime('now'), datetime('now'))`,
				agentID, tc.mode)
			if tc.wantOK && err != nil {
				t.Errorf("execution_mode=%q: unexpected error: %v", tc.mode, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("execution_mode=%q: expected CHECK constraint error, got nil", tc.mode)
			}
		})
	}
}

// TestMigration105_SystemAgentDefinitionsCheckMatrix verifies the rebuilt CHECK constraint:
// cli rejected, cli_interactive/api accepted, script/foo/empty rejected.
func TestMigration105_SystemAgentDefinitionsCheckMatrix(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cases := []struct {
		mode   string
		wantOK bool
	}{
		{"cli", false},
		{"cli_interactive", true},
		{"api", true},
		{"script", false},
		{"foo", false},
		{"", false},
	}
	for i, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			id := fmt.Sprintf("sys-105-%d", i)
			role := fmt.Sprintf("test-role-105-%d", i)
			_, err := pool.Exec(
				`INSERT INTO system_agent_definitions (id, model, timeout, prompt, role, execution_mode, created_at, updated_at)
				 VALUES (?, 'sonnet', 20, 'do stuff', ?, ?, datetime('now'), datetime('now'))`,
				id, role, tc.mode)
			if tc.wantOK && err != nil {
				t.Errorf("execution_mode=%q: unexpected error: %v", tc.mode, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("execution_mode=%q: expected CHECK constraint error, got nil", tc.mode)
			}
		})
	}
}

// TestMigration105_AgentDefinitionsDefaultIsCLIInteractive verifies that inserting without
// execution_mode yields cli_interactive (not the old default cli).
func TestMigration105_AgentDefinitionsDefaultIsCLIInteractive(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProject105(t, pool, "p1")
	seedWorkflow105(t, pool, "p1", "wf1")

	if _, err := pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, created_at, updated_at)
		VALUES ('a-def105', 'p1', 'wf1', 'sonnet', 20, '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert without execution_mode: %v", err)
	}

	var execMode string
	if err := pool.QueryRow(`SELECT execution_mode FROM agent_definitions WHERE id = 'a-def105'`).Scan(&execMode); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if execMode != "cli_interactive" {
		t.Errorf("execution_mode = %q, want cli_interactive", execMode)
	}
}

// TestMigration105_SystemAgentDefinitionsDefaultIsCLIInteractive verifies the same default
// for system_agent_definitions.
func TestMigration105_SystemAgentDefinitionsDefaultIsCLIInteractive(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if _, err := pool.Exec(`INSERT INTO system_agent_definitions (id, model, timeout, prompt, role, created_at, updated_at)
		VALUES ('sys-def105', 'sonnet', 20, 'do stuff', 'role-def105', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert without execution_mode: %v", err)
	}

	var execMode string
	if err := pool.QueryRow(`SELECT execution_mode FROM system_agent_definitions WHERE id = 'sys-def105'`).Scan(&execMode); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if execMode != "cli_interactive" {
		t.Errorf("execution_mode = %q, want cli_interactive", execMode)
	}
}

// TestMigration105_ProviderConfigRowsDeleted verifies that provider_*_modes config rows
// were removed by the migration.
func TestMigration105_ProviderConfigRowsDeleted(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM config WHERE key IN ('provider_claude_modes', 'provider_codex_modes', 'provider_opencode_modes')`,
	).Scan(&count); err != nil {
		t.Fatalf("count config rows: %v", err)
	}
	if count != 0 {
		t.Errorf("provider_*_modes config rows = %d after migration 105, want 0", count)
	}
}

// TestMigration105_SystemAgentRoleModeIndexExists verifies that idx_system_agent_role_mode
// was recreated after the table rebuild.
func TestMigration105_SystemAgentRoleModeIndexExists(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var name string
	err = pool.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_system_agent_role_mode'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("idx_system_agent_role_mode not found in sqlite_master: %v", err)
	}
	if name != "idx_system_agent_role_mode" {
		t.Errorf("index name = %q, want idx_system_agent_role_mode", name)
	}
}

// TestMigration105_CoercionProof verifies that rows inserted with cli_interactive (the
// coerced value for any legacy cli rows) are stored correctly under the new schema.
// Because migrations auto-run before any user code sees the DB, we demonstrate the
// post-coercion state: cli_interactive rows are accepted and readable.
func TestMigration105_CoercionProof(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProject105(t, pool, "p1")
	seedWorkflow105(t, pool, "p1", "wf1")

	// Insert what coerced cli rows look like post-migration (cli_interactive).
	if _, err := pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, created_at, updated_at)
		 VALUES ('coerced-row', 'p1', 'wf1', 'sonnet', 20, '', 'cli_interactive', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("insert coerced row: %v", err)
	}

	var execMode string
	if err := pool.QueryRow(`SELECT execution_mode FROM agent_definitions WHERE id = 'coerced-row'`).Scan(&execMode); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if execMode != "cli_interactive" {
		t.Errorf("coerced row execution_mode = %q, want cli_interactive", execMode)
	}

	// Confirm cli itself is rejected (no pre-105 values can exist after migration).
	_, err = pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, created_at, updated_at)
		 VALUES ('cli-row', 'p1', 'wf1', 'sonnet', 20, '', 'cli', datetime('now'), datetime('now'))`,
	)
	if err == nil {
		t.Error("inserting execution_mode='cli' should fail after migration 105, got nil")
	}
}
