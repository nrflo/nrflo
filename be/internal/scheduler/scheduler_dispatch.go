package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// dispatch inserts a ScheduleRun row, fans out one goroutine per workflow and per
// chain ID calling orchestrator.Start / wfChainRunSvc.CreateRun+wfChainRunner.Start,
// collects results, and updates the run. It also updates last_triggered_at / next_run_at.
func (s *Scheduler) dispatch(ctx context.Context, task *model.ScheduledTask) (*model.ScheduleRun, error) {
	runRepo := repo.NewScheduleRunRepo(s.pool, s.clock)
	taskRepo := repo.NewScheduledTaskRepo(s.pool, s.clock)

	// v1: workflow-name-keyed special case — generalise as min_interval_since_last_run on workflow_definitions later
	for _, wfName := range task.Workflows {
		if wfName == "claude-limits-refresh" {
			svc := service.NewClaudeLimitsService(s.pool, s.clock)
			limits, _ := svc.Get()
			if limits.UpdatedAt != "" {
				if updatedAt, parseErr := time.Parse(time.RFC3339, limits.UpdatedAt); parseErr == nil && !updatedAt.IsZero() && s.clock.Now().UTC().Sub(updatedAt) < 20*time.Minute {
					logger.Info(ctx, "scheduler: claude-limits-refresh skipped, limits fresh", "updated_at", limits.UpdatedAt)
					now := s.clock.Now().UTC()
					var nextRunAt *time.Time
					if sched, cronErr := cron.ParseStandard(task.CronExpression); cronErr == nil {
						next := sched.Next(now)
						nextRunAt = &next
					}
					if err := taskRepo.UpdateTriggerTimestamps(task.ID, &now, nextRunAt); err != nil {
						logger.Info(ctx, "scheduler: failed to update task timestamps", "id", task.ID, "err", err)
					}
					return nil, nil
				}
			}
			break
		}
	}

	// 1. Insert ScheduleRun with status=pending
	run := &model.ScheduleRun{
		ID:              uuid.New().String(),
		ScheduledTaskID: task.ID,
		ProjectID:       task.ProjectID,
		TriggeredAt:     s.clock.Now().UTC(),
		Status:          "pending",
		Workflows:       []model.ScheduleRunWorkflow{},
		ChainRuns:       []model.ScheduleRunChain{},
	}
	if err := runRepo.Insert(run); err != nil {
		return nil, err
	}

	// 2. Fan out per-workflow goroutines
	type wfResult struct {
		workflow   string
		instanceID string
		errMsg     string
	}
	wfResults := make([]wfResult, len(task.Workflows))
	var wg sync.WaitGroup

	for i, wfName := range task.Workflows {
		i, wfName := i, wfName
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := orchestrator.RunRequest{
				ProjectID:       task.ProjectID,
				WorkflowName:    wfName,
				ScopeType:       "project",
				ScheduledTaskID: task.ID,
			}
			runResult, err := s.orch.Start(ctx, req)
			if err != nil {
				wfResults[i] = wfResult{workflow: wfName, errMsg: err.Error()}
				return
			}
			wfResults[i] = wfResult{workflow: wfName, instanceID: runResult.InstanceID}
		}()
	}

	// 3. Fan out per-chain goroutines
	type chainResult struct {
		chainID    string
		chainRunID string
		errMsg     string
	}
	chainResults := make([]chainResult, len(task.WorkflowChainIDs))

	for i, chainID := range task.WorkflowChainIDs {
		i, chainID := i, chainID
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.wfChainRunSvc == nil || s.wfChainRunner == nil {
				chainResults[i] = chainResult{chainID: chainID, errMsg: "chain runner not available"}
				return
			}
			triggeredBy := fmt.Sprintf("schedule:%s", task.ID)
			detail, err := s.wfChainRunSvc.CreateRun(task.ProjectID, chainID, "", triggeredBy)
			if err != nil {
				chainResults[i] = chainResult{chainID: chainID, errMsg: err.Error()}
				return
			}
			if err := s.wfChainRunner.Start(ctx, detail.ID); err != nil {
				chainResults[i] = chainResult{chainID: chainID, chainRunID: detail.ID, errMsg: err.Error()}
				return
			}
			chainResults[i] = chainResult{chainID: chainID, chainRunID: detail.ID}
		}()
	}

	wg.Wait()

	// 4. Build ScheduleRunWorkflow slice
	runWorkflows := make([]model.ScheduleRunWorkflow, len(wfResults))
	anyFailed := false
	overallErr := ""
	for i, r := range wfResults {
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

	// 5. Build ScheduleRunChain slice
	runChains := make([]model.ScheduleRunChain, len(chainResults))
	for i, r := range chainResults {
		runChains[i] = model.ScheduleRunChain{
			ChainID:    r.chainID,
			ChainRunID: r.chainRunID,
			Error:      r.errMsg,
		}
		if r.errMsg != "" {
			anyFailed = true
			if overallErr == "" {
				overallErr = r.errMsg
			}
		}
	}

	// 6. Determine final status: failed only if all items (workflows + chains) failed
	totalItems := len(task.Workflows) + len(task.WorkflowChainIDs)
	finalStatus := "triggered"
	if anyFailed && totalItems > 0 {
		allFailed := true
		for _, r := range wfResults {
			if r.errMsg == "" {
				allFailed = false
				break
			}
		}
		if allFailed {
			for _, r := range chainResults {
				if r.errMsg == "" {
					allFailed = false
					break
				}
			}
		}
		if allFailed {
			finalStatus = "failed"
		}
	}

	// 7. Marshal and persist
	wfJSON, _ := json.Marshal(runWorkflows)
	chainJSON, _ := json.Marshal(runChains)
	if err := runRepo.UpdateStatusFull(run.ID, finalStatus, string(wfJSON), string(chainJSON), overallErr); err != nil {
		logger.Info(ctx, "scheduler: failed to update run status", "run_id", run.ID, "err", err)
	}

	// 8. Update task timestamps
	now := s.clock.Now().UTC()
	var nextRunAt *time.Time
	if sched, err := cron.ParseStandard(task.CronExpression); err == nil {
		next := sched.Next(now)
		nextRunAt = &next
	}
	if err := taskRepo.UpdateTriggerTimestamps(task.ID, &now, nextRunAt); err != nil {
		logger.Info(ctx, "scheduler: failed to update task timestamps", "id", task.ID, "err", err)
	}

	// 9. Broadcast EventScheduleTriggered
	run.Status = finalStatus
	run.Workflows = runWorkflows
	run.ChainRuns = runChains
	s.hub.Broadcast(ws.NewEvent(ws.EventScheduleTriggered, task.ProjectID, "", "", map[string]interface{}{
		"task_id":    task.ID,
		"run_id":     run.ID,
		"status":     finalStatus,
		"chain_runs": runChains,
	}))

	return run, nil
}
