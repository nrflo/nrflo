package service

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/types"
)

func TestScheduledTaskService_Update_ScheduleChangeClearsNextRun(t *testing.T) {
	t.Parallel()
	svc, pool, _, cleanup := setupScheduledTaskTestEnv(t)
	defer cleanup()
	seedProjectAndWorkflow(t, pool, "proj-cursor", "wf-cursor", "project")

	if _, err := svc.Create("proj-cursor", &types.ScheduledTaskCreateRequest{
		ID: "task-cursor", Name: "T", CronExpression: "0 * * * *", Workflows: []string{"wf-cursor"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	next := time.Now().Add(time.Hour)
	if err := repo.NewScheduledTaskRepo(pool, clock.Real()).UpdateTriggerTimestamps("task-cursor", nil, &next); err != nil {
		t.Fatalf("seed next run: %v", err)
	}

	newCron := "15 * * * *"
	task, err := svc.Update("task-cursor", &types.ScheduledTaskUpdateRequest{CronExpression: &newCron})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if task.NextRunAt != nil {
		t.Fatalf("NextRunAt = %v, want nil after cron change", task.NextRunAt)
	}
}
