package chainrunner

import (
	"context"
	"fmt"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/ws"
)

func (r *Runner) runLoop(ctx context.Context, run *model.WorkflowChainRun) {
	defer func() {
		r.mu.Lock()
		delete(r.runs, run.ID)
		r.mu.Unlock()
	}()

	pool, err := db.NewPool(r.dataPath, db.DefaultPoolConfig())
	if err != nil {
		logger.Error(ctx, "chainrunner: failed to open DB", "run_id", run.ID, "err", err)
		r.failRun(run.ID, run.ProjectID)
		return
	}
	defer pool.Close()

	rr := repo.NewWorkflowChainRunRepo(pool, r.clock)

	for {
		select {
		case <-ctx.Done():
			r.cancelRun(ctx, pool, run)
			return
		default:
		}

		step, err := rr.GetNextPendingStep(run.ID)
		if err != nil {
			logger.Error(ctx, "chainrunner: error getting next step", "run_id", run.ID, "err", err)
			r.failRun(run.ID, run.ProjectID)
			return
		}
		if step == nil {
			if err := rr.UpdateRunStatus(run.ID, "completed"); err != nil {
				logger.Error(ctx, "chainrunner: failed to mark completed", "run_id", run.ID, "err", err)
			}
			r.broadcast(ws.EventChainRunCompleted, run.ProjectID, run.ID, nil)
			logger.Info(ctx, "chainrunner: run completed", "run_id", run.ID)
			return
		}

		success, err := r.executeStep(ctx, rr, run, step)
		if err != nil {
			if ctx.Err() != nil {
				r.cancelRun(ctx, pool, run)
				return
			}
			logger.Error(ctx, "chainrunner: step error", "run_id", run.ID, "position", step.Position, "err", err)
			r.failRun(run.ID, run.ProjectID)
			return
		}
		if !success {
			logger.Error(ctx, "chainrunner: step failed", "run_id", run.ID, "position", step.Position)
			r.failRun(run.ID, run.ProjectID)
			return
		}
	}
}

func (r *Runner) executeStep(
	ctx context.Context,
	rr *repo.WorkflowChainRunRepo,
	run *model.WorkflowChainRun,
	step *model.WorkflowChainRunStep,
) (success bool, err error) {
	if err := rr.UpdateRunStepStatus(step.ID, "running"); err != nil {
		return false, err
	}
	if err := rr.SetCurrentPosition(run.ID, step.Position); err != nil {
		return false, err
	}
	r.broadcast(ws.EventChainStepStarted, run.ProjectID, run.ID, map[string]interface{}{
		"position":      step.Position,
		"workflow_name": step.WorkflowName,
		"scope_type":    step.ScopeType,
	})

	instructions := step.InstructionsUsed
	if instructions == "" {
		instructions = run.InitialInstructions
	}

	ticketID := ""
	if step.TicketID.Valid {
		ticketID = step.TicketID.String
	}

	if step.ScopeType == "ticket" && step.RequireTicketHandoff && ticketID == "" {
		rr.UpdateRunStepStatus(step.ID, "failed") //nolint:errcheck
		return false, fmt.Errorf("missing ticket handoff: step %d requires ticket_id but none was set by the previous step", step.Position)
	}

	req := orchestrator.RunRequest{
		ProjectID:    run.ProjectID,
		WorkflowName: step.WorkflowName,
		Instructions: instructions,
		ScopeType:    step.ScopeType,
		TicketID:     ticketID,
	}

	result, err := r.orch.Start(ctx, req)
	if err != nil {
		rr.UpdateRunStepStatus(step.ID, "failed") //nolint:errcheck
		return false, err
	}

	if setErr := rr.SetRunStepInstance(step.ID, result.InstanceID, ticketID, instructions); setErr != nil { //nolint:errcheck
		logger.Error(ctx, "chainrunner: failed to set step instance", "step_id", step.ID, "err", setErr)
	}

	completed, ok := r.pollInstance(ctx, result.InstanceID)
	if !completed {
		rr.UpdateRunStepStatus(step.ID, "canceled") //nolint:errcheck
		return false, ctx.Err()
	}

	if ok {
		rr.UpdateRunStepStatus(step.ID, "completed") //nolint:errcheck
		r.broadcast(ws.EventChainStepCompleted, run.ProjectID, run.ID, map[string]interface{}{
			"position":    step.Position,
			"instance_id": result.InstanceID,
		})
		return true, nil
	}

	rr.UpdateRunStepStatus(step.ID, "failed") //nolint:errcheck
	return false, nil
}

func (r *Runner) cancelRun(ctx context.Context, pool *db.Pool, run *model.WorkflowChainRun) {
	rr := repo.NewWorkflowChainRunRepo(pool, r.clock)
	steps, _ := rr.ListRunSteps(run.ID)
	for _, s := range steps {
		if s.Status == "running" {
			if s.WorkflowInstanceID.Valid {
				r.orch.StopByInstance(run.ProjectID, s.WorkflowInstanceID.String) //nolint:errcheck
			}
			rr.UpdateRunStepStatus(s.ID, "canceled") //nolint:errcheck
		} else if s.Status == "pending" {
			rr.UpdateRunStepStatus(s.ID, "canceled") //nolint:errcheck
		}
	}
	rr.UpdateRunStatus(run.ID, "canceled") //nolint:errcheck
	r.broadcast(ws.EventChainRunFailed, run.ProjectID, run.ID, map[string]interface{}{"reason": "canceled"})
	logger.Warn(ctx, "chainrunner: run canceled", "run_id", run.ID)
}

func (r *Runner) failRun(runID, projectID string) {
	pool, err := db.NewPool(r.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return
	}
	defer pool.Close()
	rr := repo.NewWorkflowChainRunRepo(pool, r.clock)
	steps, _ := rr.ListRunSteps(runID)
	for _, s := range steps {
		if s.Status == "pending" {
			rr.UpdateRunStepStatus(s.ID, "canceled") //nolint:errcheck
		}
	}
	rr.UpdateRunStatus(runID, "failed") //nolint:errcheck
	r.broadcast(ws.EventChainRunFailed, projectID, runID, nil)
}
