package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupBareRemote creates a bare git repo and configures it as origin for repoPath.
// Pushes the current main branch state to the bare repo.
// Returns the path to the bare repo.
func setupBareRemote(t *testing.T, repoPath string) string {
	t.Helper()
	safeName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	barePath := filepath.Join("/tmp", "bare_"+safeName)
	os.RemoveAll(barePath)
	t.Cleanup(func() { os.RemoveAll(barePath) })

	cmd := exec.Command("git", "init", "--bare", barePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to init bare repo: %v: %s", err, out)
	}

	cmd = exec.Command("git", "remote", "add", "origin", barePath)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to add remote: %v: %s", err, out)
	}

	cmd = exec.Command("git", "push", "origin", "main")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to push to bare: %v: %s", err, out)
	}

	return barePath
}

// addRemoteCommit clones barePath into a temp dir, creates a commit, and pushes it back.
// Clones explicitly with --branch main to ensure the main branch is checked out.
func addRemoteCommit(t *testing.T, barePath, filename, content, message string) {
	t.Helper()
	safeName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	clonePath := filepath.Join("/tmp", "clone_"+safeName)
	os.RemoveAll(clonePath)
	defer os.RemoveAll(clonePath)

	cmd := exec.Command("git", "clone", "--branch", "main", barePath, clonePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to clone: %v: %s", err, out)
	}
	for _, args := range [][]string{
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
	} {
		c := exec.Command("git", args...)
		c.Dir = clonePath
		c.Run()
	}

	createCommit(t, clonePath, filename, content, message)

	cmd = exec.Command("git", "push", "origin", "main")
	cmd.Dir = clonePath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to push remote commit: %v: %s", err, out)
	}
}

// TestSetup_NoRemote_BestEffort verifies Setup succeeds when no remote is configured.
// fetchRemote failure must be non-fatal.
func TestSetup_NoRemote_BestEffort(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	t.Cleanup(func() { os.RemoveAll(repoPath) })

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "no-remote-branch")
	if err != nil {
		t.Errorf("Setup should succeed without remote, got: %v", err)
	}
	t.Cleanup(func() { svc.Cleanup(repoPath, "no-remote-branch", worktreePath) })

	if !branchExists(t, repoPath, "no-remote-branch") {
		t.Error("branch should exist after Setup without remote")
	}
	if !worktreeExists(worktreePath) {
		t.Errorf("worktree should exist at %s", worktreePath)
	}
}

// TestMergeAndCleanup_NoRemote_BestEffort verifies MergeAndCleanup succeeds when
// no remote is configured — fetch and rev-list failures are non-fatal.
func TestMergeAndCleanup_NoRemote_BestEffort(t *testing.T) {
	orig := mergeRetryDelay
	mergeRetryDelay = 0
	t.Cleanup(func() { mergeRetryDelay = orig })

	repoPath := setupWorktreeTestRepo(t)
	t.Cleanup(func() { os.RemoveAll(repoPath) })

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "no-remote-merge")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	createCommit(t, worktreePath, "no_remote_work.txt", "work", "Work without remote")

	err = svc.MergeAndCleanup(repoPath, "main", "no-remote-merge", worktreePath)
	if err != nil {
		t.Errorf("MergeAndCleanup should succeed without remote, got: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoPath, "no_remote_work.txt")); err != nil {
		t.Error("merged file should be present in main")
	}
}

// cleanWorktree removes a worktree directory pre-emptively to avoid stale-state failures.
func cleanWorktree(t *testing.T, branchName string) {
	t.Helper()
	p := filepath.Join(worktreeBasePath, branchName)
	os.RemoveAll(p)
	t.Cleanup(func() { os.RemoveAll(p) })
}

// TestMergeAndCleanup_RemoteAhead_FastForwardsAndRebases verifies the full remote-ahead
// flow: fetch, fast-forward local default branch, rebase feature branch, then merge.
func TestMergeAndCleanup_RemoteAhead_FastForwardsAndRebases(t *testing.T) {
	orig := mergeRetryDelay
	mergeRetryDelay = 0
	t.Cleanup(func() { mergeRetryDelay = orig })

	repoPath := setupWorktreeTestRepo(t)
	t.Cleanup(func() { os.RemoveAll(repoPath) })

	cleanWorktree(t, "feature-remote-ahead")
	barePath := setupBareRemote(t, repoPath)

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "feature-remote-ahead")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	t.Cleanup(func() { svc.Cleanup(repoPath, "feature-remote-ahead", worktreePath) })

	// Commit work in the feature worktree
	createCommit(t, worktreePath, "feature_work.txt", "feature result", "Feature work")

	// Simulate upstream activity: remote main gets a new commit
	addRemoteCommit(t, barePath, "upstream_change.txt", "upstream", "Upstream commit")

	// MergeAndCleanup: should fetch, fast-forward, rebase, then merge
	err = svc.MergeAndCleanup(repoPath, "main", "feature-remote-ahead", worktreePath)
	if err != nil {
		t.Errorf("MergeAndCleanup failed: %v", err)
	}

	// Both upstream change and feature work should be in main
	if _, err := os.Stat(filepath.Join(repoPath, "upstream_change.txt")); err != nil {
		t.Error("upstream change should be in main after fast-forward")
	}
	if _, err := os.Stat(filepath.Join(repoPath, "feature_work.txt")); err != nil {
		t.Error("feature work should be in main after merge")
	}

	// Branch should be deleted
	if branchExists(t, repoPath, "feature-remote-ahead") {
		t.Error("feature branch should be deleted after successful merge")
	}
}

// TestMergeAndCleanup_RebaseConflict_RemoteAhead verifies that when remote defaultBranch
// and the feature branch both modify the same file, the rebase fails, the branch is
// preserved, and a descriptive error is returned.
func TestMergeAndCleanup_RebaseConflict_RemoteAhead(t *testing.T) {
	orig := mergeRetryDelay
	mergeRetryDelay = 0
	t.Cleanup(func() { mergeRetryDelay = orig })

	repoPath := setupWorktreeTestRepo(t)
	t.Cleanup(func() { os.RemoveAll(repoPath) })

	cleanWorktree(t, "rebase-conflict-branch")
	barePath := setupBareRemote(t, repoPath)

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "rebase-conflict-branch")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	// Branch preserved on conflict — also ensure cleanup on test exit
	t.Cleanup(func() { svc.Cleanup(repoPath, "rebase-conflict-branch", worktreePath) })

	// Feature branch modifies file1.txt
	createCommit(t, worktreePath, "file1.txt", "feature change", "Feature: change file1")

	// Remote main also modifies file1.txt (conflicts with feature branch)
	addRemoteCommit(t, barePath, "file1.txt", "remote change", "Remote: change file1")

	// MergeAndCleanup should fail at merge (rebase aborts silently, conflict surfaces at merge)
	err = svc.MergeAndCleanup(repoPath, "main", "rebase-conflict-branch", worktreePath)
	if err == nil {
		t.Fatal("expected merge conflict error, got nil")
	}

	if !strings.Contains(err.Error(), "merge failed") {
		t.Errorf("error should contain 'merge failed', got: %v", err)
	}

	// Branch must be preserved for manual resolution
	if !branchExists(t, repoPath, "rebase-conflict-branch") {
		t.Error("branch should be preserved after rebase conflict")
	}

	// Worktree is removed before rebase attempt (existing behavior)
	if worktreeExists(worktreePath) {
		t.Error("worktree should be removed before rebase attempt")
	}
}

// TestMergeAndCleanup_RemoteUpToDate verifies that when the local branch is already
// at the same commit as the remote, no fast-forward is attempted and the merge proceeds.
func TestMergeAndCleanup_RemoteUpToDate(t *testing.T) {
	orig := mergeRetryDelay
	mergeRetryDelay = 0
	t.Cleanup(func() { mergeRetryDelay = orig })

	repoPath := setupWorktreeTestRepo(t)
	t.Cleanup(func() { os.RemoveAll(repoPath) })

	cleanWorktree(t, "feature-uptodate")
	setupBareRemote(t, repoPath) // push current state; remote is NOT ahead

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "feature-uptodate")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	t.Cleanup(func() { svc.Cleanup(repoPath, "feature-uptodate", worktreePath) })

	createCommit(t, worktreePath, "uptodate_work.txt", "work", "Feature work")

	err = svc.MergeAndCleanup(repoPath, "main", "feature-uptodate", worktreePath)
	if err != nil {
		t.Errorf("MergeAndCleanup failed when remote up-to-date: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoPath, "uptodate_work.txt")); err != nil {
		t.Error("feature work should be in main")
	}
	if branchExists(t, repoPath, "feature-uptodate") {
		t.Error("branch should be deleted after successful merge")
	}
}
