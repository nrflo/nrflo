package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// makeDispatchTaskWithChains creates a ScheduledTask with both workflows and chain IDs.
func makeDispatchTaskWithChains(id, projectID string, workflows, chainIDs []string) *model.ScheduledTask {
	return &model.ScheduledTask{
		ID:               id,
		ProjectID:        projectID,
		Name:             id,
		CronExpression:   "* * * * *",
		Workflows:        workflows,
		WorkflowChainIDs: chainIDs,
		Enabled:          true,
	}
}

// insertDispatchTaskWithChains inserts a scheduled task with workflow_chain_ids.
func insertDispatchTaskWithChains(t *testing.T, pool *db.Pool, task *model.ScheduledTask) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	wJSON, _ := json.Marshal(task.Workflows)
	cJSON, _ := json.Marshal(task.WorkflowChainIDs)
	_, err := pool.Exec(
		`INSERT INTO scheduled_tasks
		 (id, project_id, name, description, cron_expression, workflows, workflow_chain_ids, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, '', ?, ?, ?, 1, ?, ?)`,
		task.ID, task.ProjectID, task.Name, task.CronExpression,
		string(wJSON), string(cJSON), now, now,
	)
	if err != nil {
		t.Fatalf("insertDispatchTaskWithChains(%q): %v", task.ID, err)
	}
}

// TestDispatch_ChainOnly_NilRunner_StatusFailed verifies that when the chain runner
// is nil, all chain dispatches fail and the run status is "failed" (all items failed).
func TestDispatch_ChainOnly_NilRunner_StatusFailed(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-chain-nil")

	task := makeDispatchTaskWithChains("task-chain-nil", "proj-chain-nil", []string{}, []string{"chain-a", "chain-b"})
	insertDispatchTaskWithChains(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if run.Status != "failed" {
		t.Errorf("run.Status = %q, want 'failed' (all chains fail with nil runner)", run.Status)
	}
	if len(run.ChainRuns) != 2 {
		t.Fatalf("len(run.ChainRuns) = %d, want 2", len(run.ChainRuns))
	}
	for i, cr := range run.ChainRuns {
		if cr.Error == "" {
			t.Errorf("ChainRuns[%d].Error is empty, want non-empty (nil runner)", i)
		}
		if cr.ChainRunID != "" {
			t.Errorf("ChainRuns[%d].ChainRunID = %q, want empty (nil runner)", i, cr.ChainRunID)
		}
	}
}

// TestDispatch_ChainOnly_NilRunner_ChainIDsPresent verifies that the ChainID field
// on each failed chain run is correctly populated even when the runner is nil.
func TestDispatch_ChainOnly_NilRunner_ChainIDsPresent(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-chain-ids")

	task := makeDispatchTaskWithChains("task-chain-ids", "proj-chain-ids", []string{}, []string{"chain-x"})
	insertDispatchTaskWithChains(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(run.ChainRuns) != 1 {
		t.Fatalf("len(run.ChainRuns) = %d, want 1", len(run.ChainRuns))
	}
	if run.ChainRuns[0].ChainID != "chain-x" {
		t.Errorf("ChainRuns[0].ChainID = %q, want 'chain-x'", run.ChainRuns[0].ChainID)
	}
}

// TestDispatch_ChainRuns_PersistedInDB verifies that chain_runs are written to
// the schedule_runs table when dispatch completes.
func TestDispatch_ChainRuns_PersistedInDB(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-chain-persist")

	task := makeDispatchTaskWithChains("task-chain-persist", "proj-chain-persist", []string{}, []string{"chain-p"})
	insertDispatchTaskWithChains(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	runRepo := repo.NewScheduleRunRepo(pool, clock.Real())
	persisted, err := runRepo.Get(run.ID)
	if err != nil {
		t.Fatalf("runRepo.Get: %v", err)
	}

	if len(persisted.ChainRuns) != 1 {
		t.Fatalf("persisted.ChainRuns len = %d, want 1", len(persisted.ChainRuns))
	}
	if persisted.ChainRuns[0].ChainID != "chain-p" {
		t.Errorf("persisted ChainRuns[0].ChainID = %q, want 'chain-p'", persisted.ChainRuns[0].ChainID)
	}
	if persisted.ChainRuns[0].Error == "" {
		t.Error("persisted ChainRuns[0].Error is empty, want non-empty (nil runner)")
	}
}

// TestDispatch_WSEvent_HasChainRunsKey verifies that the schedule.triggered WS
// event payload includes a "chain_runs" key.
func TestDispatch_WSEvent_HasChainRunsKey(t *testing.T) {
	sched, pool, hub := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-ws-chain")

	client, ch := ws.NewTestClient(hub, "ws-chain-client")
	hub.Subscribe(client, "proj-ws-chain", "")

	task := makeDispatchTaskWithChains("task-ws-chain", "proj-ws-chain", []string{}, []string{"chain-ws"})
	insertDispatchTaskWithChains(t, pool, task)

	if _, err := sched.dispatch(context.Background(), task); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var evt map[string]interface{}
			if jsonErr := json.Unmarshal(msg, &evt); jsonErr != nil {
				continue
			}
			if evt["type"] != ws.EventScheduleTriggered {
				continue
			}
			data, ok := evt["data"].(map[string]interface{})
			if !ok {
				t.Fatal("WS event data is not a map")
			}
			if _, has := data["chain_runs"]; !has {
				t.Error("WS event data missing 'chain_runs' key")
			}
			return
		case <-deadline:
			t.Fatal("did not receive schedule.triggered WS event within 2s")
		}
	}
}

// TestDispatch_WorkflowSucceeds_ChainFails_StatusTriggered verifies that when
// a workflow succeeds but a chain fails, the overall status is "triggered"
// (not all items failed → partial success).
func TestDispatch_WorkflowSucceeds_ChainFails_StatusTriggered(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-mix-chain")
	seedProjectWorkflowWithAgent(t, pool, "proj-mix-chain", "wf-mix-chain")

	task := makeDispatchTaskWithChains("task-mix-chain", "proj-mix-chain",
		[]string{"wf-mix-chain"}, []string{"chain-fail"})
	insertDispatchTaskWithChains(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	// Workflow succeeds, chain fails (nil runner) → not all failed → triggered.
	if run.Status != "triggered" {
		t.Errorf("run.Status = %q, want 'triggered' (workflow succeeded, chain failed)", run.Status)
	}
	if len(run.Workflows) != 1 {
		t.Fatalf("run.Workflows len = %d, want 1", len(run.Workflows))
	}
	if run.Workflows[0].Error != "" {
		t.Errorf("run.Workflows[0].Error = %q, want empty (workflow succeeded)", run.Workflows[0].Error)
	}
	if len(run.ChainRuns) != 1 || run.ChainRuns[0].Error == "" {
		t.Errorf("run.ChainRuns = %+v, want one failed chain", run.ChainRuns)
	}
}

// TestDispatch_WorkflowFails_ChainFails_StatusFailed verifies that when both
// a workflow and chain fail, the overall status is "failed".
func TestDispatch_WorkflowFails_ChainFails_StatusFailed(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-all-fail")

	task := makeDispatchTaskWithChains("task-all-fail", "proj-all-fail",
		[]string{"no-such-wf"}, []string{"no-such-chain"})
	insertDispatchTaskWithChains(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if run.Status != "failed" {
		t.Errorf("run.Status = %q, want 'failed' (all items fail)", run.Status)
	}
	if len(run.Workflows) != 1 || run.Workflows[0].Error == "" {
		t.Errorf("run.Workflows = %+v, want one failed workflow", run.Workflows)
	}
	if len(run.ChainRuns) != 1 || run.ChainRuns[0].Error == "" {
		t.Errorf("run.ChainRuns = %+v, want one failed chain", run.ChainRuns)
	}
}

// TestDispatch_EmptyTask_NoWorkflowsNoChains_StatusTriggered verifies that a task
// with no workflows and no chain IDs dispatches without error and returns triggered.
func TestDispatch_EmptyTask_NoWorkflowsNoChains_StatusTriggered(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-empty-task")

	task := makeDispatchTaskWithChains("task-empty", "proj-empty-task", []string{}, []string{})
	insertDispatchTaskWithChains(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	// No items to fail → anyFailed stays false → status "triggered".
	if run.Status != "triggered" {
		t.Errorf("run.Status = %q, want 'triggered' (no items)", run.Status)
	}
	if len(run.Workflows) != 0 {
		t.Errorf("run.Workflows len = %d, want 0", len(run.Workflows))
	}
	if len(run.ChainRuns) != 0 {
		t.Errorf("run.ChainRuns len = %d, want 0", len(run.ChainRuns))
	}
}

// TestDispatch_ChainFails_WorkflowAlsoFails_BothRecordedInDB verifies that both
// workflow and chain results are persisted correctly when both fail.
func TestDispatch_ChainFails_WorkflowAlsoFails_BothRecordedInDB(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-both-fail-db")

	task := makeDispatchTaskWithChains("task-both-fail-db", "proj-both-fail-db",
		[]string{"missing-wf"}, []string{"missing-chain"})
	insertDispatchTaskWithChains(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	runRepo := repo.NewScheduleRunRepo(pool, clock.Real())
	persisted, err := runRepo.Get(run.ID)
	if err != nil {
		t.Fatalf("runRepo.Get: %v", err)
	}

	if persisted.Status != "failed" {
		t.Errorf("DB run.Status = %q, want 'failed'", persisted.Status)
	}
	if len(persisted.Workflows) != 1 || persisted.Workflows[0].Error == "" {
		t.Errorf("DB Workflows = %+v, want one with error", persisted.Workflows)
	}
	if len(persisted.ChainRuns) != 1 || persisted.ChainRuns[0].Error == "" {
		t.Errorf("DB ChainRuns = %+v, want one with error", persisted.ChainRuns)
	}
}
