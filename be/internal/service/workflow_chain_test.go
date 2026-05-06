package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupWFChainSvcEnv creates a DB with project + workflow def + chain service.
// Returns pool, chainSvc, runSvc, projectID, and a valid workflowName.
func setupWFChainSvcEnv(t *testing.T) (*db.Pool, *WorkflowChainService, *WorkflowChainRunService, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "wfc_svc.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	clk := clock.Real()
	projectID := "proj-wfchain"

	projectSvc := NewProjectService(pool, clk)
	if _, err := projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:     "WF Chain Test",
		RootPath: t.TempDir(),
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	workflowSvc := NewWorkflowService(pool, clk)
	const wfName = "feature"
	if _, err := workflowSvc.CreateWorkflowDef(projectID, &types.WorkflowDefCreateRequest{
		ID: wfName,
	}); err != nil {
		t.Fatalf("create workflow def: %v", err)
	}

	chainSvc := NewWorkflowChainService(pool, clk, workflowSvc)
	runSvc := NewWorkflowChainRunService(pool, clk)
	return pool, chainSvc, runSvc, projectID, wfName
}

// TestWorkflowChainService_CreateChain_EmptySteps verifies that a chain with no steps
// is accepted at create time (rejected only at run time).
func TestWorkflowChainService_CreateChain_EmptySteps(t *testing.T) {
	t.Parallel()
	_, chainSvc, _, projectID, _ := setupWFChainSvcEnv(t)

	chain, err := chainSvc.CreateChain(projectID, &types.WorkflowChainCreateRequest{
		Name: "empty chain",
	})
	if err != nil {
		t.Fatalf("CreateChain(empty steps) = %v, want nil", err)
	}
	if chain == nil {
		t.Fatal("CreateChain returned nil")
	}
	if chain.ID == "" {
		t.Error("chain ID is empty")
	}
	if chain.Name != "empty chain" {
		t.Errorf("Name = %q, want empty chain", chain.Name)
	}
	if len(chain.Steps) != 0 {
		t.Errorf("len(Steps) = %d, want 0", len(chain.Steps))
	}
}

// TestWorkflowChainService_GetChain_EmptySteps verifies GetChain returns a non-nil
// empty slice (not nil) for a chain with no steps.
func TestWorkflowChainService_GetChain_EmptySteps(t *testing.T) {
	t.Parallel()
	_, chainSvc, _, projectID, _ := setupWFChainSvcEnv(t)

	created, err := chainSvc.CreateChain(projectID, &types.WorkflowChainCreateRequest{
		Name: "empty",
	})
	if err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	got, err := chainSvc.GetChain(projectID, created.ID)
	if err != nil {
		t.Fatalf("GetChain: %v", err)
	}
	if got == nil {
		t.Fatal("GetChain returned nil")
	}
	if got.Steps == nil {
		t.Error("GetChain Steps is nil, want empty slice")
	}
	if len(got.Steps) != 0 {
		t.Errorf("len(Steps) = %d, want 0", len(got.Steps))
	}
}

// TestWorkflowChainService_CreateChain_ValidationRejects checks that validation still
// runs and rejects invalid step configurations when steps are provided.
func TestWorkflowChainService_CreateChain_ValidationRejects(t *testing.T) {
	t.Parallel()
	_, chainSvc, _, projectID, wfName := setupWFChainSvcEnv(t)

	cases := []struct {
		name    string
		steps   []types.WorkflowChainStepRequest
		wantErr string
	}{
		{
			name: "step 0 not project scope",
			steps: []types.WorkflowChainStepRequest{
				{WorkflowName: wfName, ScopeType: "ticket"},
			},
			wantErr: "step 0",
		},
		{
			name: "unknown workflow name",
			steps: []types.WorkflowChainStepRequest{
				{WorkflowName: "no-such-workflow", ScopeType: "project"},
			},
			wantErr: "workflow not found",
		},
		{
			name: "require_ticket_handoff on project scope",
			steps: []types.WorkflowChainStepRequest{
				{WorkflowName: wfName, ScopeType: "project", RequireTicketHandoff: true},
			},
			wantErr: "require_ticket_handoff",
		},
		{
			name: "second step unknown workflow",
			steps: []types.WorkflowChainStepRequest{
				{WorkflowName: wfName, ScopeType: "project"},
				{WorkflowName: "bogus", ScopeType: "ticket"},
			},
			wantErr: "workflow not found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := chainSvc.CreateChain(projectID, &types.WorkflowChainCreateRequest{
				Name:  "validate-test",
				Steps: tc.steps,
			})
			if err == nil {
				t.Fatalf("CreateChain(%s) = nil, want error containing %q", tc.name, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("CreateChain(%s) error = %q, want containing %q", tc.name, err.Error(), tc.wantErr)
			}
		})
	}
}

// TestWorkflowChainService_CreateChain_WithSteps verifies that a chain with valid steps
// is accepted and persisted correctly.
func TestWorkflowChainService_CreateChain_WithSteps(t *testing.T) {
	t.Parallel()
	_, chainSvc, _, projectID, wfName := setupWFChainSvcEnv(t)

	chain, err := chainSvc.CreateChain(projectID, &types.WorkflowChainCreateRequest{
		Name: "two-step chain",
		Steps: []types.WorkflowChainStepRequest{
			{WorkflowName: wfName, ScopeType: "project"},
			{WorkflowName: wfName, ScopeType: "ticket"},
		},
	})
	if err != nil {
		t.Fatalf("CreateChain = %v, want nil", err)
	}
	if len(chain.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(chain.Steps))
	}
	if chain.Steps[0].Position != 0 {
		t.Errorf("Steps[0].Position = %d, want 0", chain.Steps[0].Position)
	}
	if chain.Steps[1].Position != 1 {
		t.Errorf("Steps[1].Position = %d, want 1", chain.Steps[1].Position)
	}
}

// TestWorkflowChainService_AppendStep_RejectsNonProjectFirstStep verifies that
// appending a non-project step to an empty chain is rejected by validation.
func TestWorkflowChainService_AppendStep_RejectsNonProjectFirstStep(t *testing.T) {
	t.Parallel()
	_, chainSvc, _, projectID, wfName := setupWFChainSvcEnv(t)

	empty, err := chainSvc.CreateChain(projectID, &types.WorkflowChainCreateRequest{
		Name: "empty for append",
	})
	if err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	_, err = chainSvc.AppendStep(projectID, empty.ID, &types.WorkflowChainStepRequest{
		WorkflowName: wfName,
		ScopeType:    "ticket",
	})
	if err == nil {
		t.Fatal("AppendStep(ticket scope on empty chain) = nil, want error")
	}
	if !strings.Contains(err.Error(), "step 0") {
		t.Errorf("AppendStep error = %q, want containing step 0", err.Error())
	}
}

// TestWorkflowChainService_AppendStep_AcceptsProjectFirstStep verifies that
// appending a project-scope step to an empty chain succeeds.
func TestWorkflowChainService_AppendStep_AcceptsProjectFirstStep(t *testing.T) {
	t.Parallel()
	_, chainSvc, _, projectID, wfName := setupWFChainSvcEnv(t)

	empty, err := chainSvc.CreateChain(projectID, &types.WorkflowChainCreateRequest{
		Name: "empty for project-append",
	})
	if err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	result, err := chainSvc.AppendStep(projectID, empty.ID, &types.WorkflowChainStepRequest{
		WorkflowName: wfName,
		ScopeType:    "project",
	})
	if err != nil {
		t.Fatalf("AppendStep(project scope on empty chain) = %v, want nil", err)
	}
	if len(result.Steps) != 1 {
		t.Errorf("len(Steps) = %d, want 1", len(result.Steps))
	}
	if result.Steps[0].Position != 0 {
		t.Errorf("Steps[0].Position = %d, want 0", result.Steps[0].Position)
	}
}

// TestWorkflowChainService_CreateRun_EmptyChainRejectsAtRunTime verifies that
// CreateRun on a chain with no steps is rejected at run time with a "no steps" error.
func TestWorkflowChainService_CreateRun_EmptyChainRejectsAtRunTime(t *testing.T) {
	t.Parallel()
	_, chainSvc, runSvc, projectID, _ := setupWFChainSvcEnv(t)

	empty, err := chainSvc.CreateChain(projectID, &types.WorkflowChainCreateRequest{
		Name: "empty for run",
	})
	if err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	_, err = runSvc.CreateRun(projectID, empty.ID, "", "")
	if err == nil {
		t.Fatal("CreateRun on empty chain = nil, want error")
	}
	if !strings.Contains(err.Error(), "no steps") {
		t.Errorf("CreateRun error = %q, want containing no steps", err.Error())
	}
}

// TestWorkflowChainService_ListChains_Empty verifies that ListChains returns a non-nil
// empty slice when no chains exist.
func TestWorkflowChainService_ListChains_Empty(t *testing.T) {
	t.Parallel()
	_, chainSvc, _, projectID, _ := setupWFChainSvcEnv(t)

	chains, err := chainSvc.ListChains(projectID)
	if err != nil {
		t.Fatalf("ListChains: %v", err)
	}
	if chains == nil {
		t.Error("ListChains returned nil, want empty slice")
	}
	if len(chains) != 0 {
		t.Errorf("len(chains) = %d, want 0", len(chains))
	}
}

// TestWorkflowChainService_DeleteChain verifies that DeleteChain removes a chain.
func TestWorkflowChainService_DeleteChain(t *testing.T) {
	t.Parallel()
	_, chainSvc, _, projectID, _ := setupWFChainSvcEnv(t)

	chain, err := chainSvc.CreateChain(projectID, &types.WorkflowChainCreateRequest{
		Name: "to-delete",
	})
	if err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	if err := chainSvc.DeleteChain(projectID, chain.ID); err != nil {
		t.Fatalf("DeleteChain: %v", err)
	}

	_, err = chainSvc.GetChain(projectID, chain.ID)
	if err == nil {
		t.Fatal("GetChain after delete = nil, want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("GetChain error = %q, want containing not found", err.Error())
	}
}

// TestWorkflowChainService_UpdateChain verifies that chain metadata can be updated.
func TestWorkflowChainService_UpdateChain(t *testing.T) {
	t.Parallel()
	_, chainSvc, _, projectID, _ := setupWFChainSvcEnv(t)

	chain, err := chainSvc.CreateChain(projectID, &types.WorkflowChainCreateRequest{
		Name: "original",
	})
	if err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	newName := "updated"
	newDesc := "new description"
	updated, err := chainSvc.UpdateChain(projectID, chain.ID, &types.WorkflowChainUpdateRequest{
		Name:        &newName,
		Description: &newDesc,
	})
	if err != nil {
		t.Fatalf("UpdateChain: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("Name = %q, want %q", updated.Name, newName)
	}
	if updated.Description != newDesc {
		t.Errorf("Description = %q, want %q", updated.Description, newDesc)
	}
}

// TestWorkflowChainService_CreateChain_RequiresName verifies that a missing name is rejected.
func TestWorkflowChainService_CreateChain_RequiresName(t *testing.T) {
	t.Parallel()
	_, chainSvc, _, projectID, _ := setupWFChainSvcEnv(t)

	_, err := chainSvc.CreateChain(projectID, &types.WorkflowChainCreateRequest{})
	if err == nil {
		t.Fatal("CreateChain(no name) = nil, want error")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error = %q, want containing name", err.Error())
	}
}
