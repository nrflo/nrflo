package scheduler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/ws"
)

// setupDispatchEnv creates an isolated environment for dispatch tests.
func setupDispatchEnv(t *testing.T) (*Scheduler, *db.Pool, *ws.Hub) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "dispatch_test.db")
	if err := schedCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	orch := orchestrator.New(dbPath, hub, clock.Real(), nil, false, "")
	sched := New(pool, orch, hub, clock.Real(), nil, nil)
	t.Cleanup(func() {
		orch.StopAll()
		sched.Stop()
		hub.Stop()
		pool.Close()
	})
	return sched, pool, hub
}

// seedDispatchProject inserts a project with a valid root_path.
func seedDispatchProject(t *testing.T, pool *db.Pool, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', ?, ?, ?)`,
		projectID, t.TempDir(), now, now,
	)
	if err != nil {
		t.Fatalf("seedDispatchProject(%q): %v", projectID, err)
	}
}

// seedProjectWorkflowWithAgent creates a project-scoped workflow with one agent_definition.
func seedProjectWorkflowWithAgent(t *testing.T, pool *db.Pool, projectID, workflowID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, created_at, updated_at)
		 VALUES (?, ?, '', 'project', '[]', 1, ?, ?)`,
		workflowID, projectID, now, now,
	)
	if err != nil {
		t.Fatalf("seedProjectWorkflow(%q): %v", workflowID, err)
	}
	_, err = pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, created_at, updated_at)
		 VALUES (?, ?, ?, 'sonnet', 5, 'test prompt', 0, ?, ?)`,
		"agent-sched-1", projectID, workflowID, now, now,
	)
	if err != nil {
		t.Fatalf("seedAgentDefinition: %v", err)
	}
}

// makeDispatchTask creates a ScheduledTask model without inserting it to the DB.
func makeDispatchTask(id, projectID string, workflows []string) *model.ScheduledTask {
	return &model.ScheduledTask{
		ID:             id,
		ProjectID:      projectID,
		Name:           id,
		CronExpression: "* * * * *",
		Workflows:      workflows,
		Enabled:        true,
	}
}

// insertDispatchTask inserts a scheduled task for dispatch tests.
func insertDispatchTask(t *testing.T, pool *db.Pool, task *model.ScheduledTask) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	wJSON, _ := json.Marshal(task.Workflows)
	_, err := pool.Exec(
		`INSERT INTO scheduled_tasks (id, project_id, name, description, cron_expression, workflows, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, '', ?, ?, 1, ?, ?)`,
		task.ID, task.ProjectID, task.Name, task.CronExpression, string(wJSON), now, now,
	)
	if err != nil {
		t.Fatalf("insertDispatchTask(%q): %v", task.ID, err)
	}
}

// -- dispatch --

func TestDispatch_SingleWorkflowFails_StatusFailed(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-fail")

	task := makeDispatchTask("task-fail", "proj-fail", []string{"nonexistent-workflow"})
	insertDispatchTask(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if run.Status != "failed" {
		t.Errorf("run.Status = %q, want 'failed'", run.Status)
	}
	if len(run.Workflows) != 1 {
		t.Fatalf("len(run.Workflows) = %d, want 1", len(run.Workflows))
	}
	if run.Workflows[0].Error == "" {
		t.Error("run.Workflows[0].Error is empty, want non-empty on failure")
	}
	if run.Workflows[0].InstanceID != "" {
		t.Errorf("run.Workflows[0].InstanceID = %q, want empty on failure", run.Workflows[0].InstanceID)
	}

	// Verify persisted in DB.
	runRepo := repo.NewScheduleRunRepo(pool, clock.Real())
	persisted, dbErr := runRepo.Get(run.ID)
	if dbErr != nil {
		t.Fatalf("runRepo.Get: %v", dbErr)
	}
	if persisted.Status != "failed" {
		t.Errorf("DB run.Status = %q, want 'failed'", persisted.Status)
	}
}

func TestDispatch_TwoWorkflowsBothFail_StatusFailed(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-both-fail")

	task := makeDispatchTask("task-both-fail", "proj-both-fail", []string{"wf-x", "wf-y"})
	insertDispatchTask(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if run.Status != "failed" {
		t.Errorf("run.Status = %q, want 'failed' (all fail)", run.Status)
	}
	if len(run.Workflows) != 2 {
		t.Errorf("len(run.Workflows) = %d, want 2", len(run.Workflows))
	}
}

func TestDispatch_AllSucceed_StatusTriggered(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-ok")
	seedProjectWorkflowWithAgent(t, pool, "proj-ok", "wf-ok")

	task := makeDispatchTask("task-ok", "proj-ok", []string{"wf-ok"})
	insertDispatchTask(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if run.Status != "triggered" {
		t.Errorf("run.Status = %q, want 'triggered'", run.Status)
	}
	if len(run.Workflows) != 1 {
		t.Fatalf("len(run.Workflows) = %d, want 1", len(run.Workflows))
	}
	if run.Workflows[0].InstanceID == "" {
		t.Error("run.Workflows[0].InstanceID is empty, want non-empty on success")
	}
	if run.Workflows[0].Error != "" {
		t.Errorf("run.Workflows[0].Error = %q, want empty on success", run.Workflows[0].Error)
	}

	// Verify DB row.
	runRepo := repo.NewScheduleRunRepo(pool, clock.Real())
	persisted, dbErr := runRepo.Get(run.ID)
	if dbErr != nil {
		t.Fatalf("runRepo.Get: %v", dbErr)
	}
	if persisted.Status != "triggered" {
		t.Errorf("DB run.Status = %q, want 'triggered'", persisted.Status)
	}
}

func TestDispatch_Mixed_StatusTriggered(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-mix")
	seedProjectWorkflowWithAgent(t, pool, "proj-mix", "wf-valid")

	task := makeDispatchTask("task-mix", "proj-mix", []string{"wf-valid", "wf-missing"})
	insertDispatchTask(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if run.Status != "triggered" {
		t.Errorf("run.Status = %q, want 'triggered' (at least one success)", run.Status)
	}
}

func TestDispatch_BroadcastsScheduleTriggeredEvent(t *testing.T) {
	sched, pool, hub := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-ws")

	client, ch := ws.NewTestClient(hub, "ws-test-client")
	hub.Subscribe(client, "proj-ws", "")

	task := makeDispatchTask("task-ws", "proj-ws", []string{"no-wf"})
	insertDispatchTask(t, pool, task)

	if _, err := sched.dispatch(context.Background(), task); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var evt map[string]interface{}
			if jsonErr := json.Unmarshal(msg, &evt); jsonErr == nil {
				if evt["type"] == ws.EventScheduleTriggered {
					return
				}
			}
		case <-deadline:
			t.Fatal("did not receive schedule.triggered WS event within 2s")
		}
	}
}

func TestDispatch_UpdatesTaskLastTriggeredAt(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-ts")

	task := makeDispatchTask("task-ts", "proj-ts", []string{"no-wf"})
	insertDispatchTask(t, pool, task)

	if _, err := sched.dispatch(context.Background(), task); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	taskRepo := repo.NewScheduledTaskRepo(pool, clock.Real())
	updated, err := taskRepo.Get("task-ts")
	if err != nil {
		t.Fatalf("taskRepo.Get: %v", err)
	}
	if updated.LastTriggeredAt == nil {
		t.Error("LastTriggeredAt = nil after dispatch, want non-nil")
	}
}

func TestDispatch_RunNow_InsertsPersistentRun(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	if err := sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	seedDispatchProject(t, pool, "proj-rn")

	now := time.Now().UTC().Format(time.RFC3339Nano)
	wJSON, _ := json.Marshal([]string{"no-wf"})
	_, err := pool.Exec(
		`INSERT INTO scheduled_tasks (id, project_id, name, description, cron_expression, workflows, enabled, created_at, updated_at)
		 VALUES ('task-rn', 'proj-rn', 'task-rn', '', '* * * * *', ?, 1, ?, ?)`,
		string(wJSON), now, now,
	)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	run, err := sched.RunNow("task-rn")
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if run.ScheduledTaskID != "task-rn" {
		t.Errorf("run.ScheduledTaskID = %q, want 'task-rn'", run.ScheduledTaskID)
	}
	if run.Status != "failed" && run.Status != "triggered" {
		t.Errorf("run.Status = %q, want 'failed' or 'triggered'", run.Status)
	}
	if run.ID == "" {
		t.Error("run.ID is empty")
	}

	runRepo := repo.NewScheduleRunRepo(pool, clock.Real())
	persisted, dbErr := runRepo.Get(run.ID)
	if dbErr != nil {
		t.Fatalf("runRepo.Get: %v", dbErr)
	}
	if persisted.Status == "pending" {
		t.Error("DB run.Status is still 'pending', expected final status")
	}
}

func TestDispatch_StampsScheduledTaskID(t *testing.T) {
	sched, pool, _ := setupDispatchEnv(t)
	seedDispatchProject(t, pool, "proj-stamp")
	seedProjectWorkflowWithAgent(t, pool, "proj-stamp", "wf-stamp")

	task := makeDispatchTask("task-stamp", "proj-stamp", []string{"wf-stamp"})
	insertDispatchTask(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if run.Status != "triggered" {
		t.Fatalf("run.Status = %q, want 'triggered'", run.Status)
	}
	if len(run.Workflows) != 1 {
		t.Fatalf("len(run.Workflows) = %d, want 1", len(run.Workflows))
	}
	instanceID := run.Workflows[0].InstanceID
	if instanceID == "" {
		t.Fatal("run.Workflows[0].InstanceID is empty, want non-empty on success")
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, clock.Real())
	wi, wiErr := wfiRepo.Get(instanceID)
	if wiErr != nil {
		t.Fatalf("wfiRepo.Get(%q): %v", instanceID, wiErr)
	}
	if wi.ScheduledTaskID != task.ID {
		t.Errorf("wi.ScheduledTaskID = %q, want %q", wi.ScheduledTaskID, task.ID)
	}
}
