package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestGitRepo creates a temporary git repository with test commits for testing.
func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	repoPath := filepath.Join("/tmp", "git_test_repo_"+t.Name())

	// Clean up any existing test repo
	os.RemoveAll(repoPath)

	// Create directory
	if err := os.MkdirAll(repoPath, 0755); err != nil {
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

	// Create first commit
	createCommit(t, repoPath, "file1.txt", "content1", "First commit")
	createCommit(t, repoPath, "file2.txt", "content2", "Second commit")
	createCommit(t, repoPath, "file3.txt", "content3", "Third commit")

	// Create a commit with pipe character in message to test delimiter handling
	createCommit(t, repoPath, "file4.txt", "content4", "Fourth commit | with pipe")

	// Create a commit with modification
	modifyCommit(t, repoPath, "file1.txt", "modified content", "Modified file1")

	// Create a commit with deletion
	deleteCommit(t, repoPath, "file2.txt", "Deleted file2")

	// Create a commit with renamed file
	renameCommit(t, repoPath, "file3.txt", "file3_renamed.txt", "Renamed file3")

	return repoPath
}

func createCommit(t *testing.T, repoPath, filename, content, message string) {
	t.Helper()
	filePath := filepath.Join(repoPath, filename)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file %s: %v", filename, err)
	}

	cmd := exec.Command("git", "add", filename)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add %s: %v", filename, err)
	}

	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}
}

func modifyCommit(t *testing.T, repoPath, filename, content, message string) {
	t.Helper()
	filePath := filepath.Join(repoPath, filename)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to modify file %s: %v", filename, err)
	}

	cmd := exec.Command("git", "add", filename)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add %s: %v", filename, err)
	}

	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}
}

func deleteCommit(t *testing.T, repoPath, filename, message string) {
	t.Helper()
	cmd := exec.Command("git", "rm", filename)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git rm %s: %v", filename, err)
	}

	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}
}

func renameCommit(t *testing.T, repoPath, oldName, newName, message string) {
	t.Helper()
	cmd := exec.Command("git", "mv", oldName, newName)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git mv %s to %s: %v", oldName, newName, err)
	}

	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}
}

// TestListCommits_HappyPath verifies basic pagination works correctly.
func TestListCommits_HappyPath(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &GitService{}
	commits, total, err := svc.ListCommits(repoPath, "main", 1, 20)

	if err != nil {
		t.Fatalf("ListCommits failed: %v", err)
	}

	if total != 7 {
		t.Errorf("expected total=7, got %d", total)
	}

	if len(commits) != 7 {
		t.Errorf("expected 7 commits, got %d", len(commits))
	}

	// Verify first commit (most recent)
	if commits[0].Message != "Renamed file3" {
		t.Errorf("expected first commit message 'Renamed file3', got '%s'", commits[0].Message)
	}

	// Verify commit with pipe character in message
	found := false
	for _, c := range commits {
		if c.Message == "Fourth commit | with pipe" {
			found = true
			break
		}
	}
	if !found {
		t.Error("commit with pipe character in message not found")
	}

	// Verify all commits have required fields
	for i, c := range commits {
		if c.Hash == "" {
			t.Errorf("commit %d has empty Hash", i)
		}
		if c.ShortHash == "" {
			t.Errorf("commit %d has empty ShortHash", i)
		}
		if c.Author != "Test User" {
			t.Errorf("commit %d has author '%s', expected 'Test User'", i, c.Author)
		}
		if c.AuthorEmail != "test@example.com" {
			t.Errorf("commit %d has email '%s', expected 'test@example.com'", i, c.AuthorEmail)
		}
		if c.Date == "" {
			t.Errorf("commit %d has empty Date", i)
		}
		if c.Message == "" {
			t.Errorf("commit %d has empty Message", i)
		}
	}
}

// TestListCommits_Pagination verifies page offset works correctly.
func TestListCommits_Pagination(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &GitService{}

	// Get page 1
	page1, total, err := svc.ListCommits(repoPath, "main", 1, 3)
	if err != nil {
		t.Fatalf("ListCommits page 1 failed: %v", err)
	}

	if total != 7 {
		t.Errorf("expected total=7, got %d", total)
	}

	if len(page1) != 3 {
		t.Errorf("expected 3 commits on page 1, got %d", len(page1))
	}

	// Get page 2
	page2, _, err := svc.ListCommits(repoPath, "main", 2, 3)
	if err != nil {
		t.Fatalf("ListCommits page 2 failed: %v", err)
	}

	if len(page2) != 3 {
		t.Errorf("expected 3 commits on page 2, got %d", len(page2))
	}

	// Verify page 1 and page 2 have different commits
	if page1[0].Hash == page2[0].Hash {
		t.Error("page 1 and page 2 have same first commit hash")
	}

	// Get page 3 (should have 1 commit)
	page3, _, err := svc.ListCommits(repoPath, "main", 3, 3)
	if err != nil {
		t.Fatalf("ListCommits page 3 failed: %v", err)
	}

	if len(page3) != 1 {
		t.Errorf("expected 1 commit on page 3, got %d", len(page3))
	}
}

// TestListCommits_InvalidRepoPath verifies error for non-existent repo.
func TestListCommits_InvalidRepoPath(t *testing.T) {
	svc := &GitService{}
	_, _, err := svc.ListCommits("/nonexistent/path", "main", 1, 20)

	if err == nil {
		t.Error("expected error for non-existent repo path")
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' error, got: %v", err)
	}
}

// TestListCommits_NotGitRepo verifies error for non-git directory.
func TestListCommits_NotGitRepo(t *testing.T) {
	tmpDir := filepath.Join("/tmp", "not_git_repo")
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	svc := &GitService{}
	_, _, err := svc.ListCommits(tmpDir, "main", 1, 20)

	if err == nil {
		t.Error("expected error for non-git directory")
	}

	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected 'not a git repository' error, got: %v", err)
	}
}

// TestListCommits_InvalidBranch verifies error for invalid branch name.
func TestListCommits_InvalidBranch(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &GitService{}

	testCases := []struct {
		name   string
		branch string
	}{
		{"empty branch", ""},
		{"branch with semicolon", "main;ls"},
		{"branch with pipe", "main|cat"},
		{"branch with backtick", "main`whoami`"},
		{"branch starting with dash", "-main"},
		{"branch with space", "main branch"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := svc.ListCommits(repoPath, tc.branch, 1, 20)
			if err == nil {
				t.Errorf("expected error for branch '%s'", tc.branch)
			}
		})
	}
}

// TestGetCommitDetail_HappyPath verifies commit detail retrieval.
func TestGetCommitDetail_HappyPath(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &GitService{}

	// Get the latest commit hash
	commits, _, err := svc.ListCommits(repoPath, "main", 1, 1)
	if err != nil {
		t.Fatalf("ListCommits failed: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("no commits found")
	}

	hash := commits[0].Hash

	detail, err := svc.GetCommitDetail(repoPath, hash)
	if err != nil {
		t.Fatalf("GetCommitDetail failed: %v", err)
	}

	// Verify basic fields
	if detail.Hash != hash {
		t.Errorf("expected hash %s, got %s", hash, detail.Hash)
	}
	if detail.Author != "Test User" {
		t.Errorf("expected author 'Test User', got '%s'", detail.Author)
	}
	if detail.Message != "Renamed file3" {
		t.Errorf("expected message 'Renamed file3', got '%s'", detail.Message)
	}

	// Verify files array
	if len(detail.Files) == 0 {
		t.Error("expected files array to be non-empty for rename commit")
	}

	// Verify diff is not empty
	if detail.Diff == "" {
		t.Error("expected diff to be non-empty")
	}
}

// TestGetCommitDetail_ModifiedFile verifies file status and stats for modified file.
func TestGetCommitDetail_ModifiedFile(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &GitService{}

	// Get all commits and find the "Modified file1" commit
	commits, _, err := svc.ListCommits(repoPath, "main", 1, 20)
	if err != nil {
		t.Fatalf("ListCommits failed: %v", err)
	}

	var modifyHash string
	for _, c := range commits {
		if c.Message == "Modified file1" {
			modifyHash = c.Hash
			break
		}
	}
	if modifyHash == "" {
		t.Fatal("'Modified file1' commit not found")
	}

	detail, err := svc.GetCommitDetail(repoPath, modifyHash)
	if err != nil {
		t.Fatalf("GetCommitDetail failed: %v", err)
	}

	// Find file1.txt in the files array
	var file1 *GitChangedFile
	for i := range detail.Files {
		if detail.Files[i].Path == "file1.txt" {
			file1 = &detail.Files[i]
			break
		}
	}

	if file1 == nil {
		t.Fatal("file1.txt not found in changed files")
	}

	if file1.Status != "modified" {
		t.Errorf("expected status 'modified', got '%s'", file1.Status)
	}

	// Verify additions and deletions (original had 1 line, modified has 1 line)
	if file1.Additions != 1 {
		t.Errorf("expected 1 addition, got %d", file1.Additions)
	}
	if file1.Deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", file1.Deletions)
	}
}

// TestGetCommitDetail_DeletedFile verifies file status for deleted file.
func TestGetCommitDetail_DeletedFile(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &GitService{}

	// Get all commits and find the "Deleted file2" commit
	commits, _, err := svc.ListCommits(repoPath, "main", 1, 20)
	if err != nil {
		t.Fatalf("ListCommits failed: %v", err)
	}

	var deleteHash string
	for _, c := range commits {
		if c.Message == "Deleted file2" {
			deleteHash = c.Hash
			break
		}
	}
	if deleteHash == "" {
		t.Fatal("'Deleted file2' commit not found")
	}

	detail, err := svc.GetCommitDetail(repoPath, deleteHash)
	if err != nil {
		t.Fatalf("GetCommitDetail failed: %v", err)
	}

	// Find file2.txt in the files array
	var file2 *GitChangedFile
	for i := range detail.Files {
		if detail.Files[i].Path == "file2.txt" {
			file2 = &detail.Files[i]
			break
		}
	}

	if file2 == nil {
		t.Fatal("file2.txt not found in changed files")
	}

	if file2.Status != "deleted" {
		t.Errorf("expected status 'deleted', got '%s'", file2.Status)
	}
}

// TestGetCommitDetail_RenamedFile verifies file status for renamed file.
func TestGetCommitDetail_RenamedFile(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &GitService{}

	// Get the latest commit (rename commit)
	commits, _, err := svc.ListCommits(repoPath, "main", 1, 1)
	if err != nil {
		t.Fatalf("ListCommits failed: %v", err)
	}

	detail, err := svc.GetCommitDetail(repoPath, commits[0].Hash)
	if err != nil {
		t.Fatalf("GetCommitDetail failed: %v", err)
	}

	// Git may detect the rename as "renamed" or just show it as a separate add/delete.
	// For the purposes of this test, we just verify the file exists in the changeset.
	var renamedFile *GitChangedFile
	for i := range detail.Files {
		if detail.Files[i].Path == "file3_renamed.txt" {
			renamedFile = &detail.Files[i]
			break
		}
	}

	if renamedFile == nil {
		t.Fatal("file3_renamed.txt not found in changed files")
	}

	// Verify status is either "renamed" or "added" (git may detect it differently)
	if renamedFile.Status != "renamed" && renamedFile.Status != "added" {
		t.Errorf("expected status 'renamed' or 'added', got '%s'", renamedFile.Status)
	}
}

// TestGetCommitDetail_AddedFile verifies file status for added file.
func TestGetCommitDetail_AddedFile(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &GitService{}

	// Get all commits and find the "Second commit" which definitely adds a new file
	commits, _, err := svc.ListCommits(repoPath, "main", 1, 20)
	if err != nil {
		t.Fatalf("ListCommits failed: %v", err)
	}

	var secondHash string
	for _, c := range commits {
		if c.Message == "Second commit" {
			secondHash = c.Hash
			break
		}
	}
	if secondHash == "" {
		t.Fatal("'Second commit' commit not found")
	}

	detail, err := svc.GetCommitDetail(repoPath, secondHash)
	if err != nil {
		t.Fatalf("GetCommitDetail failed: %v", err)
	}

	// Find file2.txt in the files array
	var file2 *GitChangedFile
	for i := range detail.Files {
		if detail.Files[i].Path == "file2.txt" {
			file2 = &detail.Files[i]
			break
		}
	}

	if file2 == nil {
		t.Fatal("file2.txt not found in changed files")
	}

	if file2.Status != "added" {
		t.Errorf("expected status 'added', got '%s'", file2.Status)
	}
}

// TestGetCommitDetail_InvalidHash verifies error for invalid hash format.
func TestGetCommitDetail_InvalidHash(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &GitService{}

	testCases := []struct {
		name string
		hash string
	}{
		{"too short", "abc"},
		{"non-hex chars", "xyz123"},
		{"with spaces", "abc def"},
		{"empty", ""},
		{"special chars", "abc;123"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.GetCommitDetail(repoPath, tc.hash)
			if err == nil {
				t.Errorf("expected error for hash '%s'", tc.hash)
			}
			if !strings.Contains(err.Error(), "invalid commit hash") {
				t.Errorf("expected 'invalid commit hash' error, got: %v", err)
			}
		})
	}
}

// TestGetCommitDetail_NonexistentHash verifies error for valid format but nonexistent hash.
func TestGetCommitDetail_NonexistentHash(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &GitService{}

	// Use a valid hex string that doesn't exist
	_, err := svc.GetCommitDetail(repoPath, "1234567890abcdef")

	if err == nil {
		t.Error("expected error for nonexistent commit hash")
	}
}

// TestValidateBranch verifies branch name validation.
func TestValidateBranch(t *testing.T) {
	testCases := []struct {
		name    string
		branch  string
		wantErr bool
	}{
		{"valid main", "main", false},
		{"valid feature", "feature/foo", false},
		{"valid with dash", "fix-123", false},
		{"empty", "", true},
		{"with semicolon", "main;ls", true},
		{"with pipe", "main|cat", true},
		{"with backtick", "main`whoami`", true},
		{"with dollar", "main$var", true},
		{"with ampersand", "main&sleep", true},
		{"with parens", "main()", true},
		{"with braces", "main{}", true},
		{"with space", "main branch", true},
		{"starts with dash", "-main", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBranch(tc.branch)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateBranch(%q) error = %v, wantErr = %v", tc.branch, err, tc.wantErr)
			}
		})
	}
}

// TestValidateHash verifies hash validation.
func TestValidateHash(t *testing.T) {
	testCases := []struct {
		name    string
		hash    string
		wantErr bool
	}{
		{"valid short", "abc123", false},
		{"valid full", "1234567890abcdef1234567890abcdef12345678", false},
		{"valid uppercase", "ABC123", false},
		{"too short", "abc", true},
		{"too long", "1234567890abcdef1234567890abcdef123456789", true},
		{"non-hex", "xyz123", true},
		{"with space", "abc 123", true},
		{"empty", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHash(tc.hash)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateHash(%q) error = %v, wantErr = %v", tc.hash, err, tc.wantErr)
			}
		})
	}
}

// TestValidateRepoPath verifies repo path validation.
func TestValidateRepoPath(t *testing.T) {
	// Create a valid git repo
	validRepo := setupTestGitRepo(t)
	defer os.RemoveAll(validRepo)

	// Create a directory that's not a git repo
	notGitDir := filepath.Join("/tmp", "not_git_"+t.Name())
	os.MkdirAll(notGitDir, 0755)
	defer os.RemoveAll(notGitDir)

	testCases := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid git repo", validRepo, false},
		{"not git repo", notGitDir, true},
		{"nonexistent path", "/nonexistent/path", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRepoPath(tc.path)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateRepoPath(%q) error = %v, wantErr = %v", tc.path, err, tc.wantErr)
			}
		})
	}
}
