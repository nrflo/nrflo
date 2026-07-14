package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/repo"
	"be/internal/spawner"
)

// waitForInteractivePreStep blocks until the interactive PTY session completes
// or the context is cancelled. Returns true if completed normally, false if cancelled.
func waitForInteractivePreStep(ctx context.Context, pre *interactivePreStep) bool {
	select {
	case <-pre.waitCh:
		return true
	case <-ctx.Done():
		return false
	}
}

// runInteractivePreStep waits for the interactive/plan pre-step to complete,
// runs its cleanup, and (in plan mode) stores the captured plan as
// user_instructions. Returns the layer index to resume at and false when the
// caller should stop the run (already marked failed).
func (o *Orchestrator) runInteractivePreStep(ctx context.Context, wfiID string, req RunRequest, pool *db.Pool, projectRoot string, pre *interactivePreStep, startLayerIdx int) (int, bool) {
	logger.Info(ctx, "waiting for interactive pre-step", "session_id", pre.sessionID, "mode", func() string {
		if req.PlanMode {
			return "plan"
		}
		return "interactive"
	}())
	// The plan file lives in projectRoot (a worktree for ticket runs, the repo
	// itself for project-scoped runs). Remove it once the plan has been read —
	// deferred, since cleanup() below runs before adapter.ReadPlan — so it never
	// lingers untracked for the next agent to commit.
	if pre.planFile != "" {
		defer func() { _ = os.Remove(pre.planFile) }()
	}

	completedNormally := waitForInteractivePreStep(ctx, pre)
	if !completedNormally && o.OnClosePtySession != nil {
		// Cancelled: the CLI is still running. Close its PTY before cleanup()
		// deletes the files it depends on (codex's per-session CODEX_HOME).
		o.OnClosePtySession(pre.sessionID)
	}
	if pre.cleanup != nil {
		pre.cleanup()
	}
	if !completedNormally {
		logger.Warn(ctx, "interactive pre-step cancelled")
		o.markFailed(wfiID, req, reasonCancelled)
		return startLayerIdx, false
	}
	pre.spawner.Close()

	if req.PlanMode {
		planCapture := spawner.PlanCaptureOptions{SessionID: pre.sessionID, WorkDir: projectRoot, PlanFile: pre.planFile}
		if err := handlePlanModePostStep(pre.adapter, planCapture, pool, wfiID, o.clock); err != nil {
			logger.Error(ctx, "plan mode post-step failed", "err", err)
			o.markFailed(wfiID, req, fmt.Sprintf("plan_read_failed: %v", err))
			return startLayerIdx, false
		}
	} else {
		// Interactive mode: skip L0 (user already did the work)
		startLayerIdx = 1
	}
	logger.Info(ctx, "interactive pre-step completed", "start_layer", startLayerIdx)
	return startLayerIdx, true
}

// handlePlanModePostStep reads the plan file via the adapter that ran the
// session and stores it as user_instructions. Returns an error if no plan
// content is found.
func handlePlanModePostStep(adapter spawner.CLIAdapter, planCapture spawner.PlanCaptureOptions, pool *db.Pool, wfiID string, clk clock.Clock) error {
	planContent := adapter.ReadPlan(planCapture)
	if planContent == "" {
		return fmt.Errorf("no plan file found for session %s", planCapture.SessionID)
	}

	findingRepo := repo.NewFindingRepo(pool, clk)
	instrVal, _ := json.Marshal(planContent)
	if err := findingRepo.Upsert("workflow_instance", wfiID, "user_instructions", instrVal,
		repo.Denorm{WorkflowInstanceID: wfiID},
		repo.Actor{Source: "orchestrator"}); err != nil {
		return fmt.Errorf("failed to store user_instructions finding: %w", err)
	}

	if err := repo.NewAgentSessionRepo(pool, clk).UpdateStatusToInteractiveCompleted(planCapture.SessionID); err != nil {
		logger.Error(context.Background(), "failed to mark planner session interactive_completed", "session_id", planCapture.SessionID, "err", err)
		return err
	}

	logger.Info(context.Background(), "plan file stored as user_instructions", "wfi_id", wfiID, "plan_length", len(planContent))
	return nil
}
