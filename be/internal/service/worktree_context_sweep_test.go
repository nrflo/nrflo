package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommitAndCollect_SeededContextNeverCommitted guards against the
// delegation commit sweeping seedAgentContext's files: in a project that
// does NOT gitignore CLAUDE.md/.claude, the seeded copies are visible to
// `git add -A`, and without the recorded-seeds unstage they ride the
// nrdelegate branch into the merge.
func TestCommitAndCollect_SeededContextNeverCommitted(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Untracked, not gitignored agent context in the live checkout.
	if err := os.WriteFile(filepath.Join(repoPath, "CLAUDE.md"), []byte("agent context"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoPath, ".claude", "skills"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".claude", "skills", "SKILL.md"), []byte("skill"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	svc := &WorktreeService{}
	worktreePath, baseCommit, err := svc.SetupFromHEAD(repoPath, "nrdelegate/seed-sweep")
	if err != nil {
		t.Fatalf("SetupFromHEAD: %v", err)
	}
	// Seeds materialized and recorded.
	if _, err := os.Lstat(filepath.Join(worktreePath, "CLAUDE.md")); err != nil {
		t.Fatalf("seeded CLAUDE.md missing in worktree: %v", err)
	}
	if got := seededContextPaths(worktreePath); len(got) != 2 {
		t.Fatalf("seededContextPaths = %v, want 2 entries (CLAUDE.md + .claude)", got)
	}

	// The worker's real change.
	if err := os.WriteFile(filepath.Join(worktreePath, "real.txt"), []byte("real work"), 0o644); err != nil {
		t.Fatalf("write real.txt: %v", err)
	}

	summary, err := svc.CommitAndCollect(repoPath, worktreePath, "nrdelegate/seed-sweep", baseCommit, "deleg-sweep", "seed sweep guard")
	if err != nil {
		t.Fatalf("CommitAndCollect: %v", err)
	}
	if !summary.Committed {
		t.Fatal("Committed = false, want true (real.txt changed)")
	}
	if len(summary.ChangedFiles) != 1 || summary.ChangedFiles[0] != "real.txt" {
		t.Errorf("ChangedFiles = %v, want [real.txt] (seeds must not be committed)", summary.ChangedFiles)
	}
	tree := runOutOrFatal(t, repoPath, "ls-tree", "-r", "--name-only", "nrdelegate/seed-sweep")
	if strings.Contains(tree, "CLAUDE.md") || strings.Contains(tree, ".claude") {
		t.Errorf("branch tree = %q, want no seeded agent-context paths", tree)
	}

	svc.Cleanup(repoPath, "nrdelegate/seed-sweep", worktreePath)
}
