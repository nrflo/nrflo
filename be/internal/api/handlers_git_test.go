package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/db"
)

// setupGitHandlerTestDB creates a test database with a project that has a git repo.
func setupGitHandlerTestDB(t *testing.T) (*db.Pool, string, string) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "git_handler_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	// Create a test git repository
	repoPath := filepath.Join("/tmp", "git_handler_test_repo_"+t.Name())
	os.RemoveAll(repoPath)
	os.MkdirAll(repoPath, 0755)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		pool.Close()
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

	// Create some commits
	createTestCommit(t, repoPath, "test.txt", "content", "Test commit")

	projectID := "test-project"
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = pool.Exec(`
		INSERT INTO projects (id, name, root_path, default_branch, created_at, updated_at)
		VALUES (?, 'Test Project', ?, 'main', ?, ?)`,
		strings.ToLower(projectID), repoPath, now, now)
	if err != nil {
		pool.Close()
		t.Fatalf("failed to create test project: %v", err)
	}

	return pool, projectID, repoPath
}

func createTestCommit(t *testing.T, repoPath, filename, content, message string) {
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

// TestHandleListGitCommits_DefaultPagination verifies default page and per_page values.
func TestHandleListGitCommits_DefaultPagination(t *testing.T) {
	pool, projectID, repoPath := setupGitHandlerTestDB(t)
	defer pool.Close()
	defer os.RemoveAll(repoPath)

	server := &Server{dataPath: pool.Path}
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits", nil)
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()

	server.handleListGitCommits(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["page"].(float64) != 1 {
		t.Errorf("expected default page=1, got %v", response["page"])
	}

	if response["per_page"].(float64) != 20 {
		t.Errorf("expected default per_page=20, got %v", response["per_page"])
	}

	commits, ok := response["commits"].([]interface{})
	if !ok {
		t.Fatal("commits field is not an array")
	}

	if len(commits) != 1 {
		t.Errorf("expected 1 commit, got %d", len(commits))
	}
}

// TestHandleListGitCommits_CustomPagination verifies custom page and per_page values.
func TestHandleListGitCommits_CustomPagination(t *testing.T) {
	pool, projectID, repoPath := setupGitHandlerTestDB(t)
	defer pool.Close()
	defer os.RemoveAll(repoPath)

	// Create more commits
	for i := 1; i <= 25; i++ {
		createTestCommit(t, repoPath, "file"+string(rune(i))+".txt", "content", "Commit "+string(rune(i)))
	}

	server := &Server{dataPath: pool.Path}
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits?page=2&per_page=10", nil)
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()

	server.handleListGitCommits(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["page"].(float64) != 2 {
		t.Errorf("expected page=2, got %v", response["page"])
	}

	if response["per_page"].(float64) != 10 {
		t.Errorf("expected per_page=10, got %v", response["per_page"])
	}

	commits, ok := response["commits"].([]interface{})
	if !ok {
		t.Fatal("commits field is not an array")
	}

	if len(commits) != 10 {
		t.Errorf("expected 10 commits, got %d", len(commits))
	}

	// Verify total count
	if response["total"].(float64) != 26 {
		t.Errorf("expected total=26, got %v", response["total"])
	}
}

// TestHandleListGitCommits_PerPageCappedAt100 verifies per_page is capped at 100.
func TestHandleListGitCommits_PerPageCappedAt100(t *testing.T) {
	pool, projectID, repoPath := setupGitHandlerTestDB(t)
	defer pool.Close()
	defer os.RemoveAll(repoPath)

	server := &Server{dataPath: pool.Path}
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits?per_page=500", nil)
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()

	server.handleListGitCommits(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["per_page"].(float64) != 100 {
		t.Errorf("expected per_page capped at 100, got %v", response["per_page"])
	}
}

// TestHandleListGitCommits_MissingRootPath verifies 400 error for missing root_path.
func TestHandleListGitCommits_MissingRootPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	projectID := "test-project-no-root"
	now := time.Now().UTC().Format(time.RFC3339)

	// Create project without root_path
	_, err = pool.Exec(`
		INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, 'Test Project', ?, ?)`,
		strings.ToLower(projectID), now, now)
	if err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}

	server := &Server{dataPath: pool.Path}
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits", nil)
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()

	server.handleListGitCommits(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	if !strings.Contains(rr.Body.String(), "no root_path configured") {
		t.Errorf("expected 'no root_path configured' error, got: %s", rr.Body.String())
	}
}

// TestHandleListGitCommits_ProjectNotFound verifies 404 for nonexistent project.
func TestHandleListGitCommits_ProjectNotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	server := &Server{dataPath: pool.Path}
	req := httptest.NewRequest("GET", "/api/v1/projects/nonexistent/git/commits", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	server.handleListGitCommits(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleListGitCommits_UsesDefaultBranch verifies default_branch is used if set.
func TestHandleListGitCommits_UsesDefaultBranch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	// Create a test git repository with master branch
	repoPath := filepath.Join("/tmp", "git_handler_master_test_"+t.Name())
	os.RemoveAll(repoPath)
	os.MkdirAll(repoPath, 0755)
	defer os.RemoveAll(repoPath)

	cmd := exec.Command("git", "init", "-b", "master")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = repoPath
	cmd.Run()
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = repoPath
	cmd.Run()
	createTestCommit(t, repoPath, "test.txt", "content", "Test commit on master")

	projectID := "test-project-master"
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = pool.Exec(`
		INSERT INTO projects (id, name, root_path, default_branch, created_at, updated_at)
		VALUES (?, 'Test Project', ?, 'master', ?, ?)`,
		strings.ToLower(projectID), repoPath, now, now)
	if err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}

	server := &Server{dataPath: pool.Path}
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits", nil)
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()

	server.handleListGitCommits(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleGetGitCommitDetail_HappyPath verifies commit detail endpoint.
func TestHandleGetGitCommitDetail_HappyPath(t *testing.T) {
	pool, projectID, repoPath := setupGitHandlerTestDB(t)
	defer pool.Close()
	defer os.RemoveAll(repoPath)

	// Create a second commit so we're not testing against the root commit
	createTestCommit(t, repoPath, "test2.txt", "content2", "Second test commit")

	// Get the latest commit hash
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	hashBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}
	hash := strings.TrimSpace(string(hashBytes))

	server := &Server{dataPath: pool.Path}
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits/"+hash, nil)
	req.SetPathValue("id", projectID)
	req.SetPathValue("hash", hash)
	rr := httptest.NewRecorder()

	server.handleGetGitCommitDetail(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	commit, ok := response["commit"].(map[string]interface{})
	if !ok {
		t.Fatalf("commit field is not an object: %T %+v", response["commit"], response)
	}

	if commit["hash"] != hash {
		t.Errorf("expected hash %s, got %v", hash, commit["hash"])
	}

	if commit["author"] != "Test User" {
		t.Errorf("expected author 'Test User', got %v", commit["author"])
	}

	files, ok := commit["files"].([]interface{})
	if !ok {
		t.Fatalf("files field is not an array: %T %+v", commit["files"], commit)
	}

	if len(files) == 0 {
		t.Error("expected at least one file")
	}

	diff, ok := commit["diff"].(string)
	if !ok {
		t.Fatal("diff field is not a string")
	}

	if diff == "" {
		t.Error("expected diff to be non-empty")
	}
}

// TestHandleGetGitCommitDetail_ShortHash verifies short hash works.
func TestHandleGetGitCommitDetail_ShortHash(t *testing.T) {
	pool, projectID, repoPath := setupGitHandlerTestDB(t)
	defer pool.Close()
	defer os.RemoveAll(repoPath)

	// Get the commit hash
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = repoPath
	hashBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}
	shortHash := strings.TrimSpace(string(hashBytes))

	server := &Server{dataPath: pool.Path}
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits/"+shortHash, nil)
	req.SetPathValue("id", projectID)
	req.SetPathValue("hash", shortHash)
	rr := httptest.NewRecorder()

	server.handleGetGitCommitDetail(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleGetGitCommitDetail_InvalidHashFormat verifies 400 for non-hex hash.
func TestHandleGetGitCommitDetail_InvalidHashFormat(t *testing.T) {
	pool, projectID, repoPath := setupGitHandlerTestDB(t)
	defer pool.Close()
	defer os.RemoveAll(repoPath)

	testCases := []struct {
		name string
		hash string
	}{
		{"non-hex chars", "xyz123"},
		{"too short", "abc"},
		{"with special chars", "abc;123"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := &Server{dataPath: pool.Path}
			req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits/"+tc.hash, nil)
			req.SetPathValue("id", projectID)
			req.SetPathValue("hash", tc.hash)
			rr := httptest.NewRecorder()

			server.handleGetGitCommitDetail(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
			}

			if !strings.Contains(rr.Body.String(), "invalid commit hash format") {
				t.Errorf("expected 'invalid commit hash format' error, got: %s", rr.Body.String())
			}
		})
	}
}

// TestHandleGetGitCommitDetail_MissingRootPath verifies 400 for missing root_path.
func TestHandleGetGitCommitDetail_MissingRootPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	projectID := "test-project-no-root"
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = pool.Exec(`
		INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, 'Test Project', ?, ?)`,
		strings.ToLower(projectID), now, now)
	if err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}

	server := &Server{dataPath: pool.Path}
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits/abc123", nil)
	req.SetPathValue("id", projectID)
	req.SetPathValue("hash", "abc123")
	rr := httptest.NewRecorder()

	server.handleGetGitCommitDetail(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	if !strings.Contains(rr.Body.String(), "no root_path configured") {
		t.Errorf("expected 'no root_path configured' error, got: %s", rr.Body.String())
	}
}

// TestHandleGetGitCommitDetail_ProjectNotFound verifies 404 for nonexistent project.
func TestHandleGetGitCommitDetail_ProjectNotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	server := &Server{dataPath: pool.Path}
	req := httptest.NewRequest("GET", "/api/v1/projects/nonexistent/git/commits/abc123", nil)
	req.SetPathValue("id", "nonexistent")
	req.SetPathValue("hash", "abc123")
	rr := httptest.NewRecorder()

	server.handleGetGitCommitDetail(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGitEndToEnd_FullFlow verifies the complete flow from listing to detail.
func TestGitEndToEnd_FullFlow(t *testing.T) {
	pool, projectID, repoPath := setupGitHandlerTestDB(t)
	defer pool.Close()
	defer os.RemoveAll(repoPath)

	// Create multiple commits for a more realistic test
	createTestCommit(t, repoPath, "file1.txt", "content1", "Add file1")
	createTestCommit(t, repoPath, "file2.txt", "content2", "Add file2")

	// Modify file1
	filePath := filepath.Join(repoPath, "file1.txt")
	os.WriteFile(filePath, []byte("modified content"), 0644)
	cmd := exec.Command("git", "add", "file1.txt")
	cmd.Dir = repoPath
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Modify file1")
	cmd.Dir = repoPath
	cmd.Run()

	server := &Server{dataPath: pool.Path}

	// Step 1: List commits
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits", nil)
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()

	server.handleListGitCommits(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("ListCommits failed: %d: %s", rr.Code, rr.Body.String())
	}

	var listResponse map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&listResponse); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}

	commits, ok := listResponse["commits"].([]interface{})
	if !ok || len(commits) == 0 {
		t.Fatal("no commits returned")
	}

	// Step 2: Get detail for the first commit
	firstCommit := commits[0].(map[string]interface{})
	hash := firstCommit["hash"].(string)

	req = httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits/"+hash, nil)
	req.SetPathValue("id", projectID)
	req.SetPathValue("hash", hash)
	rr = httptest.NewRecorder()

	server.handleGetGitCommitDetail(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GetCommitDetail failed: %d: %s", rr.Code, rr.Body.String())
	}

	var detailResponse map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&detailResponse); err != nil {
		t.Fatalf("failed to decode detail response: %v", err)
	}

	commit, ok := detailResponse["commit"].(map[string]interface{})
	if !ok {
		t.Fatal("commit field missing in detail response")
	}

	// Verify the detail has more information than the list
	if _, hasFiles := commit["files"]; !hasFiles {
		t.Error("detail response missing files field")
	}

	if _, hasDiff := commit["diff"]; !hasDiff {
		t.Error("detail response missing diff field")
	}

	// Verify hash matches
	if commit["hash"] != hash {
		t.Errorf("hash mismatch: expected %s, got %v", hash, commit["hash"])
	}
}

// TestHandleListGitCommits_NullDefaultBranch verifies main is used when default_branch is null.
func TestHandleListGitCommits_NullDefaultBranch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repoPath := filepath.Join("/tmp", "git_null_branch_test_"+t.Name())
	os.RemoveAll(repoPath)
	os.MkdirAll(repoPath, 0755)
	defer os.RemoveAll(repoPath)

	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

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

	createTestCommit(t, repoPath, "test.txt", "content", "Test commit")

	projectID := "test-project-null-branch"
	now := time.Now().UTC().Format(time.RFC3339)

	// Insert with NULL default_branch using sql.NullString
	_, err = pool.Exec(`
		INSERT INTO projects (id, name, root_path, default_branch, created_at, updated_at)
		VALUES (?, 'Test Project', ?, ?, ?, ?)`,
		strings.ToLower(projectID), repoPath, sql.NullString{Valid: false}, now, now)
	if err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}

	server := &Server{dataPath: pool.Path}
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/git/commits", nil)
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()

	server.handleListGitCommits(rr, req)

	// Should use "main" as default and succeed
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
