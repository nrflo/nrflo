package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupWorktreeTestRepo creates a test git repo with a main branch and commits.
func setupWorktreeTestRepo(t *testing.T) string {
	t.Helper()
	repoPath := filepath.Join("/tmp", "worktree_test_"+t.Name())

	// Clean up any existing test repo
	os.RemoveAll(repoPath)

	// Create directory
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("failed to create test repo dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = repoPath
	cmd.Run()
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = repoPath
	cmd.Run()

	// Checkout main branch explicitly
	cmd = exec.Command("git", "checkout", "-b", "main")
	cmd.Dir = repoPath
	cmd.Run()

	// Create initial commits
	createCommit(t, repoPath, "file1.txt", "initial content", "Initial commit")
	createCommit(t, repoPath, "file2.txt", "more content", "Second commit")

	return repoPath
}

// worktreeExists checks if a worktree directory exists.
func worktreeExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// branchExists checks if a branch exists in a git repo.
func branchExists(t *testing.T, repoPath, branchName string) bool {
	t.Helper()
	cmd := exec.Command("git", "branch", "--list", branchName)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), branchName)
}

// TestSetup_HappyPath verifies Setup creates branch and worktree.
func TestSetup_HappyPath(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "test-branch")

	if err != nil {
		t.Errorf("Setup failed: %v", err)
	}

	if worktreePath == "" {
		t.Error("Setup returned empty worktree path")
	}

	// Verify worktree directory exists
	if !worktreeExists(worktreePath) {
		t.Errorf("worktree directory does not exist at %s", worktreePath)
	}

	// Verify branch exists
	if !branchExists(t, repoPath, "test-branch") {
		t.Error("branch 'test-branch' was not created")
	}

	// Verify files are present in worktree
	file1Path := filepath.Join(worktreePath, "file1.txt")
	if _, err := os.Stat(file1Path); err != nil {
		t.Errorf("file1.txt not found in worktree: %v", err)
	}

	// Clean up
	svc.Cleanup(repoPath, "test-branch", worktreePath)
}

// TestSetup_AlreadyExists verifies Setup retries after cleanup when branch exists.
func TestSetup_AlreadyExists(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a stale branch manually
	cmd := exec.Command("git", "branch", "stale-branch")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create stale branch: %v", err)
	}

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "stale-branch")

	// Should succeed after cleanup and retry
	if err != nil {
		t.Errorf("Setup failed after retry: %v", err)
	}

	if worktreePath == "" {
		t.Error("Setup returned empty worktree path")
	}

	if !worktreeExists(worktreePath) {
		t.Errorf("worktree directory does not exist at %s", worktreePath)
	}

	// Clean up
	svc.Cleanup(repoPath, "stale-branch", worktreePath)
}

// TestSetup_InvalidRepoPath verifies Setup fails for non-git directory.
func TestSetup_InvalidRepoPath(t *testing.T) {
	tmpDir := filepath.Join("/tmp", "not_git_worktree_test")
	os.MkdirAll(tmpDir, 0o755)
	defer os.RemoveAll(tmpDir)

	svc := &WorktreeService{}
	_, err := svc.Setup(tmpDir, "main", "test-branch")

	if err == nil {
		t.Error("expected error for non-git directory")
	}

	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected 'not a git repository' error, got: %v", err)
	}
}

// TestSetup_InvalidBranch verifies Setup fails for invalid branch names.
func TestSetup_InvalidBranch(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}

	testCases := []struct {
		name          string
		defaultBranch string
		branchName    string
	}{
		{"empty default branch", "", "test-branch"},
		{"empty branch name", "main", ""},
		{"branch with semicolon", "main", "test;branch"},
		{"branch with pipe", "main", "test|branch"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Setup(repoPath, tc.defaultBranch, tc.branchName)
			if err == nil {
				t.Error("expected error for invalid branch name")
			}
		})
	}
}

// TestMergeAndCleanup_HappyPath verifies merge and cleanup succeed.
func TestMergeAndCleanup_HappyPath(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "feature-branch")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Make changes in worktree
	changeFile := filepath.Join(worktreePath, "change.txt")
	if err := os.WriteFile(changeFile, []byte("new content"), 0o644); err != nil {
		t.Fatalf("failed to write change file: %v", err)
	}

	cmd := exec.Command("git", "add", "change.txt")
	cmd.Dir = worktreePath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Add change")
	cmd.Dir = worktreePath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Merge and cleanup
	err = svc.MergeAndCleanup(repoPath, "main", "feature-branch", worktreePath)
	if err != nil {
		t.Errorf("MergeAndCleanup failed: %v", err)
	}

	// Verify worktree is removed
	if worktreeExists(worktreePath) {
		t.Error("worktree directory still exists after cleanup")
	}

	// Verify branch is removed
	if branchExists(t, repoPath, "feature-branch") {
		t.Error("branch still exists after cleanup")
	}

	// Verify changes were merged into main
	mainChangePath := filepath.Join(repoPath, "change.txt")
	if _, err := os.Stat(mainChangePath); err != nil {
		t.Error("merged changes not found in main branch")
	}
}

// TestMergeAndCleanup_Conflict verifies merge conflict handling.
func TestMergeAndCleanup_Conflict(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "conflict-branch")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Create conflicting changes in main
	mainFile := filepath.Join(repoPath, "file1.txt")
	if err := os.WriteFile(mainFile, []byte("main change"), 0o644); err != nil {
		t.Fatalf("failed to write to main: %v", err)
	}

	cmd := exec.Command("git", "add", "file1.txt")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Main change")
	cmd.Dir = repoPath
	cmd.Run()

	// Create conflicting changes in worktree
	wtFile := filepath.Join(worktreePath, "file1.txt")
	if err := os.WriteFile(wtFile, []byte("worktree change"), 0o644); err != nil {
		t.Fatalf("failed to write to worktree: %v", err)
	}

	cmd = exec.Command("git", "add", "file1.txt")
	cmd.Dir = worktreePath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Worktree change")
	cmd.Dir = worktreePath
	cmd.Run()

	// Attempt merge
	err = svc.MergeAndCleanup(repoPath, "main", "conflict-branch", worktreePath)

	// Should return error
	if err == nil {
		t.Error("expected error for merge conflict")
	}

	// Error should contain branch name
	if !strings.Contains(err.Error(), "conflict-branch") {
		t.Errorf("error should contain branch name, got: %v", err)
	}

	// Branch should still exist for manual resolution
	if !branchExists(t, repoPath, "conflict-branch") {
		t.Error("branch should be preserved after merge conflict")
	}

	// Worktree should still exist for manual resolution
	if !worktreeExists(worktreePath) {
		t.Error("worktree should be preserved after merge conflict")
	}

	// Clean up manually
	svc.Cleanup(repoPath, "conflict-branch", worktreePath)
}

// TestMergeAndCleanup_InvalidRepo verifies error for invalid repo.
func TestMergeAndCleanup_InvalidRepo(t *testing.T) {
	svc := &WorktreeService{}
	err := svc.MergeAndCleanup("/nonexistent", "main", "branch", "/tmp/worktree")

	if err == nil {
		t.Error("expected error for invalid repo path")
	}
}

// TestCleanup_HappyPath verifies Cleanup removes worktree and branch.
func TestCleanup_HappyPath(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "cleanup-branch")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Verify they exist
	if !worktreeExists(worktreePath) {
		t.Fatal("worktree should exist before cleanup")
	}
	if !branchExists(t, repoPath, "cleanup-branch") {
		t.Fatal("branch should exist before cleanup")
	}

	// Cleanup
	err = svc.Cleanup(repoPath, "cleanup-branch", worktreePath)
	if err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}

	// Verify they're gone
	if worktreeExists(worktreePath) {
		t.Error("worktree still exists after cleanup")
	}
	if branchExists(t, repoPath, "cleanup-branch") {
		t.Error("branch still exists after cleanup")
	}
}

// TestCleanup_AlreadyGone verifies Cleanup is idempotent.
func TestCleanup_AlreadyGone(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}

	// Cleanup non-existent worktree and branch (should not error)
	err := svc.Cleanup(repoPath, "nonexistent-branch", "/tmp/nonexistent-worktree")
	if err != nil {
		t.Errorf("Cleanup should be idempotent, got error: %v", err)
	}
}

// TestCleanup_OnlyWorktreeGone verifies Cleanup handles partial state.
func TestCleanup_OnlyWorktreeGone(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "partial-branch")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Manually remove worktree directory
	os.RemoveAll(worktreePath)

	// Cleanup should still succeed
	err = svc.Cleanup(repoPath, "partial-branch", worktreePath)
	if err != nil {
		t.Errorf("Cleanup failed when worktree already gone: %v", err)
	}

	// Branch should be removed
	if branchExists(t, repoPath, "partial-branch") {
		t.Error("branch should be removed even if worktree was already gone")
	}
}

// TestWorktreePath verifies worktree path is under /tmp/nrworkflow/worktrees.
func TestWorktreePath(t *testing.T) {
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	worktreePath, err := svc.Setup(repoPath, "main", "path-test-branch")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer svc.Cleanup(repoPath, "path-test-branch", worktreePath)

	expectedPrefix := "/tmp/nrworkflow/worktrees/"
	if !strings.HasPrefix(worktreePath, expectedPrefix) {
		t.Errorf("worktree path should be under %s, got: %s", expectedPrefix, worktreePath)
	}

	// Should contain the branch name
	if !strings.Contains(worktreePath, "path-test-branch") {
		t.Errorf("worktree path should contain branch name, got: %s", worktreePath)
	}
}
