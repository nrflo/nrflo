package service

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSetup_SeedsGitignoredAgentContext: CLAUDE.md / AGENTS.md at any depth
// are copied and .claude directories at any depth are symlinked into a fresh
// worktree when the project gitignores them (so the checkout omits them).
func TestSetup_SeedsGitignoredAgentContext(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	write := func(rel, content string, mode os.FileMode) {
		t.Helper()
		p := filepath.Join(repoPath, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}

	// All ignored — absent from any checkout. ".claude" without a trailing
	// slash so the seeded symlink is ignored in the worktree as well. The
	// .gitignore itself is tracked (as in real projects) so the same rules
	// apply inside the worktree.
	write(".gitignore", "CLAUDE.md\nAGENTS.md\n.claude\n", 0o644)
	if _, err := runGit(repoPath, "add", ".gitignore"); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(repoPath, "commit", "-m", "ignore agent context"); err != nil {
		t.Fatal(err)
	}
	write("CLAUDE.md", "root instructions", 0o644)
	write("pkg/sub/CLAUDE.md", "nested instructions", 0o644)
	write("AGENTS.md", "codex instructions", 0o644)
	write(".claude/skills/demo/SKILL.md", "skill", 0o644)
	write("svc/.claude/refs/notes.md", "nested claude dir", 0o644)

	svc := &WorktreeService{}
	branch := "ctx-seed-" + filepath.Base(repoPath)
	wt, err := svc.Setup(repoPath, "main", branch)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer svc.Cleanup(repoPath, branch, wt)

	// Copied files, full hierarchy.
	for _, rel := range []string{"CLAUDE.md", "pkg/sub/CLAUDE.md", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(wt, rel)); err != nil {
			t.Errorf("expected %s copied into worktree: %v", rel, err)
		}
	}

	// .claude dirs symlinked to the main checkout, at both depths.
	for _, rel := range []string{".claude", "svc/.claude"} {
		info, err := os.Lstat(filepath.Join(wt, rel))
		if err != nil {
			t.Fatalf("expected %s in worktree: %v", rel, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s should be a symlink, got %v", rel, info.Mode())
		}
	}
	// Content reachable through the link (stays in sync with the root).
	if _, err := os.Stat(filepath.Join(wt, ".claude/skills/demo/SKILL.md")); err != nil {
		t.Errorf("skill not reachable through .claude symlink: %v", err)
	}

	// Seeding must not dirty the worktree (agent `git add -A` safety).
	out, err := runGit(wt, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("worktree not clean after seeding:\n%s", out)
	}
}

// TestSetup_SeedNeverOverwritesCheckout: tracked files arrive via the checkout
// itself; the seed step copies only absent paths and leaves tracked content
// untouched.
func TestSetup_SeedNeverOverwritesCheckout(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	if err := os.WriteFile(filepath.Join(repoPath, "CLAUDE.md"), []byte("tracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(repoPath, "add", "CLAUDE.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(repoPath, "commit", "-m", "track claude.md"); err != nil {
		t.Fatal(err)
	}
	// Uncommitted local edit in the root checkout: the worktree must get the
	// committed version (git's), not the dirty root copy.
	if err := os.WriteFile(filepath.Join(repoPath, "CLAUDE.md"), []byte("dirty local edit"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := &WorktreeService{}
	branch := "ctx-tracked-" + filepath.Base(repoPath)
	wt, err := svc.Setup(repoPath, "main", branch)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer svc.Cleanup(repoPath, branch, wt)

	got, err := os.ReadFile(filepath.Join(wt, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "tracked" {
		t.Errorf("tracked CLAUDE.md content = %q, want %q (seed must not overwrite)", got, "tracked")
	}
	out, err := runGit(wt, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("worktree dirty after setup:\n%s", out)
	}
}
