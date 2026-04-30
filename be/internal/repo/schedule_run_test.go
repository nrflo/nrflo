package repo

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

type scheduleRunTestEnv struct {
	taskRepo *ScheduledTaskRepo
	runRepo  *ScheduleRunRepo
	projectID string
	taskID    string
}

func setupScheduleRunDB(t *testing.T) *scheduleRunTestEnv {
	t.Helper()
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	taskRepo := NewScheduledTaskRepo(database, clk)
	task := &model.ScheduledTask{
		ID:             "st-1",
		ProjectID:      "proj-1",
		Name:           "hourly",
		CronExpression: "0 * * * *",
		Workflows:      []string{"feature"},
		Enabled:        true,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("create scheduled task: %v", err)
	}

	return &scheduleRunTestEnv{
		taskRepo:  taskRepo,
		runRepo:   NewScheduleRunRepo(database, clk),
		projectID: "proj-1",
		taskID:    "st-1",
	}
}

func makeRun(id, taskID, projectID, status string) *model.ScheduleRun {
	return &model.ScheduleRun{
		ID:              id,
		ScheduledTaskID: taskID,
		ProjectID:       projectID,
		Status:          status,
		Workflows:       []model.ScheduleRunWorkflow{},
	}
}

func TestScheduleRunRepo_Insert_AutoFillsTriggeredAt(t *testing.T) {
	fixedTime := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'T', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	taskRepo := NewScheduledTaskRepo(database, clk)
	if err := taskRepo.Create(&model.ScheduledTask{
		ID: "st-auto", ProjectID: "p1", Name: "t",
		CronExpression: "* * * * *", Workflows: []string{}, Enabled: true,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	runRepo := NewScheduleRunRepo(database, clk)
	run := makeRun("run-auto", "st-auto", "p1", "running")
	// TriggeredAt is zero — repo should use clock
	if err := runRepo.Insert(run); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if run.TriggeredAt.IsZero() {
		t.Fatalf("TriggeredAt not set on run after Insert")
	}
	if !run.TriggeredAt.Equal(fixedTime) {
		t.Errorf("TriggeredAt = %v, want %v", run.TriggeredAt, fixedTime)
	}

	got, err := runRepo.Get("run-auto")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.TriggeredAt.Equal(fixedTime) {
		t.Errorf("persisted TriggeredAt = %v, want %v", got.TriggeredAt, fixedTime)
	}
}

func TestScheduleRunRepo_Insert_KeepsExplicitTriggeredAt(t *testing.T) {
	env := setupScheduleRunDB(t)

	explicitTime := time.Date(2026, 3, 10, 8, 30, 0, 0, time.UTC)
	run := makeRun("run-explicit", env.taskID, env.projectID, "running")
	run.TriggeredAt = explicitTime

	if err := env.runRepo.Insert(run); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := env.runRepo.Get("run-explicit")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.TriggeredAt.Equal(explicitTime) {
		t.Errorf("TriggeredAt = %v, want %v", got.TriggeredAt, explicitTime)
	}
}

func TestScheduleRunRepo_Insert_WorkflowsRoundTrip(t *testing.T) {
	env := setupScheduleRunDB(t)

	run := &model.ScheduleRun{
		ID:              "run-wf",
		ScheduledTaskID: env.taskID,
		ProjectID:       env.projectID,
		Status:          "completed",
		Workflows: []model.ScheduleRunWorkflow{
			{Workflow: "feature", InstanceID: "inst-abc"},
			{Workflow: "bugfix", InstanceID: "inst-def", Error: "some error"},
		},
	}
	if err := env.runRepo.Insert(run); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := env.runRepo.Get("run-wf")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Workflows) != 2 {
		t.Fatalf("Workflows len = %d, want 2", len(got.Workflows))
	}
	if got.Workflows[0].Workflow != "feature" || got.Workflows[0].InstanceID != "inst-abc" {
		t.Errorf("Workflows[0] = %+v, want {feature inst-abc}", got.Workflows[0])
	}
	if got.Workflows[1].Error != "some error" {
		t.Errorf("Workflows[1].Error = %q, want some error", got.Workflows[1].Error)
	}
}

func TestScheduleRunRepo_Get_NotFound(t *testing.T) {
	env := setupScheduleRunDB(t)

	_, err := env.runRepo.Get("no-such-run")
	if err == nil {
		t.Fatalf("Get missing run: expected error, got nil")
	}
}

func TestScheduleRunRepo_UpdateStatus(t *testing.T) {
	env := setupScheduleRunDB(t)

	run := makeRun("run-upd", env.taskID, env.projectID, "running")
	if err := env.runRepo.Insert(run); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	wfs := []model.ScheduleRunWorkflow{
		{Workflow: "feature", InstanceID: "inst-xyz"},
	}
	wfsJSON, _ := json.Marshal(wfs)

	if err := env.runRepo.UpdateStatus("run-upd", "completed", string(wfsJSON), ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err := env.runRepo.Get("run-upd")
	if err != nil {
		t.Fatalf("Get after UpdateStatus: %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if len(got.Workflows) != 1 || got.Workflows[0].InstanceID != "inst-xyz" {
		t.Errorf("Workflows = %+v, want [{feature inst-xyz}]", got.Workflows)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty", got.Error)
	}
}

func TestScheduleRunRepo_UpdateStatus_WithError(t *testing.T) {
	env := setupScheduleRunDB(t)

	run := makeRun("run-fail", env.taskID, env.projectID, "running")
	if err := env.runRepo.Insert(run); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := env.runRepo.UpdateStatus("run-fail", "failed", "[]", "connection timeout"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err := env.runRepo.Get("run-fail")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	if got.Error != "connection timeout" {
		t.Errorf("Error = %q, want connection timeout", got.Error)
	}
}

func TestScheduleRunRepo_UpdateStatus_NotFound(t *testing.T) {
	env := setupScheduleRunDB(t)

	err := env.runRepo.UpdateStatus("no-such", "completed", "[]", "")
	if err == nil {
		t.Fatalf("UpdateStatus missing run: expected error, got nil")
	}
}

func TestScheduleRunRepo_ListByTask_OrderedDescWithLimitOffset(t *testing.T) {
	fixedBase := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedBase)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'T', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	taskRepo := NewScheduledTaskRepo(database, clk)
	if err := taskRepo.Create(&model.ScheduledTask{
		ID: "st-list", ProjectID: "p1", Name: "t",
		CronExpression: "* * * * *", Workflows: []string{}, Enabled: true,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	runRepo := NewScheduleRunRepo(database, clk)

	// Insert 5 runs with strictly increasing triggered_at
	for i := 0; i < 5; i++ {
		clk.Set(fixedBase.Add(time.Duration(i) * time.Minute))
		run := makeRun(fmt.Sprintf("run-%d", i+1), "st-list", "p1", "completed")
		run.TriggeredAt = clk.Now()
		if err := runRepo.Insert(run); err != nil {
			t.Fatalf("Insert run-%d: %v", i+1, err)
		}
	}

	// All 5, ordered DESC: run-5, run-4, run-3, run-2, run-1
	all, err := runRepo.ListByTask("st-list", 10, 0)
	if err != nil {
		t.Fatalf("ListByTask all: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("ListByTask all count = %d, want 5", len(all))
	}
	if all[0].ID != "run-5" {
		t.Errorf("all[0].ID = %q, want run-5 (most recent)", all[0].ID)
	}
	if all[4].ID != "run-1" {
		t.Errorf("all[4].ID = %q, want run-1 (oldest)", all[4].ID)
	}

	// Limit 2
	first2, err := runRepo.ListByTask("st-list", 2, 0)
	if err != nil {
		t.Fatalf("ListByTask limit2: %v", err)
	}
	if len(first2) != 2 {
		t.Fatalf("ListByTask limit2 count = %d, want 2", len(first2))
	}
	if first2[0].ID != "run-5" || first2[1].ID != "run-4" {
		t.Errorf("first2 = [%s %s], want [run-5 run-4]", first2[0].ID, first2[1].ID)
	}

	// Offset 2, limit 2 → run-3, run-2
	page2, err := runRepo.ListByTask("st-list", 2, 2)
	if err != nil {
		t.Fatalf("ListByTask page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("ListByTask page2 count = %d, want 2", len(page2))
	}
	if page2[0].ID != "run-3" || page2[1].ID != "run-2" {
		t.Errorf("page2 = [%s %s], want [run-3 run-2]", page2[0].ID, page2[1].ID)
	}
}

func TestScheduleRunRepo_ListByTask_EmptySliceWhenNoRows(t *testing.T) {
	env := setupScheduleRunDB(t)

	runs, err := env.runRepo.ListByTask(env.taskID, 10, 0)
	if err != nil {
		t.Fatalf("ListByTask empty: %v", err)
	}
	if runs == nil {
		t.Fatalf("ListByTask returned nil, want empty slice")
	}
	if len(runs) != 0 {
		t.Errorf("ListByTask count = %d, want 0", len(runs))
	}
}

func TestScheduleRunRepo_ListByTask_CascadeDelete(t *testing.T) {
	env := setupScheduleRunDB(t)

	// Insert a couple of runs
	for i := 0; i < 3; i++ {
		run := makeRun(fmt.Sprintf("run-cascade-%d", i), env.taskID, env.projectID, "completed")
		if err := env.runRepo.Insert(run); err != nil {
			t.Fatalf("Insert run %d: %v", i, err)
		}
	}

	// Verify runs exist
	before, err := env.runRepo.ListByTask(env.taskID, 10, 0)
	if err != nil {
		t.Fatalf("ListByTask before delete: %v", err)
	}
	if len(before) != 3 {
		t.Fatalf("before delete count = %d, want 3", len(before))
	}

	// Delete the parent scheduled_task row directly via raw SQL
	// (bypassing the repo to simulate what would happen at the DB level)
	_, err = env.taskRepo.db.Exec(`DELETE FROM scheduled_tasks WHERE id = ?`, env.taskID)
	if err != nil {
		t.Fatalf("raw DELETE scheduled_tasks: %v", err)
	}

	// ON DELETE CASCADE should have removed the schedule_runs rows
	after, err := env.runRepo.ListByTask(env.taskID, 10, 0)
	if err != nil {
		t.Fatalf("ListByTask after cascade delete: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("after cascade delete count = %d, want 0", len(after))
	}
}
