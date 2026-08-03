package db

import (
	"strings"
	"testing"
)

// 000228 seeds two readonly `injectable` default_templates rows describing
// where a spawned agent's checkout lives: workspace-live-tree and
// workspace-worktree. Both share a reporting rule (read branch/commit from
// git, never derive from ticket id) under a `## Workspace` heading.

func TestMigration228_WorkspaceContextRowsSeeded(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, id := range []string{"workspace-live-tree", "workspace-worktree"} {
		t.Run(id, func(t *testing.T) {
			var template, defaultTemplate, typ string
			var readonly int
			err := pool.QueryRow(
				`SELECT template, default_template, readonly, type FROM default_templates WHERE id = ?`, id,
			).Scan(&template, &defaultTemplate, &readonly, &typ)
			if err != nil {
				t.Fatalf("SELECT %s: %v", id, err)
			}
			if typ != "injectable" {
				t.Errorf("type = %q, want %q", typ, "injectable")
			}
			if readonly != 1 {
				t.Errorf("readonly = %d, want 1", readonly)
			}
			if template != defaultTemplate {
				t.Errorf("template != default_template (readonly invariant violated)")
			}
			if !strings.Contains(template, "## Workspace") {
				t.Errorf("template missing '## Workspace' heading; got %q", template)
			}
			for _, anchor := range []string{
				"git branch --show-current",
				"git rev-parse --short HEAD",
				"never derive a branch name from the ticket id",
				"Never create or switch branches",
			} {
				if !strings.Contains(template, anchor) {
					t.Errorf("template missing anchor %q; got %q", anchor, template)
				}
			}
		})
	}
}

func TestMigration228_LiveTreeVsWorktreeWording(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var liveTree, worktree string
	if err := pool.QueryRow(`SELECT template FROM default_templates WHERE id = 'workspace-live-tree'`).Scan(&liveTree); err != nil {
		t.Fatalf("SELECT workspace-live-tree: %v", err)
	}
	if err := pool.QueryRow(`SELECT template FROM default_templates WHERE id = 'workspace-worktree'`).Scan(&worktree); err != nil {
		t.Fatalf("SELECT workspace-worktree: %v", err)
	}

	if strings.Contains(liveTree, "worktree") {
		t.Errorf("workspace-live-tree template mentions worktree; got %q", liveTree)
	}
	if !strings.Contains(worktree, "worktree") {
		t.Errorf("workspace-worktree template missing worktree wording; got %q", worktree)
	}
	if !strings.Contains(liveTree, "${WORK_ROOT}") {
		t.Errorf("workspace-live-tree template missing ${WORK_ROOT} placeholder; got %q", liveTree)
	}
	if !strings.Contains(worktree, "${WORK_ROOT}") {
		t.Errorf("workspace-worktree template missing ${WORK_ROOT} placeholder; got %q", worktree)
	}
}
