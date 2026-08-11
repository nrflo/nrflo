package db

import (
	"strings"
	"testing"
)

// 000238 appends the no-commit rule to the workspace-worktree injectable:
// delegate worktree commits are server-owned (CommitAndCollect), so workers
// must never run `git commit` themselves. The live-tree variant is untouched.
func TestMigration238_WorktreeNoCommitGuidance(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var template, defaultTemplate string
	err = pool.QueryRow(
		`SELECT template, default_template FROM default_templates WHERE id = 'workspace-worktree'`,
	).Scan(&template, &defaultTemplate)
	if err != nil {
		t.Fatalf("SELECT workspace-worktree: %v", err)
	}
	if template != defaultTemplate {
		t.Error("template != default_template (readonly invariant violated)")
	}
	if !strings.Contains(template, "Never run `git commit`") {
		t.Errorf("workspace-worktree missing no-commit rule; got %q", template)
	}
	// The bullet must land as its own line after the branch bullet.
	if !strings.Contains(template, "Never create or switch branches.\n- Never run `git commit`") {
		t.Error("no-commit bullet not on its own line after the branch bullet")
	}

	var liveTemplate string
	if err := pool.QueryRow(
		`SELECT template FROM default_templates WHERE id = 'workspace-live-tree'`,
	).Scan(&liveTemplate); err != nil {
		t.Fatalf("SELECT workspace-live-tree: %v", err)
	}
	if strings.Contains(liveTemplate, "git commit") {
		t.Errorf("workspace-live-tree must stay untouched; got %q", liveTemplate)
	}
}
