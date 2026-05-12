package db

import (
	"fmt"
	"path/filepath"
	"testing"
)

func seedProject101(t *testing.T, pool *Pool, projectID string) {
	t.Helper()
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project %s: %v", projectID, err)
	}
}

func seedWorkflow101(t *testing.T, pool *Pool, projectID, workflowID string) {
	t.Helper()
	if _, err := pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'ticket', datetime('now'), datetime('now'))`, projectID, workflowID); err != nil {
		t.Fatalf("seed workflow %s: %v", workflowID, err)
	}
}

// TestMigration101_AgentDefinitionsAcceptsCLIInteractive verifies that after migration 101 the
// agent_definitions CHECK constraint accepts cli|cli_interactive|api|script and rejects foo/empty/batch.
// This mirrors TestMigration085_ExecutionModeCheckIncludesScript but adds cli_interactive.
func TestMigration101_AgentDefinitionsAcceptsCLIInteractive(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProject101(t, pool, "p1")
	seedWorkflow101(t, pool, "p1", "wf1")

	cases := []struct {
		mode   string
		wantOK bool
	}{
		{"cli", true},
		{"cli_interactive", true},
		{"api", true},
		{"script", true},
		{"foo", false},
		{"", false},
		{"batch", false},
	}
	for i, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			agentID := fmt.Sprintf("a%d", i)
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

// TestMigration101_SystemAgentDefinitionsAcceptsCLIInteractive verifies that after migration 101
// the system_agent_definitions CHECK constraint accepts cli|cli_interactive|api and rejects
// script, foo, and empty (script is invalid for system agents).
func TestMigration101_SystemAgentDefinitionsAcceptsCLIInteractive(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cases := []struct {
		mode   string
		wantOK bool
	}{
		{"cli", true},
		{"cli_interactive", true},
		{"api", true},
		{"script", false},
		{"foo", false},
		{"", false},
	}
	for i, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			id := fmt.Sprintf("sys-101-%d", i)
			role := fmt.Sprintf("test-role-101-%d", i)
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

// TestMigration101_InteractiveCLIModeConfigDeleted verifies that after migration 101 there are
// no rows with key='interactive_cli_mode' in the config table. The migration deletes these rows
// because the project-wide toggle is superseded by per-agent execution_mode='cli_interactive'.
func TestMigration101_InteractiveCLIModeConfigDeleted(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM config WHERE key = 'interactive_cli_mode'`).Scan(&count); err != nil {
		t.Fatalf("count config rows: %v", err)
	}
	if count != 0 {
		t.Errorf("config rows with key='interactive_cli_mode' = %d after migration 101, want 0", count)
	}
}
