package scheduler

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/ws"
)

// dispatch inserts a ScheduleRun row, fans out one goroutine per workflow calling
// orchestrator.Start with ScopeType="project", collects results, and updates the run.
// It also updates last_triggered_at / next_run_at on the task.
func (s *Scheduler) dispatch(ctx context.Context, task *model.ScheduledTask) (*model.ScheduleRun, error) {
	runRepo := repo.NewScheduleRunRepo(s.pool, s.clock)
	taskRepo := repo.NewScheduledTaskRepo(s.pool, s.clock)

	// 1. Insert ScheduleRun with status=pending
	run := &model.ScheduleRun{
		ID:              uuid.New().String(),
		ScheduledTaskID: task.ID,
		ProjectID:       task.ProjectID,
		TriggeredAt:     s.clock.Now().UTC(),
		Status:          "pending",
		Workflows:       []model.ScheduleRunWorkflow{},
	}
	if err := runRepo.Insert(run); err != nil {
		return nil, err
	}

	// 2. Fan out per-workflow goroutines
	type result struct {
		workflow   string
		instanceID string
		errMsg     string
	}
	results := make([]result, len(task.Workflows))
	var wg sync.WaitGroup

	for i, wfName := range task.Workflows {
		i, wfName := i, wfName
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := orchestrator.RunRequest{
				ProjectID:    task.ProjectID,
				WorkflowName: wfName,
				ScopeType:    "project",
			}
			runResult, err := s.orch.Start(ctx, req)
			if err != nil {
				results[i] = result{workflow: wfName, errMsg: err.Error()}
				return
			}
			results[i] = result{workflow: wfName, instanceID: runResult.InstanceID}
		}()
	}
	wg.Wait()

	// 3. Build ScheduleRunWorkflow slice
	runWorkflows := make([]model.ScheduleRunWorkflow, len(results))
	overallErr := ""
	anyFailed := false
	for i, r := range results {
		runWorkflows[i] = model.ScheduleRunWorkflow{
			Workflow:   r.workflow,
			InstanceID: r.instanceID,
			Error:      r.errMsg,
		}
		if r.errMsg != "" {
			anyFailed = true
			if overallErr == "" {
				overallErr = r.errMsg
			}
		}
	}

	// 4. Marshal workflows JSON and update run status
	wfJSON, _ := json.Marshal(runWorkflows)
	finalStatus := "triggered"
	if anyFailed && len(task.Workflows) > 0 {
		allFailed := true
		for _, r := range results {
			if r.errMsg == "" {
				allFailed = false
				break
			}
		}
		if allFailed {
			finalStatus = "failed"
		}
	}
	if err := runRepo.UpdateStatus(run.ID, finalStatus, string(wfJSON), overallErr); err != nil {
		logger.Info(ctx, "scheduler: failed to update run status", "run_id", run.ID, "err", err)
	}

	// 5. Update task timestamps
	now := s.clock.Now().UTC()
	var nextRunAt *time.Time
	if sched, err := cron.ParseStandard(task.CronExpression); err == nil {
		next := sched.Next(now)
		nextRunAt = &next
	}
	if err := taskRepo.UpdateTriggerTimestamps(task.ID, &now, nextRunAt); err != nil {
		logger.Info(ctx, "scheduler: failed to update task timestamps", "id", task.ID, "err", err)
	}

	// 6. Broadcast EventScheduleTriggered
	run.Status = finalStatus
	run.Workflows = runWorkflows
	s.hub.Broadcast(ws.NewEvent(ws.EventScheduleTriggered, task.ProjectID, "", "", map[string]interface{}{
		"task_id": task.ID,
		"run_id":  run.ID,
		"status":  finalStatus,
	}))

	return run, nil
}
