package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// setupWFChainRunEnv creates a DB with project + workflow chain + 2 steps.
// Returns the service, pool, projectID, and chainID.
func setupWFChainRunEnv(t *testing.T) (*WorkflowChainRunService, *db.Pool, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "wfcr_svc.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	clk := clock.Real()
	projectID := "proj-wfcr"

	projectSvc := NewProjectService(pool, clk)
	if _, err := projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:     "Chain Run Test",
		RootPath: t.TempDir(),
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	chainID := "chain-wfcr"
	chainRepo := repo.NewWorkflowChainRepo(pool, clk)
	if err := chainRepo.CreateChain(&model.WorkflowChain{
		ID:          chainID,
		ProjectID:   projectID,
		Name:        "WF Chain Run Test",
		Description: "test",
	}); err != nil {
		t.Fatalf("create chain: %v", err)
	}

	stepRepo := repo.NewWorkflowChainStepRepo(pool, clk)
	for i, scopeType := range []string{"project", "ticket"} {
		if err := stepRepo.UpsertStep(&model.WorkflowChainStep{
			ID:           fmt.Sprintf("step-wfcr-%d", i),
			ProjectID:    projectID,
			ChainID:      chainID,
			Position:     i,
			WorkflowName: "feature",
			ScopeType:    scopeType,
		}); err != nil {
			t.Fatalf("upsert step %d: %v", i, err)
		}
	}

	svc := NewWorkflowChainRunService(pool, clk)
	return svc, pool, projectID, chainID
}

func TestWorkflowChainRunService_CreateRun_HappyPath(t *testing.T) {
	t.Parallel()
	svc, _, projectID, chainID := setupWFChainRunEnv(t)

	detail, err := svc.CreateRun(projectID, chainID, "test instructions", "user@test.com")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if detail == nil {
		t.Fatal("CreateRun returned nil detail")
	}
	if detail.ID == "" {
		t.Error("run ID is empty")
	}
	if detail.Status != "pending" {
		t.Errorf("Status = %q, want pending", detail.Status)
	}
	if detail.InitialInstructions != "test instructions" {
		t.Errorf("InitialInstructions = %q, want test instructions", detail.InitialInstructions)
	}
	if detail.TriggeredBy != "user@test.com" {
		t.Errorf("TriggeredBy = %q, want user@test.com", detail.TriggeredBy)
	}
	if len(detail.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(detail.Steps))
	}
	if detail.Steps[0].Position != 0 {
		t.Errorf("Steps[0].Position = %d, want 0", detail.Steps[0].Position)
	}
	if detail.Steps[1].Position != 1 {
		t.Errorf("Steps[1].Position = %d, want 1", detail.Steps[1].Position)
	}
	for _, s := range detail.Steps {
		if s.Status != "pending" {
			t.Errorf("Step %d Status = %q, want pending", s.Position, s.Status)
		}
	}
}

func TestWorkflowChainRunService_CreateRun_ChainNotFound(t *testing.T) {
	t.Parallel()
	svc, _, projectID, _ := setupWFChainRunEnv(t)

	_, err := svc.CreateRun(projectID, "no-such-chain", "", "")
	if err == nil {
		t.Fatal("expected error for missing chain, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestWorkflowChainRunService_CreateRun_NoSteps(t *testing.T) {
	t.Parallel()
	svc, pool, projectID, _ := setupWFChainRunEnv(t)

	// Create a chain with no steps
	emptyChainID := "chain-empty-steps"
	chainRepo := repo.NewWorkflowChainRepo(pool, clock.Real())
	if err := chainRepo.CreateChain(&model.WorkflowChain{
		ID:        emptyChainID,
		ProjectID: projectID,
		Name:      "Empty",
	}); err != nil {
		t.Fatalf("create empty chain: %v", err)
	}

	_, err := svc.CreateRun(projectID, emptyChainID, "", "")
	if err == nil {
		t.Fatal("expected error for chain with no steps, got nil")
	}
	if !strings.Contains(err.Error(), "no steps") {
		t.Errorf("expected no steps error, got: %v", err)
	}
}

func TestWorkflowChainRunService_GetRunDetail_HappyPath(t *testing.T) {
	t.Parallel()
	svc, _, projectID, chainID := setupWFChainRunEnv(t)

	created, err := svc.CreateRun(projectID, chainID, "instructions", "bot")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	detail, err := svc.GetRunDetail(projectID, created.ID)
	if err != nil {
		t.Fatalf("GetRunDetail: %v", err)
	}
	if detail.ID != created.ID {
		t.Errorf("ID = %q, want %q", detail.ID, created.ID)
	}
	if len(detail.Steps) != 2 {
		t.Errorf("len(Steps) = %d, want 2", len(detail.Steps))
	}
}

func TestWorkflowChainRunService_GetRunDetail_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, projectID, _ := setupWFChainRunEnv(t)

	_, err := svc.GetRunDetail(projectID, "no-such-run")
	if err == nil {
		t.Fatal("expected error for missing run, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestWorkflowChainRunService_GetRunDetail_WrongProject(t *testing.T) {
	t.Parallel()
	svc, _, projectID, chainID := setupWFChainRunEnv(t)

	created, err := svc.CreateRun(projectID, chainID, "", "")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	_, err = svc.GetRunDetail("other-project", created.ID)
	if err == nil {
		t.Fatal("expected error for wrong project, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestWorkflowChainRunService_ListRuns_ByProject(t *testing.T) {
	t.Parallel()
	svc, _, projectID, chainID := setupWFChainRunEnv(t)

	for range 3 {
		if _, err := svc.CreateRun(projectID, chainID, "", ""); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
	}

	runs, err := svc.ListRuns(projectID, "")
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("len(runs) = %d, want 3", len(runs))
	}

	noRuns, err := svc.ListRuns("other-project", "")
	if err != nil {
		t.Fatalf("ListRuns other-project: %v", err)
	}
	if len(noRuns) != 0 {
		t.Errorf("len(noRuns) = %d, want 0", len(noRuns))
	}
}

func TestWorkflowChainRunService_ListRuns_ChainFilter(t *testing.T) {
	t.Parallel()
	svc, pool, projectID, chainID := setupWFChainRunEnv(t)

	// Create a second chain with steps
	chain2ID := "chain-wfcr-2"
	chainRepo := repo.NewWorkflowChainRepo(pool, clock.Real())
	if err := chainRepo.CreateChain(&model.WorkflowChain{
		ID:        chain2ID,
		ProjectID: projectID,
		Name:      "Chain 2",
	}); err != nil {
		t.Fatalf("create chain2: %v", err)
	}
	stepRepo := repo.NewWorkflowChainStepRepo(pool, clock.Real())
	if err := stepRepo.UpsertStep(&model.WorkflowChainStep{
		ID:           "step-wfcr-2-0",
		ProjectID:    projectID,
		ChainID:      chain2ID,
		Position:     0,
		WorkflowName: "feature",
		ScopeType:    "project",
	}); err != nil {
		t.Fatalf("upsert step chain2: %v", err)
	}

	if _, err := svc.CreateRun(projectID, chainID, "", ""); err != nil {
		t.Fatalf("CreateRun chain1: %v", err)
	}
	if _, err := svc.CreateRun(projectID, chain2ID, "", ""); err != nil {
		t.Fatalf("CreateRun chain2: %v", err)
	}

	runs, err := svc.ListRuns(projectID, chainID)
	if err != nil {
		t.Fatalf("ListRuns filtered: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if !strings.EqualFold(runs[0].ChainID, chainID) {
		t.Errorf("ChainID = %q, want %q", runs[0].ChainID, chainID)
	}
}

func TestWorkflowChainRunService_SetNextStepInstructions(t *testing.T) {
	t.Parallel()
	svc, pool, projectID, chainID := setupWFChainRunEnv(t)

	detail, err := svc.CreateRun(projectID, chainID, "init", "")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Assign a workflow_instance_id to step 0
	const instanceID = "wfi-svc-step0"
	rr := repo.NewWorkflowChainRunRepo(pool, clock.Real())
	if err := rr.SetRunStepInstance(detail.Steps[0].ID, instanceID, "", ""); err != nil {
		t.Fatalf("SetRunStepInstance: %v", err)
	}

	if err := svc.SetNextStepInstructions(instanceID, "handoff instructions"); err != nil {
		t.Fatalf("SetNextStepInstructions: %v", err)
	}

	// Verify step 1 has updated instructions
	updated, err := svc.GetRunDetail(projectID, detail.ID)
	if err != nil {
		t.Fatalf("GetRunDetail: %v", err)
	}
	if updated.Steps[1].InstructionsUsed != "handoff instructions" {
		t.Errorf("Steps[1].InstructionsUsed = %q, want handoff instructions", updated.Steps[1].InstructionsUsed)
	}
}

func TestWorkflowChainRunService_SetNextStepInstructions_UnknownInstance(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := setupWFChainRunEnv(t)

	if err := svc.SetNextStepInstructions("no-such-instance", "instructions"); err == nil {
		t.Fatal("expected error for unknown instance, got nil")
	}
}

func TestWorkflowChainRunService_SetNextStepTicket(t *testing.T) {
	t.Parallel()
	svc, pool, projectID, chainID := setupWFChainRunEnv(t)

	detail, err := svc.CreateRun(projectID, chainID, "init", "")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	const instanceID = "wfi-svc-ticket-step0"
	rr := repo.NewWorkflowChainRunRepo(pool, clock.Real())
	if err := rr.SetRunStepInstance(detail.Steps[0].ID, instanceID, "", ""); err != nil {
		t.Fatalf("SetRunStepInstance: %v", err)
	}

	if err := svc.SetNextStepTicket(instanceID, "TICKET-123"); err != nil {
		t.Fatalf("SetNextStepTicket: %v", err)
	}

	steps, err := rr.ListRunSteps(detail.ID)
	if err != nil {
		t.Fatalf("ListRunSteps: %v", err)
	}
	if !steps[1].TicketID.Valid || steps[1].TicketID.String != "TICKET-123" {
		t.Errorf("Steps[1].TicketID = %v, want TICKET-123", steps[1].TicketID)
	}
}

func TestWorkflowChainRunService_SetNextStepTicket_UnknownInstance(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := setupWFChainRunEnv(t)

	if err := svc.SetNextStepTicket("no-such-instance", "TICKET-1"); err == nil {
		t.Fatal("expected error for unknown instance, got nil")
	}
}
