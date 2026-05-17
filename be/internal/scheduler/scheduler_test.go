package scheduler

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/ws"
)

// schedTestEnv holds shared state for scheduler lifecycle tests.
type schedTestEnv struct {
	pool   *db.Pool
	sched  *Scheduler
	hub    *ws.Hub
	dbPath string
}

func newSchedTestEnv(t *testing.T) *schedTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sched_test.db")
	if err := schedCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	orch := orchestrator.New(dbPath, hub, clock.Real(), nil, "")
	sched := New(pool, orch, hub, clock.Real(), nil, nil)
	t.Cleanup(func() {
		sched.Stop()
		hub.Stop()
		pool.Close()
	})
	return &schedTestEnv{pool: pool, sched: sched, hub: hub, dbPath: dbPath}
}

// insertEnabledTask inserts a scheduled task row directly for testing.
func insertEnabledTask(t *testing.T, pool *db.Pool, id, projectID, cronExpr string, enabled bool) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := pool.Exec(
		`INSERT INTO scheduled_tasks (id, project_id, name, description, cron_expression, workflows, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, '', ?, '[]', ?, ?, ?)`,
		id, projectID, id, cronExpr, enabledInt, now, now,
	)
	if err != nil {
		t.Fatalf("insertEnabledTask(%q): %v", id, err)
	}
}

// insertSchedProject inserts a minimal project for scheduler tests.
func insertSchedProject(t *testing.T, pool *db.Pool, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', ?, ?, ?)`,
		projectID, t.TempDir(), now, now,
	)
	if err != nil {
		t.Fatalf("insertSchedProject(%q): %v", projectID, err)
	}
}

// -- Start --

func TestStart_EmptyDB_NoCronEntries(t *testing.T) {
	env := newSchedTestEnv(t)
	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := len(env.sched.cron.Entries()); got != 0 {
		t.Errorf("cron.Entries() = %d, want 0", got)
	}
}

func TestStart_EnabledTask_RegistersCronEntry(t *testing.T) {
	env := newSchedTestEnv(t)
	insertSchedProject(t, env.pool, "proj-a")
	insertEnabledTask(t, env.pool, "task-a", "proj-a", "* * * * *", true)

	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := len(env.sched.cron.Entries()); got != 1 {
		t.Errorf("cron.Entries() = %d, want 1", got)
	}
}

func TestStart_EnabledTask_PersistsNextRunAt(t *testing.T) {
	env := newSchedTestEnv(t)
	insertSchedProject(t, env.pool, "proj-b")
	insertEnabledTask(t, env.pool, "task-b", "proj-b", "* * * * *", true)

	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	taskRepo := repo.NewScheduledTaskRepo(env.pool, clock.Real())
	task, err := taskRepo.Get("task-b")
	if err != nil {
		t.Fatalf("taskRepo.Get: %v", err)
	}
	if task.NextRunAt == nil {
		t.Error("NextRunAt = nil after Start, want non-nil")
	}
}

func TestStart_DisabledTask_NotRegistered(t *testing.T) {
	env := newSchedTestEnv(t)
	insertSchedProject(t, env.pool, "proj-c")
	insertEnabledTask(t, env.pool, "task-c", "proj-c", "* * * * *", false)

	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := len(env.sched.cron.Entries()); got != 0 {
		t.Errorf("cron.Entries() = %d, want 0 for disabled task", got)
	}
}

func TestStart_InvalidCron_Skipped(t *testing.T) {
	env := newSchedTestEnv(t)
	insertSchedProject(t, env.pool, "proj-d")
	insertEnabledTask(t, env.pool, "task-d", "proj-d", "not-a-cron-expression", true)

	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := len(env.sched.cron.Entries()); got != 0 {
		t.Errorf("cron.Entries() = %d, want 0 (invalid cron skipped)", got)
	}
}

func TestStart_MultipleEnabledTasks(t *testing.T) {
	env := newSchedTestEnv(t)
	insertSchedProject(t, env.pool, "proj-e")
	insertEnabledTask(t, env.pool, "task-e1", "proj-e", "*/5 * * * *", true)
	insertEnabledTask(t, env.pool, "task-e2", "proj-e", "0 * * * *", true)

	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := len(env.sched.cron.Entries()); got != 2 {
		t.Errorf("cron.Entries() = %d, want 2", got)
	}
}

// -- Reload --

func TestReload_RemovesTask_ZeroEntries(t *testing.T) {
	env := newSchedTestEnv(t)
	insertSchedProject(t, env.pool, "proj-f")
	insertEnabledTask(t, env.pool, "task-f", "proj-f", "* * * * *", true)

	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, err := env.pool.Exec(`DELETE FROM scheduled_tasks WHERE id='task-f'`); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	if err := env.sched.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := len(env.sched.cron.Entries()); got != 0 {
		t.Errorf("after Reload: cron.Entries() = %d, want 0", got)
	}
}

func TestReload_AddsTask_EntryRegistered(t *testing.T) {
	env := newSchedTestEnv(t)
	insertSchedProject(t, env.pool, "proj-g")

	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	insertEnabledTask(t, env.pool, "task-g", "proj-g", "* * * * *", true)
	if err := env.sched.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := len(env.sched.cron.Entries()); got != 1 {
		t.Errorf("after Reload: cron.Entries() = %d, want 1", got)
	}
}

// -- Stop --

func TestStop_SetsCronNil(t *testing.T) {
	env := newSchedTestEnv(t)
	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	env.sched.Stop()
	if env.sched.cron != nil {
		t.Error("cron != nil after Stop, want nil")
	}
}

func TestStop_BeforeStart_Idempotent(t *testing.T) {
	env := newSchedTestEnv(t)
	env.sched.Stop()
	env.sched.Stop()
	// No panic — idempotent
}

func TestStop_DoubleAfterStart_Idempotent(t *testing.T) {
	env := newSchedTestEnv(t)
	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	env.sched.Stop()
	env.sched.Stop()
	// No panic
}

// -- RunNow --

func TestRunNow_UnknownTask_ReturnsNotFoundError(t *testing.T) {
	env := newSchedTestEnv(t)
	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err := env.sched.RunNow("no-such-task-id")
	if err == nil {
		t.Fatal("RunNow: got nil error, want not-found error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("RunNow error = %q, want to contain 'not found'", err.Error())
	}
}
