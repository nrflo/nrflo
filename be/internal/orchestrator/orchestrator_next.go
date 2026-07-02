package orchestrator

import (
	"context"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// maybeRestartEndlessLoop starts a fresh workflow instance for the same
// (project_id, workflow) when the just-completed instance had endless loop enabled
// and the stop flag was not toggled. Called from runLoop after markCompleted.
func (o *Orchestrator) maybeRestartEndlessLoop(wfiID string, req RunRequest) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(context.Background(), "endless loop: failed to open DB", "err", err)
		return
	}
	wfiRepo := repo.NewWorkflowInstanceRepo(db.WrapAsPool(database), o.clock)
	wi, err := wfiRepo.Get(wfiID)
	database.Close()
	if err != nil {
		logger.Error(context.Background(), "endless loop: failed to re-read instance", "err", err)
		return
	}
	if wi.StopEndlessLoopAfterIteration {
		logger.Info(context.Background(), "endless loop: stop flag set, exiting loop", "workflow", req.WorkflowName, "instance_id", wfiID)
		return
	}

	logger.Info(context.Background(), "endless loop: starting next iteration", "workflow", req.WorkflowName, "prev_instance_id", wfiID)

	o.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowUpdated, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id":            wfiID,
		"endless_loop_iterating": true,
	}))

	go func() {
		nextReq := RunRequest{
			ProjectID:    req.ProjectID,
			WorkflowName: req.WorkflowName,
			ScopeType:    "project",
			EndlessLoop:  true,
		}
		if _, err := o.Start(context.Background(), nextReq); err != nil {
			logger.Error(context.Background(), "endless loop: auto-restart failed", "workflow", req.WorkflowName, "err", err)
		}
	}()
}

// maybeStartNextOnSuccess spawns the workflow named in next_workflow_on_success for the
// source workflow def when finalResult is non-empty. Runs in a detached goroutine so
// source teardown cannot cancel the child. Skipped on empty finalResult, cancelled ctx,
// or when LaunchDepth >= maxNextWorkflowOnSuccessDepth.
func (o *Orchestrator) maybeStartNextOnSuccess(ctx context.Context, req RunRequest, finalResult string) {
	if finalResult == "" {
		return
	}
	if ctx.Err() != nil {
		return
	}
	if req.ParentInstanceID != "" {
		return // sub-workflow results flow back via get_subworkflow; successors would escape the caps and cascade
	}
	if req.LaunchDepth >= maxNextWorkflowOnSuccessDepth {
		logger.Warn(ctx, "next_workflow_on_success depth cap reached, skipping",
			"workflow", req.WorkflowName, "depth", req.LaunchDepth)
		return
	}

	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(context.Background(), "next_workflow_on_success: failed to open DB", "err", err)
		return
	}
	pool := db.WrapAsPool(database)
	wfSvc := service.NewWorkflowService(pool, o.clock)
	sourceDef, err := wfSvc.GetWorkflowDef(req.ProjectID, req.WorkflowName)
	database.Close()
	if err != nil {
		logger.Error(context.Background(), "next_workflow_on_success: failed to load source def",
			"workflow", req.WorkflowName, "err", err)
		return
	}
	if sourceDef.NextWorkflowOnSuccess == "" {
		return
	}

	nextWorkflow := sourceDef.NextWorkflowOnSuccess
	nextDepth := req.LaunchDepth + 1
	logger.Info(context.Background(), "next_workflow_on_success: spawning next workflow",
		"source", req.WorkflowName, "next", nextWorkflow, "depth", nextDepth)

	go func() {
		nextReq := RunRequest{
			ProjectID:    req.ProjectID,
			WorkflowName: nextWorkflow,
			ScopeType:    "project",
			Instructions: finalResult,
			LaunchDepth:  nextDepth,
		}
		if _, err := o.Start(context.Background(), nextReq); err != nil {
			logger.Error(context.Background(), "next_workflow_on_success: auto-start failed",
				"workflow", nextWorkflow, "err", err)
		}
	}()
}
