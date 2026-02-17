package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// setupGitRepo creates a test git repo with commits for worktree testing.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	repoPath := filepath.Join("/tmp", "orch_worktree_test_"+t.Name())

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
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	return repoPath
}

// TestWorktreeSetup_EnabledWithDefaultBranch verifies worktree creation when enabled.
func TestWorktreeSetup_EnabledWithDefaultBranch(t *testing.T) {
	gitRepo := setupGitRepo(t)
	defer os.RemoveAll(gitRepo)

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	projectID := "worktree-project"
	ticketID := "ticket-123"

	// Create project
	projectSvc := service.NewProjectService(pool, clock.Real())
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:          "Test Project",
		RootPath:      gitRepo,
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Update project to enable worktrees
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	trueVal := true
	projectRepo := repo.NewProjectRepo(database, clock.Real())
	err = projectRepo.Update(projectID, &repo.ProjectUpdateFields{
		UseGitWorktrees: &trueVal,
	})
	if err != nil {
		t.Fatalf("failed to update project: %v", err)
	}

	// Verify project settings
	project, err := projectRepo.Get(projectID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}

	if !project.UseGitWorktrees {
		t.Error("UseGitWorktrees should be true")
	}

	if !project.DefaultBranch.Valid || project.DefaultBranch.String != "main" {
		t.Errorf("DefaultBranch should be 'main', got: %v", project.DefaultBranch)
	}

	// Test setupWorktree
	wt, effectiveRoot, err := setupWorktree(project, gitRepo, ticketID, "ticket")
	if err != nil {
		t.Fatalf("setupWorktree failed: %v", err)
	}

	// Verify worktree info
	if wt == nil {
		t.Fatal("worktreeInfo should not be nil when worktrees enabled")
	}

	if wt.projectRoot != gitRepo {
		t.Errorf("projectRoot mismatch: got %s, want %s", wt.projectRoot, gitRepo)
	}

	if wt.branchName != ticketID {
		t.Errorf("branchName should be ticket ID, got: %s", wt.branchName)
	}

	if wt.defaultBranch != "main" {
		t.Errorf("defaultBranch should be 'main', got: %s", wt.defaultBranch)
	}

	// Verify effectiveRoot is worktree path, not original
	if effectiveRoot == gitRepo {
		t.Error("effectiveRoot should be worktree path, not original project root")
	}

	if effectiveRoot != wt.worktreePath {
		t.Errorf("effectiveRoot mismatch: got %s, want %s", effectiveRoot, wt.worktreePath)
	}

	// Verify worktree exists
	if _, err := os.Stat(wt.worktreePath); err != nil {
		t.Errorf("worktree path does not exist: %v", err)
	}

	// Verify branch exists
	cmd := exec.Command("git", "branch", "--list", ticketID)
	cmd.Dir = gitRepo
	out, _ := cmd.Output()
	if !strings.Contains(string(out), ticketID) {
		t.Errorf("branch %s not found", ticketID)
	}

	// Clean up
	wtSvc := &service.WorktreeService{}
	wtSvc.Cleanup(wt.projectRoot, wt.branchName, wt.worktreePath)
}

// TestWorktreeSetup_DisabledWhenFlagFalse verifies no worktree when disabled.
func TestWorktreeSetup_DisabledWhenFlagFalse(t *testing.T) {
	gitRepo := setupGitRepo(t)
	defer os.RemoveAll(gitRepo)

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	projectID := "no-worktree-project"
	ticketID := "ticket-456"

	// Create project with worktrees disabled (default)
	projectSvc := service.NewProjectService(pool, clock.Real())
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:     "Test Project",
		RootPath: gitRepo,
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	projectRepo := repo.NewProjectRepo(database, clock.Real())
	project, err := projectRepo.Get(projectID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}

	wt, effectiveRoot, err := setupWorktree(project, gitRepo, ticketID, "ticket")
	if err != nil {
		t.Fatalf("setupWorktree failed: %v", err)
	}

	// Verify worktree is nil
	if wt != nil {
		t.Error("worktreeInfo should be nil when worktrees disabled")
	}

	// Verify effectiveRoot is original project root
	if effectiveRoot != gitRepo {
		t.Errorf("effectiveRoot should be original project root when disabled, got: %s", effectiveRoot)
	}
}

// TestWorktreeSetup_DisabledWhenNoDefaultBranch verifies no worktree when no default branch.
func TestWorktreeSetup_DisabledWhenNoDefaultBranch(t *testing.T) {
	gitRepo := setupGitRepo(t)
	defer os.RemoveAll(gitRepo)

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	projectID := "no-default-branch-project"
	ticketID := "ticket-789"

	// Create project with worktrees enabled but no default branch
	projectSvc := service.NewProjectService(pool, clock.Real())
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:     "Test Project",
		RootPath: gitRepo,
		// DefaultBranch intentionally omitted
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	// Enable worktrees without setting default branch
	trueVal := true
	projectRepo := repo.NewProjectRepo(database, clock.Real())
	err = projectRepo.Update(projectID, &repo.ProjectUpdateFields{
		UseGitWorktrees: &trueVal,
	})
	if err != nil {
		t.Fatalf("failed to update project: %v", err)
	}

	project, err := projectRepo.Get(projectID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}

	wt, effectiveRoot, err := setupWorktree(project, gitRepo, ticketID, "ticket")
	if err != nil {
		t.Fatalf("setupWorktree failed: %v", err)
	}

	// Verify worktree is nil
	if wt != nil {
		t.Error("worktreeInfo should be nil when no default branch")
	}

	// Verify effectiveRoot is original project root
	if effectiveRoot != gitRepo {
		t.Errorf("effectiveRoot should be original project root, got: %s", effectiveRoot)
	}
}

// TestWorktreeSetup_ProjectScope verifies that project-scoped workflows skip worktree creation.
func TestWorktreeSetup_ProjectScope(t *testing.T) {
	gitRepo := setupGitRepo(t)
	defer os.RemoveAll(gitRepo)

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	projectID := "project-scope-test"

	// Create project with worktrees enabled
	projectSvc := service.NewProjectService(pool, clock.Real())
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:          "Test Project",
		RootPath:      gitRepo,
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	trueVal := true
	projectRepo := repo.NewProjectRepo(database, clock.Real())
	err = projectRepo.Update(projectID, &repo.ProjectUpdateFields{
		UseGitWorktrees: &trueVal,
	})
	if err != nil {
		t.Fatalf("failed to update project: %v", err)
	}

	project, err := projectRepo.Get(projectID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}

	// Project scope should skip worktree even when UseGitWorktrees=true
	wt, effectiveRoot, err := setupWorktree(project, gitRepo, "unused-branch", "project")
	if err != nil {
		t.Fatalf("setupWorktree failed: %v", err)
	}

	if wt != nil {
		t.Error("worktreeInfo should be nil for project-scoped workflows")
	}

	if effectiveRoot != gitRepo {
		t.Errorf("effectiveRoot should be original project root for project scope, got: %s", effectiveRoot)
	}
}

// TestRunLoop_WorktreeCleanupOnFailure verifies worktree cleanup when workflow fails.
func TestRunLoop_WorktreeCleanupOnFailure(t *testing.T) {
	gitRepo := setupGitRepo(t)
	defer os.RemoveAll(gitRepo)

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	projectID := "cleanup-test"
	ticketID := "ticket-fail-cleanup"

	// Create project
	projectSvc := service.NewProjectService(pool, clock.Real())
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:          "Test Project",
		RootPath:      gitRepo,
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Update project to enable worktrees
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	trueVal := true
	projectRepo := repo.NewProjectRepo(database, clock.Real())
	err = projectRepo.Update(projectID, &repo.ProjectUpdateFields{
		UseGitWorktrees: &trueVal,
	})
	if err != nil {
		t.Fatalf("failed to update project: %v", err)
	}

	// Setup worktree manually to verify cleanup
	project, err := projectRepo.Get(projectID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}

	wt, _, err := setupWorktree(project, gitRepo, ticketID, "ticket")
	if err != nil {
		t.Fatalf("setupWorktree failed: %v", err)
	}

	// Verify worktree exists
	if _, err := os.Stat(wt.worktreePath); err != nil {
		t.Fatalf("worktree should exist: %v", err)
	}

	// Simulate workflow failure by calling Cleanup
	wtSvc := &service.WorktreeService{}
	err = wtSvc.Cleanup(wt.projectRoot, wt.branchName, wt.worktreePath)
	if err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}

	// Verify worktree is removed
	if _, err := os.Stat(wt.worktreePath); err == nil {
		t.Error("worktree should be removed after cleanup")
	}

	// Verify branch is removed
	cmd := exec.Command("git", "branch", "--list", ticketID)
	cmd.Dir = gitRepo
	out, _ := cmd.Output()
	if strings.Contains(string(out), ticketID) {
		t.Errorf("branch %s should be removed", ticketID)
	}
}

// TestRunLoop_WorktreeMergeOnSuccess verifies worktree merge when workflow succeeds.
func TestRunLoop_WorktreeMergeOnSuccess(t *testing.T) {
	gitRepo := setupGitRepo(t)
	defer os.RemoveAll(gitRepo)

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	projectID := "merge-test"
	ticketID := "ticket-merge"

	// Create project
	projectSvc := service.NewProjectService(pool, clock.Real())
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:          "Test Project",
		RootPath:      gitRepo,
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Update project to enable worktrees
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	trueVal := true
	projectRepo := repo.NewProjectRepo(database, clock.Real())
	err = projectRepo.Update(projectID, &repo.ProjectUpdateFields{
		UseGitWorktrees: &trueVal,
	})
	if err != nil {
		t.Fatalf("failed to update project: %v", err)
	}

	project, err := projectRepo.Get(projectID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}

	wt, worktreePath, err := setupWorktree(project, gitRepo, ticketID, "ticket")
	if err != nil {
		t.Fatalf("setupWorktree failed: %v", err)
	}

	// Make changes in worktree
	changeFile := filepath.Join(worktreePath, "change.txt")
	if err := os.WriteFile(changeFile, []byte("success change"), 0o644); err != nil {
		t.Fatalf("failed to write change file: %v", err)
	}

	cmd := exec.Command("git", "add", "change.txt")
	cmd.Dir = worktreePath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Add change from worktree")
	cmd.Dir = worktreePath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Simulate workflow success by calling MergeAndCleanup
	wtSvc := &service.WorktreeService{}
	err = wtSvc.MergeAndCleanup(wt.projectRoot, wt.defaultBranch, wt.branchName, wt.worktreePath)
	if err != nil {
		t.Fatalf("MergeAndCleanup failed: %v", err)
	}

	// Verify changes were merged into main
	mainChangePath := filepath.Join(gitRepo, "change.txt")
	content, err := os.ReadFile(mainChangePath)
	if err != nil {
		t.Errorf("merged file not found in main: %v", err)
	}
	if string(content) != "success change" {
		t.Errorf("merged file content mismatch: got %s, want 'success change'", string(content))
	}

	// Verify worktree is removed
	if _, err := os.Stat(wt.worktreePath); err == nil {
		t.Error("worktree should be removed after merge")
	}

	// Verify branch is removed
	cmd = exec.Command("git", "branch", "--list", ticketID)
	cmd.Dir = gitRepo
	out, _ := cmd.Output()
	if strings.Contains(string(out), ticketID) {
		t.Errorf("branch %s should be removed after merge", ticketID)
	}
}

// TestRunLoop_WorktreeMergeConflict verifies branch preservation on merge conflict.
func TestRunLoop_WorktreeMergeConflict(t *testing.T) {
	gitRepo := setupGitRepo(t)
	defer os.RemoveAll(gitRepo)

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	projectID := "conflict-test"
	ticketID := "ticket-conflict"

	// Create project
	projectSvc := service.NewProjectService(pool, clock.Real())
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:          "Test Project",
		RootPath:      gitRepo,
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Update project to enable worktrees
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	trueVal := true
	projectRepo := repo.NewProjectRepo(database, clock.Real())
	err = projectRepo.Update(projectID, &repo.ProjectUpdateFields{
		UseGitWorktrees: &trueVal,
	})
	if err != nil {
		t.Fatalf("failed to update project: %v", err)
	}

	project, err := projectRepo.Get(projectID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}

	wt, worktreePath, err := setupWorktree(project, gitRepo, ticketID, "ticket")
	if err != nil {
		t.Fatalf("setupWorktree failed: %v", err)
	}

	// Create conflicting changes in main
	mainFile := filepath.Join(gitRepo, "test.txt")
	if err := os.WriteFile(mainFile, []byte("main change"), 0o644); err != nil {
		t.Fatalf("failed to write to main: %v", err)
	}

	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = gitRepo
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Main change")
	cmd.Dir = gitRepo
	cmd.Run()

	// Create conflicting changes in worktree
	wtFile := filepath.Join(worktreePath, "test.txt")
	if err := os.WriteFile(wtFile, []byte("worktree change"), 0o644); err != nil {
		t.Fatalf("failed to write to worktree: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = worktreePath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Worktree change")
	cmd.Dir = worktreePath
	cmd.Run()

	// Attempt merge (should fail with conflict)
	wtSvc := &service.WorktreeService{}
	err = wtSvc.MergeAndCleanup(wt.projectRoot, wt.defaultBranch, wt.branchName, wt.worktreePath)

	if err == nil {
		t.Fatal("expected error for merge conflict")
	}

	// Verify error message contains branch name
	if !strings.Contains(err.Error(), ticketID) {
		t.Errorf("error should contain branch name, got: %v", err)
	}

	// Verify branch still exists (for manual resolution)
	cmd = exec.Command("git", "branch", "--list", ticketID)
	cmd.Dir = gitRepo
	out, _ := cmd.Output()
	if !strings.Contains(string(out), ticketID) {
		t.Errorf("branch %s should be preserved after conflict", ticketID)
	}

	// Worktree is removed before merge attempt (worktree removal happens first
	// to avoid lock contention; branch is preserved for manual resolution)
	if _, err := os.Stat(wt.worktreePath); err == nil {
		t.Error("worktree should be removed before merge attempt")
	}

	// Clean up manually
	wtSvc.Cleanup(wt.projectRoot, wt.branchName, wt.worktreePath)
}

// TestWorktreeCleanup_Idempotent verifies cleanup can be called multiple times.
func TestWorktreeCleanup_Idempotent(t *testing.T) {
	gitRepo := setupGitRepo(t)
	defer os.RemoveAll(gitRepo)

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	projectID := "idempotent-test"
	ticketID := "ticket-idempotent"

	// Create project
	projectSvc := service.NewProjectService(pool, clock.Real())
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:          "Test Project",
		RootPath:      gitRepo,
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Update project to enable worktrees
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	trueVal := true
	projectRepo := repo.NewProjectRepo(database, clock.Real())
	err = projectRepo.Update(projectID, &repo.ProjectUpdateFields{
		UseGitWorktrees: &trueVal,
	})
	if err != nil {
		t.Fatalf("failed to update project: %v", err)
	}

	project, err := projectRepo.Get(projectID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}

	wt, _, err := setupWorktree(project, gitRepo, ticketID, "ticket")
	if err != nil {
		t.Fatalf("setupWorktree failed: %v", err)
	}

	wtSvc := &service.WorktreeService{}

	// First cleanup
	err = wtSvc.Cleanup(wt.projectRoot, wt.branchName, wt.worktreePath)
	if err != nil {
		t.Errorf("first cleanup failed: %v", err)
	}

	// Give cleanup a moment to complete
	time.Sleep(100 * time.Millisecond)

	// Second cleanup (should not error)
	err = wtSvc.Cleanup(wt.projectRoot, wt.branchName, wt.worktreePath)
	if err != nil {
		t.Errorf("second cleanup should be idempotent, got error: %v", err)
	}

	// Third cleanup (should not error)
	err = wtSvc.Cleanup(wt.projectRoot, wt.branchName, wt.worktreePath)
	if err != nil {
		t.Errorf("third cleanup should be idempotent, got error: %v", err)
	}
}
