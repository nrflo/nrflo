package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestListCommits_FallsBackToParentRepo verifies that ListCommits resolves a child
// subdirectory to its parent git repository.
func TestListCommits_FallsBackToParentRepo(t *testing.T) {
	t.Parallel()
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	childDir := filepath.Join(repoPath, "child")
	if err := os.MkdirAll(childDir, 0755); err != nil {
		t.Fatalf("failed to create child dir: %v", err)
	}

	svc := &GitService{}
	commits, total, err := svc.ListCommits(childDir, "main", 1, 20)
	if err != nil {
		t.Fatalf("ListCommits from child dir failed: %v", err)
	}
	if total == 0 {
		t.Error("expected commits from parent repo, got total=0")
	}
	if len(commits) == 0 {
		t.Error("expected commit list to be non-empty when called from child subdir")
	}
}

// TestGetCommitDetail_FallsBackToParentRepo verifies that GetCommitDetail resolves
// a child subdirectory to its parent git repository.
func TestGetCommitDetail_FallsBackToParentRepo(t *testing.T) {
	t.Parallel()
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	childDir := filepath.Join(repoPath, "child")
	if err := os.MkdirAll(childDir, 0755); err != nil {
		t.Fatalf("failed to create child dir: %v", err)
	}

	svc := &GitService{}
	commits, _, err := svc.ListCommits(repoPath, "main", 1, 1)
	if err != nil {
		t.Fatalf("ListCommits failed: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("no commits found in repo")
	}

	detail, err := svc.GetCommitDetail(childDir, commits[0].Hash)
	if err != nil {
		t.Fatalf("GetCommitDetail from child dir failed: %v", err)
	}
	if detail.Hash != commits[0].Hash {
		t.Errorf("GetCommitDetail returned wrong hash: got %s, want %s", detail.Hash, commits[0].Hash)
	}
}

// TestResolveRepoPath_StopsAtOneLevel verifies that resolveRepoPath does not
// walk more than one level up from the given path.
func TestResolveRepoPath_StopsAtOneLevel(t *testing.T) {
	t.Parallel()
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a path 3 levels deep: repo/a/b/c
	// resolveRepoPath only checks one level up (repo/a/b), not repo itself.
	deepChild := filepath.Join(repoPath, "a", "b", "c")
	if err := os.MkdirAll(deepChild, 0755); err != nil {
		t.Fatalf("failed to create deep child dir: %v", err)
	}

	_, err := resolveRepoPath(deepChild)
	if err == nil {
		t.Error("expected error for path 3 levels below repo root, got nil")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected 'not a git repository' in error, got: %v", err)
	}
}
