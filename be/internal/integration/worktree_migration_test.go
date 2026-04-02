package integration

import (
	"encoding/json"
	"testing"

	"be/internal/repo"
	"be/internal/types"
)

// TestMigration041WorktreeColumnsExist verifies migration 000041 added
// worktree_path and branch_name columns to workflow_instances.
func TestMigration041WorktreeColumnsExist(t *testing.T) {
	env := NewTestEnv(t)

	for _, col := range []string{"worktree_path", "branch_name"} {
		var count int
		err := env.Pool.QueryRow(`
			SELECT COUNT(*) FROM pragma_table_info('workflow_instances')
			WHERE name = ?`, col).Scan(&count)
		if err != nil {
			t.Fatalf("schema query for column %s: %v", col, err)
		}
		if count != 1 {
			t.Errorf("column %s should exist in workflow_instances after migration 000041, found %d", col, count)
		}
	}
}

// TestBuildV4State_IncludesWorktreeFields verifies that worktree_path and branch_name
// appear in the GetStatus response when set on the workflow instance.
func TestBuildV4State_IncludesWorktreeFields(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WK041-1", "Worktree fields in status")
	env.InitWorkflow(t, "WK041-1")
	wfiID := env.GetWorkflowInstanceID(t, "WK041-1", "test")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, env.Clock)
	if err := wfiRepo.UpdateWorktree(wfiID, "/tmp/nrflow/worktrees/WK041-1", "WK041-1"); err != nil {
		t.Fatalf("UpdateWorktree: %v", err)
	}

	statusRaw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "WK041-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	data, _ := json.Marshal(statusRaw)
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}

	wtp, ok := result["worktree_path"].(string)
	if !ok {
		t.Errorf("worktree_path missing or not a string in response: %v", result["worktree_path"])
	} else if wtp != "/tmp/nrflow/worktrees/WK041-1" {
		t.Errorf("worktree_path = %q, want /tmp/nrflow/worktrees/WK041-1", wtp)
	}

	bn, ok := result["branch_name"].(string)
	if !ok {
		t.Errorf("branch_name missing or not a string in response: %v", result["branch_name"])
	} else if bn != "WK041-1" {
		t.Errorf("branch_name = %q, want WK041-1", bn)
	}
}

// TestBuildV4State_OmitsWorktreeFieldsWhenNull verifies that worktree_path and branch_name
// are absent from GetStatus when not set (i.e. NULL in DB).
func TestBuildV4State_OmitsWorktreeFieldsWhenNull(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WK041-2", "Null worktree fields omitted")
	env.InitWorkflow(t, "WK041-2")

	statusRaw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "WK041-2", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	data, _ := json.Marshal(statusRaw)
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}

	if v, ok := result["worktree_path"]; ok {
		t.Errorf("worktree_path should be absent from response when NULL, got: %v", v)
	}
	if v, ok := result["branch_name"]; ok {
		t.Errorf("branch_name should be absent from response when NULL, got: %v", v)
	}
}

// TestWorktreeFieldsPersistAcrossReads verifies that worktree fields survive DB round-trips
// and are consistently returned on repeated GetStatus calls.
func TestWorktreeFieldsPersistAcrossReads(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WK041-3", "Persistence across reads")
	env.InitWorkflow(t, "WK041-3")
	wfiID := env.GetWorkflowInstanceID(t, "WK041-3", "test")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, env.Clock)
	if err := wfiRepo.UpdateWorktree(wfiID, "/tmp/nrflow/worktrees/WK041-3", "WK041-3"); err != nil {
		t.Fatalf("UpdateWorktree: %v", err)
	}

	for i := 0; i < 3; i++ {
		statusRaw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "WK041-3", &types.WorkflowGetRequest{
			Workflow: "test",
		})
		if err != nil {
			t.Fatalf("GetStatus iteration %d: %v", i, err)
		}
		data, _ := json.Marshal(statusRaw)
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		if result["worktree_path"] != "/tmp/nrflow/worktrees/WK041-3" {
			t.Errorf("read %d: worktree_path = %v, want /tmp/nrflow/worktrees/WK041-3", i, result["worktree_path"])
		}
		if result["branch_name"] != "WK041-3" {
			t.Errorf("read %d: branch_name = %v, want WK041-3", i, result["branch_name"])
		}
	}
}

// TestWorktreeFieldsMarshalJSON verifies WorkflowInstance JSON marshaling conditionally
// includes worktree_path and branch_name based on whether the fields are valid (non-NULL).
func TestWorktreeFieldsMarshalJSON(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WK041-4", "MarshalJSON conditional fields")
	env.InitWorkflow(t, "WK041-4")
	wfiID := env.GetWorkflowInstanceID(t, "WK041-4", "test")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, env.Clock)

	// Before UpdateWorktree: fields should be absent in JSON.
	wi, err := wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("Get before update: %v", err)
	}
	data, _ := json.Marshal(wi)
	var before map[string]interface{}
	json.Unmarshal(data, &before)
	if v, ok := before["worktree_path"]; ok {
		t.Errorf("worktree_path should be absent in JSON before UpdateWorktree, got: %v", v)
	}
	if v, ok := before["branch_name"]; ok {
		t.Errorf("branch_name should be absent in JSON before UpdateWorktree, got: %v", v)
	}

	// After UpdateWorktree: fields should be present in JSON.
	if err := wfiRepo.UpdateWorktree(wfiID, "/tmp/nrflow/worktrees/WK041-4", "WK041-4"); err != nil {
		t.Fatalf("UpdateWorktree: %v", err)
	}
	wi2, err := wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	data2, _ := json.Marshal(wi2)
	var after map[string]interface{}
	json.Unmarshal(data2, &after)
	if after["worktree_path"] != "/tmp/nrflow/worktrees/WK041-4" {
		t.Errorf("worktree_path in JSON = %v, want /tmp/nrflow/worktrees/WK041-4", after["worktree_path"])
	}
	if after["branch_name"] != "WK041-4" {
		t.Errorf("branch_name in JSON = %v, want WK041-4", after["branch_name"])
	}
}
