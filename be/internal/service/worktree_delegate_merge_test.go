package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitOut runs a git command in repoPath returning trimmed stdout.
func gitOut(t *testing.T, repoPath string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestMergeDelegateBranch_HappyPath(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	runGitInRepo(t, repoPath, "checkout", "-b", "nrdelegate/happy")
	createCommit(t, repoPath, "delegate.txt", "delegate work", "Delegate commit")
	runGitInRepo(t, repoPath, "checkout", "main")

	svc := &WorktreeService{}
	sha, already, err := svc.MergeDelegateBranch(repoPath, "nrdelegate/happy")
	if err != nil {
		t.Fatalf("MergeDelegateBranch: %v", err)
	}
	if already {
		t.Error("alreadyMerged = true, want false for an unmerged branch")
	}
	if sha != gitOut(t, repoPath, "rev-parse", "HEAD") {
		t.Errorf("mergeCommit = %q, want live HEAD", sha)
	}
	if _, err := os.Stat(filepath.Join(repoPath, "delegate.txt")); err != nil {
		t.Errorf("delegate.txt not present after merge: %v", err)
	}
	if out := gitOut(t, repoPath, "branch", "--list", "nrdelegate/happy"); out != "" {
		t.Errorf("branch still exists after merge: %q", out)
	}
}

func TestMergeDelegateBranch_AlreadyMerged(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	runGitInRepo(t, repoPath, "checkout", "-b", "nrdelegate/already")
	createCommit(t, repoPath, "a.txt", "a", "A commit")
	runGitInRepo(t, repoPath, "checkout", "main")
	runGitInRepo(t, repoPath, "merge", "nrdelegate/already", "--no-edit")

	svc := &WorktreeService{}
	sha, already, err := svc.MergeDelegateBranch(repoPath, "nrdelegate/already")
	if err != nil {
		t.Fatalf("MergeDelegateBranch: %v", err)
	}
	if !already {
		t.Error("alreadyMerged = false, want true")
	}
	if sha == "" {
		t.Error("mergeCommit empty, want HEAD sha")
	}
	if out := gitOut(t, repoPath, "branch", "--list", "nrdelegate/already"); out != "" {
		t.Errorf("branch still exists after already-merged cleanup: %q", out)
	}
}

func TestMergeDelegateBranch_DirtyTreeRefused(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	runGitInRepo(t, repoPath, "checkout", "-b", "nrdelegate/dirty")
	createCommit(t, repoPath, "b.txt", "b", "B commit")
	runGitInRepo(t, repoPath, "checkout", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "uncommitted.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := &WorktreeService{}
	_, _, err := svc.MergeDelegateBranch(repoPath, "nrdelegate/dirty")
	if err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("err = %v, want uncommitted-changes refusal", err)
	}
	if out := gitOut(t, repoPath, "branch", "--list", "nrdelegate/dirty"); out == "" {
		t.Error("branch deleted despite refused merge")
	}
}

func TestMergeDelegateBranch_ConflictAbortsAndPreservesBranch(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	runGitInRepo(t, repoPath, "checkout", "-b", "nrdelegate/conflict")
	createCommit(t, repoPath, "shared.txt", "delegate version", "Delegate side")
	runGitInRepo(t, repoPath, "checkout", "main")
	createCommit(t, repoPath, "shared.txt", "main version", "Main side")

	svc := &WorktreeService{}
	_, _, err := svc.MergeDelegateBranch(repoPath, "nrdelegate/conflict")
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("err = %v, want conflict error", err)
	}
	if !strings.Contains(err.Error(), "shared.txt") {
		t.Errorf("err = %v, want conflicted file named", err)
	}
	if out := gitOut(t, repoPath, "status", "--porcelain"); out != "" {
		t.Errorf("live tree not clean after aborted merge: %q", out)
	}
	if out := gitOut(t, repoPath, "branch", "--list", "nrdelegate/conflict"); out == "" {
		t.Error("branch deleted despite failed merge")
	}
}

func TestMergeDelegateBranch_UnknownBranch(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	_, _, err := svc.MergeDelegateBranch(repoPath, "nrdelegate/missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want branch-not-found error", err)
	}
}
