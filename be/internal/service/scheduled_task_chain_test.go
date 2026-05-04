package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// setupScheduledTaskChainTestEnv creates an isolated env with a real WorkflowChainService
// wired to enable chain ID validation in ScheduledTaskService.
func setupScheduledTaskChainTestEnv(t *testing.T) (*ScheduledTaskService, *db.Pool, func()) {
	t.Helper()
	dbPath := t.TempDir() + "/chain_svc_test.db"
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	clk := clock.Real()
	wfSvc := NewWorkflowService(pool, clk)
	wfChainSvc := NewWorkflowChainService(pool, clk, wfSvc)
	svc := NewScheduledTaskService(pool, clk, &mockReloader{}, wfChainSvc)
	return svc, pool, func() { pool.Close() }
}

// insertWorkflowChain inserts a minimal workflow_chain row for chain ID validation tests.
func insertWorkflowChain(t *testing.T, pool *db.Pool, projectID, chainID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(
		`INSERT INTO workflow_chains (id, project_id, name, description, created_at, updated_at) VALUES (?, ?, ?, '', ?, ?)`,
		chainID, projectID, "test-chain-"+chainID, now, now,
	)
	if err != nil {
		t.Fatalf("insertWorkflowChain(%q, %q): %v", projectID, chainID, err)
	}
}

// -- Create with workflow_chain_ids --

// TestScheduledTaskService_Create_ChainIDsOnly_NilChainSvc verifies that when
// wfChainSvc is nil, chain IDs bypass validation and the task is created.
func TestScheduledTaskService_Create_ChainIDsOnly_NilChainSvc(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-chainonly", "wf-chainonly", "project")

	task, err := svc.Create("proj-chainonly", &types.ScheduledTaskCreateRequest{
		ID:               "task-chainonly",
		Name:             "Chain Only",
		CronExpression:   "* * * * *",
		Workflows:        []string{},
		WorkflowChainIDs: []string{"chain-999"},
	})
	if err != nil {
		t.Fatalf("Create with chain IDs only (nil svc): %v", err)
	}
	if task.ID != "task-chainonly" {
		t.Errorf("ID = %q, want task-chainonly", task.ID)
	}
	if len(task.WorkflowChainIDs) != 1 || task.WorkflowChainIDs[0] != "chain-999" {
		t.Errorf("WorkflowChainIDs = %v, want [chain-999]", task.WorkflowChainIDs)
	}
}

// TestScheduledTaskService_Create_BothWorkflowsAndChains_NilChainSvc verifies
// that a task with both workflows and chain IDs is created when wfChainSvc is nil.
func TestScheduledTaskService_Create_BothWorkflowsAndChains_NilChainSvc(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-both", "wf-both", "project")

	task, err := svc.Create("proj-both", &types.ScheduledTaskCreateRequest{
		ID:               "task-both",
		Name:             "Both",
		CronExpression:   "* * * * *",
		Workflows:        []string{"wf-both"},
		WorkflowChainIDs: []string{"chain-a", "chain-b"},
	})
	if err != nil {
		t.Fatalf("Create with both types: %v", err)
	}
	if len(task.Workflows) != 1 || len(task.WorkflowChainIDs) != 2 {
		t.Errorf("Workflows=%v ChainIDs=%v, want 1 workflow and 2 chains", task.Workflows, task.WorkflowChainIDs)
	}
}

// TestScheduledTaskService_Create_EmptyBothWorkflowsAndChains verifies that
// providing neither workflows nor chain IDs returns a workflows_required error.
func TestScheduledTaskService_Create_EmptyBothWorkflowsAndChains(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-empty-both", "wf-empty", "project")

	_, err := svc.Create("proj-empty-both", &types.ScheduledTaskCreateRequest{
		Name:             "Empty Both",
		CronExpression:   "* * * * *",
		Workflows:        []string{},
		WorkflowChainIDs: []string{},
	})
	if err == nil {
		t.Fatal("expected error for empty both fields, got nil")
	}
	if !strings.Contains(err.Error(), "workflows_required") {
		t.Errorf("error = %q, want to contain 'workflows_required'", err.Error())
	}
}

// TestScheduledTaskService_Create_ValidChainID verifies that a valid chain ID
// passes validateChainIDs when wfChainSvc is provided.
func TestScheduledTaskService_Create_ValidChainID(t *testing.T) {
	t.Parallel()
	svc, pool, cleanup := setupScheduledTaskChainTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-valid-chain", "wf-vc", "project")
	insertWorkflowChain(t, pool, "proj-valid-chain", "real-chain-1")

	task, err := svc.Create("proj-valid-chain", &types.ScheduledTaskCreateRequest{
		ID:               "task-valid-chain",
		Name:             "Valid Chain",
		CronExpression:   "* * * * *",
		Workflows:        []string{},
		WorkflowChainIDs: []string{"real-chain-1"},
	})
	if err != nil {
		t.Fatalf("Create with valid chain ID: %v", err)
	}
	if len(task.WorkflowChainIDs) != 1 || task.WorkflowChainIDs[0] != "real-chain-1" {
		t.Errorf("WorkflowChainIDs = %v, want [real-chain-1]", task.WorkflowChainIDs)
	}
}

// TestScheduledTaskService_Create_InvalidChainID verifies that an unknown chain ID
// returns an invalid_chain error when wfChainSvc is provided.
func TestScheduledTaskService_Create_InvalidChainID(t *testing.T) {
	t.Parallel()
	svc, pool, cleanup := setupScheduledTaskChainTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-invalid-chain", "wf-ic", "project")

	_, err := svc.Create("proj-invalid-chain", &types.ScheduledTaskCreateRequest{
		Name:             "Invalid Chain",
		CronExpression:   "* * * * *",
		Workflows:        []string{},
		WorkflowChainIDs: []string{"no-such-chain"},
	})
	if err == nil {
		t.Fatal("Create with invalid chain ID: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_chain") {
		t.Errorf("error = %q, want to contain 'invalid_chain'", err.Error())
	}
}

// -- Update with workflow_chain_ids --

// TestScheduledTaskService_Update_SetWorkflowChainIDs verifies that Update can
// add chain IDs to an existing task.
func TestScheduledTaskService_Update_SetWorkflowChainIDs(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-upd-chain", "wf-upd-chain", "project")

	if _, err := svc.Create("proj-upd-chain", &types.ScheduledTaskCreateRequest{
		ID: "task-upd-chain", Name: "T", CronExpression: "* * * * *",
		Workflows: []string{"wf-upd-chain"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	chains := []string{"chain-x", "chain-y"}
	task, err := svc.Update("task-upd-chain", &types.ScheduledTaskUpdateRequest{
		WorkflowChainIDs: &chains,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(task.WorkflowChainIDs) != 2 {
		t.Errorf("WorkflowChainIDs len = %d, want 2", len(task.WorkflowChainIDs))
	}

	// Verify persisted.
	r := repo.NewScheduledTaskRepo(pool, clock.Real())
	persisted, err := r.Get("task-upd-chain")
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}
	if len(persisted.WorkflowChainIDs) != 2 || persisted.WorkflowChainIDs[0] != "chain-x" {
		t.Errorf("persisted WorkflowChainIDs = %v, want [chain-x chain-y]", persisted.WorkflowChainIDs)
	}
}

// TestScheduledTaskService_Update_ClearWorkflows_ChainIDsRemain verifies that
// clearing the workflows field while keeping chain IDs is valid (at-least-one-of).
func TestScheduledTaskService_Update_ClearWorkflows_ChainIDsRemain(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-clr-wf", "wf-clr", "project")

	// Create with both workflows and chain IDs.
	chains := []string{"chain-keep"}
	if _, err := svc.Create("proj-clr-wf", &types.ScheduledTaskCreateRequest{
		ID: "task-clr-wf", Name: "T", CronExpression: "* * * * *",
		Workflows:        []string{"wf-clr"},
		WorkflowChainIDs: chains,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Clear workflows but keep chain IDs.
	emptyWfs := []string{}
	task, err := svc.Update("task-clr-wf", &types.ScheduledTaskUpdateRequest{
		Workflows: &emptyWfs,
	})
	if err != nil {
		t.Fatalf("Update (clear workflows): %v", err)
	}
	if len(task.Workflows) != 0 {
		t.Errorf("Workflows = %v, want empty", task.Workflows)
	}
	if len(task.WorkflowChainIDs) != 1 || task.WorkflowChainIDs[0] != "chain-keep" {
		t.Errorf("WorkflowChainIDs = %v, want [chain-keep]", task.WorkflowChainIDs)
	}
}

// TestScheduledTaskService_Update_ClearBothFields_ReturnsError verifies that
// clearing both workflows and chain IDs simultaneously returns workflows_required.
func TestScheduledTaskService_Update_ClearBothFields_ReturnsError(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-clr-both", "wf-clr-both", "project")

	if _, err := svc.Create("proj-clr-both", &types.ScheduledTaskCreateRequest{
		ID: "task-clr-both", Name: "T", CronExpression: "* * * * *",
		Workflows: []string{"wf-clr-both"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	emptyWfs := []string{}
	emptyChains := []string{}
	_, err := svc.Update("task-clr-both", &types.ScheduledTaskUpdateRequest{
		Workflows:        &emptyWfs,
		WorkflowChainIDs: &emptyChains,
	})
	if err == nil {
		t.Fatal("Update with both empty: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "workflows_required") {
		t.Errorf("error = %q, want to contain 'workflows_required'", err.Error())
	}
}

// TestScheduledTaskService_ListRuns_ChainRunsReturned verifies that ListRuns
// returns ChainRuns data from the DB.
func TestScheduledTaskService_ListRuns_ChainRunsReturned(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-runs-chain", "wf-runs-chain", "project")

	if _, err := svc.Create("proj-runs-chain", &types.ScheduledTaskCreateRequest{
		ID: "task-runs-chain", Name: "R", CronExpression: "* * * * *",
		Workflows: []string{"wf-runs-chain"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Insert a schedule_run with chain_runs JSON directly.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	chainJSON := `[{"chain_id":"c1","chain_run_id":"cr-1"},{"chain_id":"c2","error":"boom"}]`
	if _, err := pool.Exec(
		`INSERT INTO schedule_runs (id, scheduled_task_id, project_id, triggered_at, status, workflows, chain_runs, error)
		 VALUES ('run-chain-data', 'task-runs-chain', 'proj-runs-chain', ?, 'triggered', '[]', ?, '')`,
		now, chainJSON,
	); err != nil {
		t.Fatalf("insert schedule_run: %v", err)
	}

	runs, err := svc.ListRuns("task-runs-chain", 50, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListRuns count = %d, want 1", len(runs))
	}
	if len(runs[0].ChainRuns) != 2 {
		t.Fatalf("runs[0].ChainRuns len = %d, want 2", len(runs[0].ChainRuns))
	}
	if runs[0].ChainRuns[0].ChainID != "c1" || runs[0].ChainRuns[0].ChainRunID != "cr-1" {
		t.Errorf("ChainRuns[0] = %+v, want {c1 cr-1}", runs[0].ChainRuns[0])
	}
	if runs[0].ChainRuns[1].Error != "boom" {
		t.Errorf("ChainRuns[1].Error = %q, want 'boom'", runs[0].ChainRuns[1].Error)
	}
}

// TestScheduledTaskService_Create_WorkflowChainIDs_Persisted verifies that
// WorkflowChainIDs are persisted to DB and visible after Get.
func TestScheduledTaskService_Create_WorkflowChainIDs_Persisted(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-persist-chain", "wf-persist-chain", "project")

	if _, err := svc.Create("proj-persist-chain", &types.ScheduledTaskCreateRequest{
		ID: "task-persist-chain", Name: "T", CronExpression: "* * * * *",
		Workflows:        []string{"wf-persist-chain"},
		WorkflowChainIDs: []string{"c-1", "c-2", "c-3"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Get("task-persist-chain")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.WorkflowChainIDs) != 3 {
		t.Errorf("WorkflowChainIDs len = %d, want 3", len(got.WorkflowChainIDs))
	}

	// Verify via repo directly too.
	r := repo.NewScheduledTaskRepo(pool, clock.Real())
	raw, err := r.Get("task-persist-chain")
	if err != nil {
		t.Fatalf("repo.Get: %v", err)
	}
	if len(raw.WorkflowChainIDs) != 3 || raw.WorkflowChainIDs[2] != "c-3" {
		t.Errorf("repo WorkflowChainIDs = %v, want [c-1 c-2 c-3]", raw.WorkflowChainIDs)
	}
}

// TestScheduledTaskService_List_IncludesWorkflowChainIDs verifies that List
// includes WorkflowChainIDs on each returned task.
func TestScheduledTaskService_List_IncludesWorkflowChainIDs(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-list-chain", "wf-list-chain", "project")

	if _, err := svc.Create("proj-list-chain", &types.ScheduledTaskCreateRequest{
		ID: "task-list-chain", Name: "T", CronExpression: "* * * * *",
		Workflows:        []string{"wf-list-chain"},
		WorkflowChainIDs: []string{"chain-listed"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tasks, err := svc.List("proj-list-chain")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("List count = %d, want 1", len(tasks))
	}
	if len(tasks[0].WorkflowChainIDs) != 1 || tasks[0].WorkflowChainIDs[0] != "chain-listed" {
		t.Errorf("tasks[0].WorkflowChainIDs = %v, want [chain-listed]", tasks[0].WorkflowChainIDs)
	}
}

// setupScheduledTaskTestEnv shadows the main test env setup but is in this file
// for readability. It delegates to the package-level function in scheduled_task_test.go
// which already handles the template DB copy.
// (Removed - use the existing one directly.)

// Compile-time assertion: model types are correct.
var _ = model.ScheduleRunChain{}
