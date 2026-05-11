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
	"be/internal/service"
	"be/internal/ws"
)

// setupDispatchEnvWithClock creates an isolated environment with a caller-supplied clock.
// Follows the same pattern as setupDispatchEnv but accepts an explicit clock.Clock so
// tests can manipulate time without sleeping.
func setupDispatchEnvWithClock(t *testing.T, clk clock.Clock) (*Scheduler, *db.Pool) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "dispatch_limits.db")
	if err := schedCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	orch := orchestrator.New(dbPath, hub, clk, nil, false, "")
	sched := New(pool, orch, hub, clk, nil, nil)
	t.Cleanup(func() {
		orch.StopAll()
		sched.Stop()
		hub.Stop()
		pool.Close()
	})
	return sched, pool
}

// TestDispatch_ClaudeLimitsRefresh_FreshLimits_SkipsDispatch verifies that when
// claude_limits_updated_at is within 20 minutes of now, dispatch short-circuits:
// it returns (nil, nil), inserts no schedule_runs row, inserts no workflow_instances
// row, but still advances last_triggered_at and next_run_at on the task.
func TestDispatch_ClaudeLimitsRefresh_FreshLimits_SkipsDispatch(t *testing.T) {
	fixedTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	testClock := clock.NewTest(fixedTime)
	sched, pool := setupDispatchEnvWithClock(t, testClock)
	seedDispatchProject(t, pool, "proj-fresh-skip")

	// Seed limits as fresh (updated_at = fixedTime).
	limitsSvc := service.NewClaudeLimitsService(pool, testClock)
	if err := limitsSvc.Update(service.ClaudeLimits{FiveHourUsedPct: 50, SevenDayUsedPct: 30}); err != nil {
		t.Fatalf("limitsSvc.Update: %v", err)
	}

	task := makeDispatchTask("task-fresh-skip", "proj-fresh-skip", []string{"claude-limits-refresh"})
	insertDispatchTask(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch returned unexpected error: %v", err)
	}
	if run != nil {
		t.Errorf("dispatch returned non-nil run = %+v, want nil (skip-if-fresh)", run)
	}

	// No schedule_runs row.
	var runCount int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM schedule_runs WHERE scheduled_task_id = ?`, task.ID).Scan(&runCount); err != nil {
		t.Fatalf("query schedule_runs: %v", err)
	}
	if runCount != 0 {
		t.Errorf("schedule_runs count = %d, want 0 (dispatch was skipped)", runCount)
	}

	// No workflow_instances row.
	var wiCount int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances WHERE project_id = ?`, task.ProjectID).Scan(&wiCount); err != nil {
		t.Fatalf("query workflow_instances: %v", err)
	}
	if wiCount != 0 {
		t.Errorf("workflow_instances count = %d, want 0 (dispatch was skipped)", wiCount)
	}

	// Timestamps must still be advanced even when skipping.
	taskRepo := repo.NewScheduledTaskRepo(pool, testClock)
	updated, err := taskRepo.Get(task.ID)
	if err != nil {
		t.Fatalf("taskRepo.Get: %v", err)
	}
	if updated.LastTriggeredAt == nil {
		t.Error("LastTriggeredAt = nil after skip, want non-nil (timestamps advanced)")
	}
	if updated.NextRunAt == nil {
		t.Error("NextRunAt = nil after skip, want non-nil (next cron tick computed)")
	}
}

// TestDispatch_ClaudeLimitsRefresh_ProceedsWhenNotFresh uses a table-driven approach
// to cover both the stale-limits case (updated >20 min ago) and the absent-limits case
// (UpdatedAt never written). In both cases dispatch must proceed and insert a
// schedule_runs row.
func TestDispatch_ClaudeLimitsRefresh_ProceedsWhenNotFresh(t *testing.T) {
	fixedTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		seedLimits bool
		offsetMins int // minutes in the past for the limit timestamp; ignored when !seedLimits
	}{
		{name: "stale_limits_25min", seedLimits: true, offsetMins: 25},
		{name: "no_limits_set", seedLimits: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			testClock := clock.NewTest(fixedTime)
			sched, pool := setupDispatchEnvWithClock(t, testClock)
			projectID := "proj-proceed-" + tc.name
			seedDispatchProject(t, pool, projectID)

			if tc.seedLimits {
				// Seed limits with a timestamp in the past so the guard sees them as stale.
				staleClock := clock.NewTest(fixedTime.Add(-time.Duration(tc.offsetMins) * time.Minute))
				limitsSvc := service.NewClaudeLimitsService(pool, staleClock)
				if err := limitsSvc.Update(service.ClaudeLimits{FiveHourUsedPct: 70, SevenDayUsedPct: 20}); err != nil {
					t.Fatalf("limitsSvc.Update: %v", err)
				}
			}

			task := makeDispatchTask("task-proceed-"+tc.name, projectID, []string{"claude-limits-refresh"})
			insertDispatchTask(t, pool, task)

			run, err := sched.dispatch(context.Background(), task)
			if err != nil {
				t.Fatalf("dispatch error: %v", err)
			}
			if run == nil {
				t.Fatal("dispatch returned nil run, want non-nil (guard should not skip)")
			}

			var runCount int
			if err := pool.QueryRow(`SELECT COUNT(*) FROM schedule_runs WHERE scheduled_task_id = ?`, task.ID).Scan(&runCount); err != nil {
				t.Fatalf("query schedule_runs: %v", err)
			}
			if runCount == 0 {
				t.Error("schedule_runs count = 0, want ≥1 (dispatch proceeded)")
			}
		})
	}
}

// TestDispatch_ClaudeLimitsRefresh_BoundaryAt20Min verifies that limits updated
// exactly 20 minutes ago are considered fresh (< 20min strict inequality → skip).
func TestDispatch_ClaudeLimitsRefresh_BoundaryAt20Min(t *testing.T) {
	fixedTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	testClock := clock.NewTest(fixedTime)
	sched, pool := setupDispatchEnvWithClock(t, testClock)
	seedDispatchProject(t, pool, "proj-boundary")

	// Seed limits exactly 20 minutes ago — dispatch code uses Sub(...) < 20*time.Minute.
	// At exactly 20 min the condition is false (not < 20), so dispatch proceeds.
	boundary := fixedTime.Add(-20 * time.Minute)
	boundaryClock := clock.NewTest(boundary)
	limitsSvc := service.NewClaudeLimitsService(pool, boundaryClock)
	if err := limitsSvc.Update(service.ClaudeLimits{FiveHourUsedPct: 60, SevenDayUsedPct: 10}); err != nil {
		t.Fatalf("limitsSvc.Update: %v", err)
	}

	task := makeDispatchTask("task-boundary", "proj-boundary", []string{"claude-limits-refresh"})
	insertDispatchTask(t, pool, task)

	run, err := sched.dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	// At exactly 20 min the guard condition (< 20min) is false → dispatch proceeds.
	if run == nil {
		t.Error("dispatch returned nil run at exactly 20-min boundary, want non-nil (proceed)")
	}
}

// TestMigration100_ClaudeLimitsRefresherSeeded is a smoke test for migration 000100.
// It opens a fresh database (running all migrations) and verifies that the
// claude-limits-refresher system agent definition was seeded with the expected fields.
func TestMigration100_ClaudeLimitsRefresherSeeded(t *testing.T) {
	tmpPath := filepath.Join(t.TempDir(), "mig100_smoke.db")
	pool, err := db.NewPoolPath(tmpPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	svc := service.NewSystemAgentDefinitionService(pool, clock.Real())
	def, err := svc.GetForBackend("claude-limits-refresher", "cli")
	if err != nil {
		t.Fatalf("GetForBackend(claude-limits-refresher, cli): %v", err)
	}

	if def.Model != "haiku" {
		t.Errorf("model = %q, want 'haiku'", def.Model)
	}
	if def.Timeout != 1 {
		t.Errorf("timeout = %d, want 1", def.Timeout)
	}
	if def.StallStartTimeoutSec == nil || *def.StallStartTimeoutSec != 30 {
		t.Errorf("stall_start_timeout_sec = %v, want 30", def.StallStartTimeoutSec)
	}
	if def.StallRunningTimeoutSec == nil || *def.StallRunningTimeoutSec != 60 {
		t.Errorf("stall_running_timeout_sec = %v, want 60", def.StallRunningTimeoutSec)
	}
	if !strings.Contains(def.Prompt, "exit") {
		t.Errorf("prompt = %q, want to contain 'exit'", def.Prompt)
	}
	if def.ExecutionMode != "cli" {
		t.Errorf("execution_mode = %q, want 'cli'", def.ExecutionMode)
	}
}
