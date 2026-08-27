package scheduler

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

func TestStart_PersistsNextRunInCronTimezone(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}
	clk := clock.NewTest(time.Date(2026, 8, 27, 16, 2, 0, 0, loc))
	env := newSchedTestEnvWithClock(t, clk)
	insertSchedProject(t, env.pool, "proj-tz")
	insertEnabledTask(t, env.pool, "task-tz", "proj-tz", "0 8-22/2 * * *", true)

	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	task, err := repo.NewScheduledTaskRepo(env.pool, clk).Get("task-tz")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := time.Date(2026, 8, 27, 18, 0, 0, 0, loc)
	if task.NextRunAt == nil || !task.NextRunAt.Equal(want) {
		t.Fatalf("NextRunAt = %v, want %v", task.NextRunAt, want)
	}
}

func TestStart_RecordsMissedOccurrencesAsSkipped(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}
	clk := clock.NewTest(time.Date(2026, 8, 27, 15, 5, 0, 0, loc))
	env := newSchedTestEnvWithClock(t, clk)
	insertSchedProject(t, env.pool, "proj-skipped")
	insertEnabledTask(t, env.pool, "task-skipped", "proj-skipped", "15 2,8,14,20 * * *", true)

	lastTriggered := time.Date(2026, 8, 26, 20, 15, 0, 0, loc)
	firstMissed := time.Date(2026, 8, 27, 2, 15, 0, 0, loc)
	taskRepo := repo.NewScheduledTaskRepo(env.pool, clk)
	if err := taskRepo.UpdateTriggerTimestamps("task-skipped", &lastTriggered, &firstMissed); err != nil {
		t.Fatalf("seed timestamps: %v", err)
	}

	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	runs, err := repo.NewScheduleRunRepo(env.pool, clk).ListByTask("task-skipped", 10, 0)
	if err != nil {
		t.Fatalf("ListByTask: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("skipped runs = %d, want 3", len(runs))
	}
	wantHours := []int{14, 8, 2}
	for i, run := range runs {
		if run.Status != "skipped" || run.Error != missedRunReason {
			t.Errorf("run[%d] = status %q error %q, want skipped/%s", i, run.Status, run.Error, missedRunReason)
		}
		if got := run.TriggeredAt.In(loc).Hour(); got != wantHours[i] {
			t.Errorf("run[%d] hour = %d, want %d", i, got, wantHours[i])
		}
	}

	task, err := taskRepo.Get("task-skipped")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	wantNext := time.Date(2026, 8, 27, 20, 15, 0, 0, loc)
	if task.NextRunAt == nil || !task.NextRunAt.Equal(wantNext) {
		t.Fatalf("NextRunAt = %v, want %v", task.NextRunAt, wantNext)
	}

	env.sched.Stop()
	if err := env.sched.Start(context.Background()); err != nil {
		t.Fatalf("restart: %v", err)
	}
	runs, err = repo.NewScheduleRunRepo(env.pool, clk).ListByTask("task-skipped", 10, 0)
	if err != nil {
		t.Fatalf("ListByTask after restart: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("skipped runs after restart = %d, want 3", len(runs))
	}
}
