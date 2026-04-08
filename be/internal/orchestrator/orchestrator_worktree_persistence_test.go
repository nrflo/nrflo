package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// TestWorktreePersistence_FieldsPopulatedOnInstance verifies the full flow:
// setupWorktree() returns path+branch, UpdateWorktree() persists them, and
// Get() returns the populated fields — mirroring what orchestrator.Start() does.
func TestWorktreePersistence_FieldsPopulatedOnInstance(t *testing.T) {
	gitRepo := setupGitRepo(t)
	defer os.RemoveAll(gitRepo)

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := orchCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	defer pool.Close()

	projectID := "persist-wt-test"
	ticketID := "ticket-persist-wt"

	// Create project with worktrees enabled
	projectSvc := service.NewProjectService(pool, clock.Real())
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:          "Persist WT Project",
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
	if err := projectRepo.Update(projectID, &repo.ProjectUpdateFields{UseGitWorktrees: &trueVal}); err != nil {
		t.Fatalf("failed to enable worktrees: %v", err)
	}

	project, err := projectRepo.Get(projectID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}

	// Step 1: setupWorktree — mirrors what orchestrator.Start() calls first.
	wt, _, err := setupWorktree(project, gitRepo, ticketID, "ticket")
	if err != nil {
		t.Fatalf("setupWorktree: %v", err)
	}
	if wt == nil {
		t.Fatal("expected non-nil worktreeInfo when worktrees enabled")
	}
	defer func() {
		wtSvc := &service.WorktreeService{}
		wtSvc.Cleanup(wt.projectRoot, wt.branchName, wt.worktreePath)
	}()

	// Step 2: create a workflow instance (mirrors wfService.Init in orchestrator.Start).
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, clock.Real())
	wi := &model.WorkflowInstance{
		ID:         "wfi-persist-wt-1",
		ProjectID:  projectID,
		TicketID:   ticketID,
		WorkflowID: "test-workflow",
		ScopeType:  "ticket",
		Status:     model.WorkflowInstanceActive,
		Findings:   "{}",
	}

	// Insert project and workflow records needed for FK constraints.
	_, err = pool.Exec(`INSERT OR IGNORE INTO workflows
		(id, project_id, description, scope_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"test-workflow", projectID, "Test Workflow", "ticket")
	if err != nil {
		t.Fatalf("failed to insert workflow: %v", err)
	}
	if err := wfiRepo.Create(wi); err != nil {
		t.Fatalf("Create workflow instance: %v", err)
	}

	// Step 3: UpdateWorktree — mirrors wfiRepo.UpdateWorktree in orchestrator.Start.
	if err := wfiRepo.UpdateWorktree(wi.ID, wt.worktreePath, wt.branchName); err != nil {
		t.Fatalf("UpdateWorktree: %v", err)
	}

	// Step 4: read back and assert fields are populated.
	got, err := wfiRepo.Get(wi.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if !got.WorktreePath.Valid {
		t.Error("WorktreePath.Valid should be true after orchestrator persists worktree info")
	}
	if got.WorktreePath.String != wt.worktreePath {
		t.Errorf("WorktreePath.String = %q, want %q", got.WorktreePath.String, wt.worktreePath)
	}
	if !got.BranchName.Valid {
		t.Error("BranchName.Valid should be true after orchestrator persists worktree info")
	}
	if got.BranchName.String != wt.branchName {
		t.Errorf("BranchName.String = %q, want %q", got.BranchName.String, wt.branchName)
	}
	if got.BranchName.String != ticketID {
		t.Errorf("BranchName should equal ticketID, got %q want %q", got.BranchName.String, ticketID)
	}
}

// TestWorktreePersistence_ProjectScopeHasNullFields verifies that project-scoped workflows
// (which skip worktree creation) result in NULL worktree fields on the instance.
func TestWorktreePersistence_ProjectScopeHasNullFields(t *testing.T) {
	gitRepo := setupGitRepo(t)
	defer os.RemoveAll(gitRepo)

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := orchCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	defer pool.Close()

	projectID := "proj-scope-persist"

	projectSvc := service.NewProjectService(pool, clock.Real())
	_, err = projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:          "Proj Scope Test",
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
	projectRepo.Update(projectID, &repo.ProjectUpdateFields{UseGitWorktrees: &trueVal})

	project, err := projectRepo.Get(projectID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}

	// setupWorktree with project scope returns nil — no worktree created.
	wt, _, err := setupWorktree(project, gitRepo, "unused-branch", "project")
	if err != nil {
		t.Fatalf("setupWorktree: %v", err)
	}
	if wt != nil {
		t.Fatal("expected nil worktreeInfo for project-scoped workflow")
	}

	// Create workflow instance without worktree info (wt == nil, so orchestrator skips UpdateWorktree).
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, clock.Real())
	_, err = pool.Exec(`INSERT OR IGNORE INTO workflows
		(id, project_id, description, scope_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"proj-workflow", projectID, "Proj Workflow", "project")
	if err != nil {
		t.Fatalf("failed to insert workflow: %v", err)
	}

	wi := &model.WorkflowInstance{
		ID:         "wfi-proj-scope",
		ProjectID:  projectID,
		WorkflowID: "proj-workflow",
		ScopeType:  "project",
		Status:     model.WorkflowInstanceActive,
		Findings:   "{}",
	}
	if err := wfiRepo.Create(wi); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := wfiRepo.Get(wi.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.WorktreePath.Valid {
		t.Errorf("WorktreePath should be NULL for project-scoped instance, got %q", got.WorktreePath.String)
	}
	if got.BranchName.Valid {
		t.Errorf("BranchName should be NULL for project-scoped instance, got %q", got.BranchName.String)
	}
}
