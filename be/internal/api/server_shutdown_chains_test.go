package api

import (
	"context"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// TestShutdownCleanup_ScheduleRuns verifies that running and triggered schedule runs
// are marked failed with error=server_shutdown, and completed runs are left untouched.
func TestShutdownCleanup_ScheduleRuns(t *testing.T) {
	srv := newShutdownTestServer(t)
	pid := sdProject(t, srv)

	_, err := srv.pool.Exec(
		`INSERT INTO scheduled_tasks (id, project_id, name, cron_expression, workflows, workflow_chain_ids, enabled, created_at, updated_at)
		VALUES ('task-sd-001', ?, 'Test', '0 * * * *', '[]', '[]', 1, datetime('now'), datetime('now'))`,
		pid)
	if err != nil {
		t.Fatalf("seed scheduled_task: %v", err)
	}

	srRepo := repo.NewScheduleRunRepo(srv.pool, srv.clock)

	for _, tc := range []struct{ id, status string }{
		{"sr-running-001", "running"},
		{"sr-triggered-001", "triggered"},
	} {
		if err := srRepo.Insert(&model.ScheduleRun{
			ID:              tc.id,
			ScheduledTaskID: "task-sd-001",
			ProjectID:       pid,
			Status:          tc.status,
			Workflows:       []model.ScheduleRunWorkflow{},
			ChainRuns:       []model.ScheduleRunChain{},
		}); err != nil {
			t.Fatalf("Insert %s: %v", tc.id, err)
		}
	}

	srv.shutdownCleanup(context.Background())

	for _, id := range []string{"sr-running-001", "sr-triggered-001"} {
		run, err := srRepo.Get(id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if run.Status != "failed" {
			t.Errorf("%s: status = %q, want failed", id, run.Status)
		}
		if run.Error != "server_shutdown" {
			t.Errorf("%s: error = %q, want server_shutdown", id, run.Error)
		}
	}

	// Completed schedule run must survive the sweep untouched.
	if err := srRepo.Insert(&model.ScheduleRun{
		ID: "sr-completed-001", ScheduledTaskID: "task-sd-001", ProjectID: pid,
		Status: "completed", Workflows: []model.ScheduleRunWorkflow{}, ChainRuns: []model.ScheduleRunChain{},
	}); err != nil {
		t.Fatalf("Insert completed: %v", err)
	}
	srv.shutdownCleanup(context.Background())
	completed, err := srRepo.Get("sr-completed-001")
	if err != nil {
		t.Fatalf("Get completed: %v", err)
	}
	if completed.Status != "completed" {
		t.Errorf("completed run status changed to %q (should be no-op)", completed.Status)
	}
}

// TestShutdownCleanup_WorkflowChainRuns verifies that running workflow_chain_runs and
// their steps are failed/canceled and EventChainRunFailed is broadcast.
func TestShutdownCleanup_WorkflowChainRuns(t *testing.T) {
	srv := newShutdownTestServer(t)
	pid := sdProject(t, srv)

	ch := sdWSClient(t, srv, pid, "")

	clk := srv.clock
	chainID := "wchain-sd-001"
	cr := repo.NewWorkflowChainRepo(srv.pool, clk)
	if err := cr.CreateChain(&model.WorkflowChain{
		ID: chainID, ProjectID: pid, Name: "Test",
	}); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	sr := repo.NewWorkflowChainStepRepo(srv.pool, clk)
	steps := []*model.WorkflowChainStep{
		{ID: "step-s0", ProjectID: pid, ChainID: chainID, Position: 0, WorkflowName: "feature", ScopeType: "project"},
		{ID: "step-s1", ProjectID: pid, ChainID: chainID, Position: 1, WorkflowName: "feature", ScopeType: "project"},
	}
	for _, step := range steps {
		if err := sr.UpsertStep(step); err != nil {
			t.Fatalf("UpsertStep: %v", err)
		}
	}

	runID := "wcrun-sd-001"
	rr := repo.NewWorkflowChainRunRepo(srv.pool, clk)
	if err := rr.CreateRun(&model.WorkflowChainRun{
		ID: runID, ProjectID: pid, ChainID: chainID, Status: "running",
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := rr.MaterializeRunSteps(runID, steps); err != nil {
		t.Fatalf("MaterializeRunSteps: %v", err)
	}

	srv.shutdownCleanup(context.Background())

	run, err := rr.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != "failed" {
		t.Errorf("run status = %q, want failed", run.Status)
	}

	runSteps, err := rr.ListRunSteps(runID)
	if err != nil {
		t.Fatalf("ListRunSteps: %v", err)
	}
	for _, s := range runSteps {
		if s.Status != "canceled" {
			t.Errorf("step %s status = %q, want canceled", s.ID, s.Status)
		}
	}

	if !waitForEvent(t, ch, 2*time.Second, ws.EventChainRunFailed) {
		t.Error("EventChainRunFailed not received within 2s")
	}
}

// TestShutdownCleanup_ChainExecutions verifies that running chain_executions are marked
// failed, items are canceled, locks are released, and EventChainUpdated is broadcast.
func TestShutdownCleanup_ChainExecutions(t *testing.T) {
	srv := newShutdownTestServer(t)
	pid := sdProject(t, srv)

	ch := sdWSClient(t, srv, pid, "")

	clk := srv.clock
	chainID := "chain-exec-sd-001"

	chainRepo := repo.NewChainRepo(srv.pool, clk)
	if err := chainRepo.Create(&model.ChainExecution{
		ID:           chainID,
		ProjectID:    pid,
		WorkflowName: "feature",
		Status:       model.ChainStatusRunning,
	}); err != nil {
		t.Fatalf("Create chain execution: %v", err)
	}

	itemRepo := repo.NewChainItemRepo(srv.pool, clk)
	items := []*model.ChainExecutionItem{
		{ID: "citem-001", ChainID: chainID, TicketID: "tkt-ci-a", Position: 0, Status: model.ChainItemRunning},
		{ID: "citem-002", ChainID: chainID, TicketID: "tkt-ci-b", Position: 1, Status: model.ChainItemPending},
	}
	if err := itemRepo.BatchInsert(items); err != nil {
		t.Fatalf("BatchInsert items: %v", err)
	}

	lockRepo := repo.NewChainLockRepo(srv.pool)
	if err := lockRepo.InsertLocks(pid, chainID, []string{"tkt-ci-a"}); err != nil {
		t.Fatalf("InsertLocks: %v", err)
	}

	srv.shutdownCleanup(context.Background())

	chain, err := chainRepo.Get(chainID)
	if err != nil {
		t.Fatalf("Get chain: %v", err)
	}
	if chain.Status != model.ChainStatusFailed {
		t.Errorf("chain status = %q, want failed", chain.Status)
	}

	// Verify items are canceled by querying the table directly (avoids JOIN dependency).
	for _, itemID := range []string{"citem-001", "citem-002"} {
		var status string
		if err := srv.pool.QueryRow(
			`SELECT status FROM chain_execution_items WHERE id = ?`, itemID,
		).Scan(&status); err != nil {
			t.Fatalf("scan item %s status: %v", itemID, err)
		}
		if status != string(model.ChainItemCanceled) {
			t.Errorf("item %s status = %q, want canceled", itemID, status)
		}
	}

	// Lock must be released.
	conflicts, err := lockRepo.CheckConflicts(pid, []string{"tkt-ci-a"}, "")
	if err != nil {
		t.Fatalf("CheckConflicts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts after lock release, got %d", len(conflicts))
	}

	if !waitForEvent(t, ch, 2*time.Second, ws.EventChainUpdated) {
		t.Error("EventChainUpdated not received within 2s")
	}
}
