package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

// TestMigration224_IsolateWorktreeSeedValues verifies the new isolate_worktree
// column defaults to 0 for every tier and is flipped to 1 only for
// _t1_executor, the sole tier isolated by default (Option 1: write-capable
// executor tiers only).
func TestMigration224_IsolateWorktreeSeedValues(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration224-seed.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}

	want := map[string]int{"_t1_executor": 1, "_t2_extractor": 0}
	for id, wantVal := range want {
		var got int
		if err := sqlDB.QueryRow(`SELECT isolate_worktree FROM system_agent_definitions WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("query %s isolate_worktree: %v", id, err)
		}
		if got != wantVal {
			t.Errorf("%s isolate_worktree = %d, want %d", id, got, wantVal)
		}
	}
}

// TestMigration224_OtherAgentDefsUnaffected verifies the migration's
// _t1_executor-scoped prompt rewrite does not collaterally touch an
// operator-edited prompt on a different system agent definition row.
func TestMigration224_OtherAgentDefsUnaffected(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration224-other-unaffected.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Migrate(223); err != nil {
		t.Fatalf("migrate to 223: %v", err)
	}
	const editedPrompt = "operator's own custom extractor prompt"
	if _, err := sqlDB.Exec(`UPDATE system_agent_definitions SET prompt = ? WHERE id = '_t2_extractor'`, editedPrompt); err != nil {
		t.Fatalf("seed operator edit: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate remaining (000224): %v", err)
	}

	var got string
	if err := sqlDB.QueryRow(`SELECT prompt FROM system_agent_definitions WHERE id = '_t2_extractor'`).Scan(&got); err != nil {
		t.Fatalf("query _t2_extractor prompt: %v", err)
	}
	if got != editedPrompt {
		t.Errorf("_t2_extractor prompt = %q, want unchanged operator edit %q", got, editedPrompt)
	}
}

// TestMigration224_T1ExecutorPromptWarnsAgainstSelfCommit verifies the
// rewritten _t1_executor prompt tells the worker it runs on a disposable
// branch and must not commit/push/switch itself (the server owns the commit).
func TestMigration224_T1ExecutorPromptWarnsAgainstSelfCommit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration224-prompt.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}

	var prompt string
	if err := sqlDB.QueryRow(`SELECT prompt FROM system_agent_definitions WHERE id = '_t1_executor'`).Scan(&prompt); err != nil {
		t.Fatalf("query _t1_executor prompt: %v", err)
	}
	if !strings.Contains(prompt, "disposable branch") {
		t.Errorf("_t1_executor prompt = %q, want it to mention the disposable branch", prompt)
	}
	if !strings.Contains(prompt, "never commit") {
		t.Errorf("_t1_executor prompt = %q, want it to warn against self-committing", prompt)
	}
}

// TestMigration224_DelegationGuidanceTemplateMentionsBranch verifies the
// readonly delegation-guidance injectable's template and default_template
// both pick up the executor-results-on-a-branch note.
func TestMigration224_DelegationGuidanceTemplateMentionsBranch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration224-template.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}

	var template, defaultTemplate string
	if err := sqlDB.QueryRow(`SELECT template, default_template FROM default_templates WHERE id = 'delegation-guidance'`).Scan(&template, &defaultTemplate); err != nil {
		t.Fatalf("query delegation-guidance template: %v", err)
	}
	if !strings.Contains(template, "branch") {
		t.Errorf("delegation-guidance template = %q, want it to mention the branch", template)
	}
	if template != defaultTemplate {
		t.Errorf("delegation-guidance template != default_template, want them kept in sync")
	}
}

// TestMigration224_DelegationsWorktreeColumnsDefaultEmpty verifies a
// delegations row inserted after the migration gets ” for all four new
// worktree columns absent an explicit value (in-place, non-isolated run).
func TestMigration224_DelegationsWorktreeColumnsDefaultEmpty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration224-delegations-default.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}

	if _, err := sqlDB.Exec(`INSERT INTO delegations (id, caller_session_id, workflow_instance_id, project_id, tier, brief, fanout, worker_session_ids, spawn_errors, depth, created_at)
		VALUES ('deleg-224', 'caller', 'wfi', 'proj', 'extractor', 'b', 1, '[""]', '[""]', 1, datetime('now'))`); err != nil {
		t.Fatalf("insert delegation row: %v", err)
	}

	var path, branch, base, summary string
	if err := sqlDB.QueryRow(`SELECT worktree_path, branch_name, base_commit, worktree_summary FROM delegations WHERE id = 'deleg-224'`).Scan(&path, &branch, &base, &summary); err != nil {
		t.Fatalf("query worktree columns: %v", err)
	}
	if path != "" || branch != "" || base != "" || summary != "" {
		t.Errorf("worktree columns = (%q,%q,%q,%q), want all empty by default", path, branch, base, summary)
	}
}
