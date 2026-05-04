// Package scheduler manages cron-driven scheduled workflow triggers.
package scheduler

import (
	"context"
	"sync"

	"github.com/robfig/cron/v3"

	"be/internal/chainrunner"
	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// Scheduler drives per-project cron schedules that trigger orchestrator.Start calls.
type Scheduler struct {
	pool            *db.Pool
	orch            *orchestrator.Orchestrator
	hub             *ws.Hub
	clock           clock.Clock
	mu              sync.Mutex
	cron            *cron.Cron
	ctx             context.Context
	wfChainRunSvc   *service.WorkflowChainRunService
	wfChainRunner   *chainrunner.Runner
}

// New constructs a Scheduler. Call Start(ctx) to begin scheduling.
func New(pool *db.Pool, orch *orchestrator.Orchestrator, hub *ws.Hub, clk clock.Clock, wfChainRunSvc *service.WorkflowChainRunService, wfChainRunner *chainrunner.Runner) *Scheduler {
	return &Scheduler{pool: pool, orch: orch, hub: hub, clock: clk, wfChainRunSvc: wfChainRunSvc, wfChainRunner: wfChainRunner}
}

// Start loads enabled tasks and registers cron entries. Stores ctx for dispatched runs.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx = ctx
	return s.rebuild()
}

// Reload stops the current cron, re-loads enabled tasks, and restarts scheduling.
// Call after any Create/Update/Delete on scheduled_tasks.
func (s *Scheduler) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopCron()
	return s.rebuild()
}

// Stop drains in-flight cron jobs and halts scheduling.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopCron()
}

// RunNow dispatches a task immediately regardless of its schedule.
// Returns the inserted ScheduleRun row.
func (s *Scheduler) RunNow(taskID string) (*model.ScheduleRun, error) {
	taskRepo := repo.NewScheduledTaskRepo(s.pool, s.clock)
	task, err := taskRepo.Get(taskID)
	if err != nil {
		return nil, err
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return s.dispatch(ctx, task)
}

// rebuild registers all enabled tasks with a fresh cron.Cron instance.
// Caller must hold s.mu.
func (s *Scheduler) rebuild() error {
	taskRepo := repo.NewScheduledTaskRepo(s.pool, s.clock)
	tasks, err := taskRepo.ListEnabled()
	if err != nil {
		return err
	}

	c := cron.New()
	now := s.clock.Now()

	for _, task := range tasks {
		task := task // capture loop var
		sched, err := cron.ParseStandard(task.CronExpression)
		if err != nil {
			logger.Info(context.Background(), "scheduler: skipping task with invalid cron", "id", task.ID, "expr", task.CronExpression, "err", err)
			continue
		}

		nextRun := sched.Next(now)
		if updateErr := taskRepo.UpdateTriggerTimestamps(task.ID, task.LastTriggeredAt, &nextRun); updateErr != nil {
			logger.Info(context.Background(), "scheduler: failed to update next_run_at", "id", task.ID, "err", updateErr)
		}

		c.Schedule(sched, cron.FuncJob(func() {
			ctx := s.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			freshRepo := repo.NewScheduledTaskRepo(s.pool, s.clock)
			freshTask, getErr := freshRepo.Get(task.ID)
			if getErr != nil {
				logger.Info(ctx, "scheduler: failed to reload task before dispatch", "id", task.ID, "err", getErr)
				return
			}
			if _, dispatchErr := s.dispatch(ctx, freshTask); dispatchErr != nil {
				logger.Info(ctx, "scheduler: dispatch error", "id", task.ID, "err", dispatchErr)
			}
		}))
	}

	s.cron = c
	c.Start()
	return nil
}

// stopCron stops the cron scheduler and waits for in-flight jobs to finish.
// Caller must hold s.mu.
func (s *Scheduler) stopCron() {
	if s.cron == nil {
		return
	}
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	s.cron = nil
}
