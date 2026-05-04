package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/types"
)

// mockReloader counts Reload calls.
type mockReloader struct {
	count int
}

func (m *mockReloader) Reload() error {
	m.count++
	return nil
}

// setupScheduledTaskTestEnv creates an isolated DB for scheduled task service tests.
func setupScheduledTaskTestEnv(t *testing.T) (*ScheduledTaskService, *db.Pool, *mockReloader, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sched_svc_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	reloader := &mockReloader{}
	svc := NewScheduledTaskService(pool, clock.Real(), reloader)
	return svc, pool, reloader, func() { pool.Close() }
}

// seedProjectAndWorkflow creates a project and a project-scoped workflow.
func seedProjectAndWorkflow(t *testing.T, pool *db.Pool, projectID, workflowID, scopeType string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now,
	)
	if err != nil {
		t.Fatalf("seedProject(%q): %v", projectID, err)
	}
	_, err = pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, created_at, updated_at)
		 VALUES (?, ?, '', ?, '[]', 1, ?, ?)`,
		workflowID, projectID, scopeType, now, now,
	)
	if err != nil {
		t.Fatalf("seedWorkflow(%q, scope=%q): %v", workflowID, scopeType, err)
	}
}

// -- List --

func TestScheduledTaskService_List_Empty(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-list", "wf-list", "project")

	tasks, err := svc.List("proj-list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("len(tasks) = %d, want 0", len(tasks))
	}
}

func TestScheduledTaskService_List_ReturnsTasks(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-list2", "wf-proj", "project")

	if _, err := svc.Create("proj-list2", &types.ScheduledTaskCreateRequest{
		ID: "task-1", Name: "T1", CronExpression: "* * * * *", Workflows: []string{"wf-proj"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tasks, err := svc.List("proj-list2")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("len(tasks) = %d, want 1", len(tasks))
	}
	if tasks[0].ID != "task-1" {
		t.Errorf("tasks[0].ID = %q, want 'task-1'", tasks[0].ID)
	}
}

// -- Get --

func TestScheduledTaskService_Get_Found(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-get", "wf-get", "project")

	if _, err := svc.Create("proj-get", &types.ScheduledTaskCreateRequest{
		ID: "task-get", Name: "Get Task", CronExpression: "0 * * * *", Workflows: []string{"wf-get"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	task, err := svc.Get("task-get")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if task.Name != "Get Task" {
		t.Errorf("Name = %q, want 'Get Task'", task.Name)
	}
}

func TestScheduledTaskService_Get_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()

	_, err := svc.Get("no-such-task")
	if err == nil {
		t.Fatal("Get: expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

// -- Create validation --

func TestScheduledTaskService_Create_Valid(t *testing.T) {
	t.Parallel()
	svc, pool, reloader, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-create", "wf-create", "project")

	task, err := svc.Create("proj-create", &types.ScheduledTaskCreateRequest{
		ID: "task-new", Name: "New Task", CronExpression: "*/5 * * * *", Workflows: []string{"wf-create"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.ID != "task-new" {
		t.Errorf("ID = %q, want 'task-new'", task.ID)
	}
	if task.Enabled != true {
		t.Error("Enabled = false, want true (default)")
	}
	if reloader.count != 1 {
		t.Errorf("reloader.count = %d, want 1", reloader.count)
	}
}

func TestScheduledTaskService_Create_MissingName(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-cn", "wf-cn", "project")

	_, err := svc.Create("proj-cn", &types.ScheduledTaskCreateRequest{
		CronExpression: "* * * * *", Workflows: []string{"wf-cn"},
	})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestScheduledTaskService_Create_InvalidCron(t *testing.T) {
	t.Parallel()
	cases := []string{"bad-cron", "*/5 *", "0 0 * * * * *"}
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-cron", "wf-cron", "project")

	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, err := svc.Create("proj-cron", &types.ScheduledTaskCreateRequest{
				Name: "T", CronExpression: c, Workflows: []string{"wf-cron"},
			})
			if err == nil {
				t.Fatalf("Create with cron %q: expected error, got nil", c)
			}
			if !strings.Contains(err.Error(), "invalid cron") {
				t.Errorf("error = %q, want to contain 'invalid cron'", err.Error())
			}
		})
	}
}

func TestScheduledTaskService_Create_EmptyWorkflows(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-ewf", "wf-ewf", "project")

	_, err := svc.Create("proj-ewf", &types.ScheduledTaskCreateRequest{
		Name: "T", CronExpression: "* * * * *", Workflows: []string{},
	})
	if err == nil {
		t.Fatal("expected error for empty workflows, got nil")
	}
	if !strings.Contains(err.Error(), "workflows_required") {
		t.Errorf("error = %q, want to contain 'workflows_required'", err.Error())
	}
}

func TestScheduledTaskService_Create_NonExistentWorkflow(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-newf", "wf-newf", "project")

	_, err := svc.Create("proj-newf", &types.ScheduledTaskCreateRequest{
		Name: "T", CronExpression: "* * * * *", Workflows: []string{"no-such-wf"},
	})
	if err == nil {
		t.Fatal("expected error for non-existent workflow, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_workflow") {
		t.Errorf("error = %q, want to contain 'invalid_workflow'", err.Error())
	}
}

func TestScheduledTaskService_Create_TicketScopedWorkflow(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-tscope", "wf-ticket", "ticket")

	_, err := svc.Create("proj-tscope", &types.ScheduledTaskCreateRequest{
		Name: "T", CronExpression: "* * * * *", Workflows: []string{"wf-ticket"},
	})
	if err == nil {
		t.Fatal("expected error for ticket-scoped workflow, got nil")
	}
	if !strings.Contains(err.Error(), "not_project_scope") {
		t.Errorf("error = %q, want to contain 'not_project_scope'", err.Error())
	}
}

func TestScheduledTaskService_Create_DuplicateID(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-dup", "wf-dup", "project")

	if _, err := svc.Create("proj-dup", &types.ScheduledTaskCreateRequest{
		ID: "dup-id", Name: "T", CronExpression: "* * * * *", Workflows: []string{"wf-dup"},
	}); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err := svc.Create("proj-dup", &types.ScheduledTaskCreateRequest{
		ID: "dup-id", Name: "T2", CronExpression: "* * * * *", Workflows: []string{"wf-dup"},
	})
	if err == nil {
		t.Fatal("duplicate Create: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want to contain 'already exists'", err.Error())
	}
}

// -- Update --

func TestScheduledTaskService_Update_NameOnly(t *testing.T) {
	t.Parallel()
	svc, pool, reloader, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-upd", "wf-upd", "project")

	if _, err := svc.Create("proj-upd", &types.ScheduledTaskCreateRequest{
		ID: "task-upd", Name: "Original", CronExpression: "* * * * *", Workflows: []string{"wf-upd"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	reloader.count = 0

	newName := "Updated"
	task, err := svc.Update("task-upd", &types.ScheduledTaskUpdateRequest{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if task.Name != "Updated" {
		t.Errorf("Name = %q, want 'Updated'", task.Name)
	}
	if reloader.count != 1 {
		t.Errorf("reloader.count = %d, want 1", reloader.count)
	}
}

func TestScheduledTaskService_Update_InvalidCron(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-ucron", "wf-ucron", "project")

	if _, err := svc.Create("proj-ucron", &types.ScheduledTaskCreateRequest{
		ID: "task-ucron", Name: "T", CronExpression: "* * * * *", Workflows: []string{"wf-ucron"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	badCron := "not-valid"
	_, err := svc.Update("task-ucron", &types.ScheduledTaskUpdateRequest{CronExpression: &badCron})
	if err == nil {
		t.Fatal("Update with invalid cron: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid cron") {
		t.Errorf("error = %q, want to contain 'invalid cron'", err.Error())
	}
}

func TestScheduledTaskService_Update_EnabledToggle(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-toggle", "wf-toggle", "project")

	if _, err := svc.Create("proj-toggle", &types.ScheduledTaskCreateRequest{
		ID: "task-toggle", Name: "T", CronExpression: "* * * * *", Workflows: []string{"wf-toggle"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	disabled := false
	task, err := svc.Update("task-toggle", &types.ScheduledTaskUpdateRequest{Enabled: &disabled})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if task.Enabled {
		t.Error("Enabled = true, want false after disabling")
	}
}

func TestScheduledTaskService_Update_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()

	newName := "X"
	_, err := svc.Update("no-such", &types.ScheduledTaskUpdateRequest{Name: &newName})
	if err == nil {
		t.Fatal("Update not-found: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

// -- Delete --

func TestScheduledTaskService_Delete_RemovesTask(t *testing.T) {
	t.Parallel()
	svc, pool, reloader, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-del", "wf-del", "project")

	if _, err := svc.Create("proj-del", &types.ScheduledTaskCreateRequest{
		ID: "task-del", Name: "D", CronExpression: "* * * * *", Workflows: []string{"wf-del"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	reloader.count = 0

	if err := svc.Delete("task-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if reloader.count != 1 {
		t.Errorf("reloader.count = %d, want 1", reloader.count)
	}

	_, err := svc.Get("task-del")
	if err == nil {
		t.Error("Get after Delete: expected not-found error, got nil")
	}
}

func TestScheduledTaskService_Delete_CascadesScheduleRuns(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-casc", "wf-casc", "project")

	if _, err := svc.Create("proj-casc", &types.ScheduledTaskCreateRequest{
		ID: "task-casc", Name: "C", CronExpression: "* * * * *", Workflows: []string{"wf-casc"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Insert a schedule_run row manually.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(
		`INSERT INTO schedule_runs (id, scheduled_task_id, project_id, triggered_at, status, workflows, error)
		 VALUES ('run-1', 'task-casc', 'proj-casc', ?, 'triggered', '[]', '')`,
		now,
	)
	if err != nil {
		t.Fatalf("insert schedule_run: %v", err)
	}

	if err := svc.Delete("task-casc"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify schedule_runs were cascaded.
	runRepo := repo.NewScheduleRunRepo(pool, clock.Real())
	runs, listErr := runRepo.ListByTask("task-casc", 50, 0)
	if listErr != nil {
		t.Fatalf("ListByTask: %v", listErr)
	}
	if len(runs) != 0 {
		t.Errorf("len(runs) = %d, want 0 after cascade delete", len(runs))
	}
}

func TestScheduledTaskService_Delete_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()

	err := svc.Delete("no-such")
	if err == nil {
		t.Fatal("Delete not-found: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

// -- ListRuns --

func TestScheduledTaskService_ListRuns_Empty(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-runs", "wf-runs", "project")

	if _, err := svc.Create("proj-runs", &types.ScheduledTaskCreateRequest{
		ID: "task-runs", Name: "R", CronExpression: "* * * * *", Workflows: []string{"wf-runs"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	runs, err := svc.ListRuns("task-runs", 50, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if runs == nil {
		t.Fatal("ListRuns returned nil, want empty slice")
	}
	if len(runs) != 0 {
		t.Errorf("len(runs) = %d, want 0", len(runs))
	}
}
