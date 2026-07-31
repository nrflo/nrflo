package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGit runs a git command in repoPath, failing the test on error.
func runGitInRepo(t *testing.T, repoPath string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// TestBranchMerged_Merged verifies BranchMerged returns true for a branch
// whose history is fully merged into the default branch.
func TestBranchMerged_Merged(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	runGitInRepo(t, repoPath, "checkout", "-b", "feature-merged")
	createCommit(t, repoPath, "feature.txt", "feature content", "Feature commit")
	runGitInRepo(t, repoPath, "checkout", "main")
	runGitInRepo(t, repoPath, "merge", "feature-merged", "--no-edit")

	svc := &WorktreeService{}
	merged, err := svc.BranchMerged(repoPath, "main", "feature-merged")

	if err != nil {
		t.Fatalf("BranchMerged returned unexpected error: %v", err)
	}
	if !merged {
		t.Error("BranchMerged = false, want true for a branch merged into default")
	}
}

// TestBranchMerged_Unmerged verifies BranchMerged returns false (with no
// error) for a branch with a commit not yet present in the default branch.
func TestBranchMerged_Unmerged(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	runGitInRepo(t, repoPath, "checkout", "-b", "feature-unmerged")
	createCommit(t, repoPath, "feature.txt", "feature content", "Feature commit")
	runGitInRepo(t, repoPath, "checkout", "main")

	svc := &WorktreeService{}
	merged, err := svc.BranchMerged(repoPath, "main", "feature-unmerged")

	if err != nil {
		t.Fatalf("BranchMerged returned unexpected error for unmerged branch: %v", err)
	}
	if merged {
		t.Error("BranchMerged = true, want false for a branch with commits not yet in default")
	}
}

// TestBranchMerged_UnknownBranch verifies BranchMerged returns an error
// (not false-negative silently) when branchName doesn't exist.
func TestBranchMerged_UnknownBranch(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	_, err := svc.BranchMerged(repoPath, "main", "does-not-exist")

	if err == nil {
		t.Error("expected error for unknown branch name")
	}
}

// TestBranchMerged_NonRepoPath verifies BranchMerged errors out for a path
// that isn't a git repository at all.
func TestBranchMerged_NonRepoPath(t *testing.T) {
	t.Parallel()
	tmpDir := filepath.Join("/tmp", "not_git_branch_merged_test")
	os.MkdirAll(tmpDir, 0o755)
	defer os.RemoveAll(tmpDir)

	svc := &WorktreeService{}
	_, err := svc.BranchMerged(tmpDir, "main", "feature")

	if err == nil {
		t.Error("expected error for non-git directory")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected 'not a git repository' error, got: %v", err)
	}
}
