package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// loadModelConfigs loads the enabled unified model registry once per run.
func (o *Orchestrator) loadModelConfigs(pool *db.Pool) (map[string]spawner.ModelConfig, error) {
	modelSvc := service.NewModelService(pool, o.clock)
	models, err := modelSvc.ListEnabled()
	if err != nil {
		return nil, fmt.Errorf("failed to load model configs: %w", err)
	}
	configs := make(map[string]spawner.ModelConfig, len(models))
	for _, m := range models {
		configs[m.ID] = spawner.ModelConfig{
			Provider:       m.Provider,
			CLIModel:       m.CLIModel,
			CLIContext:     m.CLIContext,
			CLIEfforts:     m.CLIEfforts,
			APIModel:       m.APIModel,
			APIContext:     m.APIContext,
			APIEfforts:     m.APIEfforts,
			FallbackModels: m.FallbackModels,
			DefaultEffort:  m.DefaultEffort,
		}
	}
	return configs, nil
}

// cliNameFromModelConfigs derives the CLI from the registry provider. Unknown
// raw model strings retain the Claude passthrough default.
func cliNameFromModelConfigs(modelConfigs map[string]spawner.ModelConfig, model string) string {
	if mc, ok := modelConfigs[model]; ok && mc.Provider == "openai" {
		return "codex"
	}
	return "claude"
}

// setupWorktree creates a git worktree for a workflow run if the project has
// worktrees enabled. Returns worktreeInfo (nil if disabled) and the effective
// projectRoot (worktree path if enabled, original path if disabled).
func setupWorktree(project *model.Project, projectRoot, branchName, scopeType string) (*worktreeInfo, string, error) {
	if scopeType == "project" {
		return nil, projectRoot, nil
	}
	if !project.UseGitWorktrees || !project.DefaultBranch.Valid {
		return nil, projectRoot, nil
	}
	defaultBranch := project.DefaultBranch.String

	wtService := &service.WorktreeService{}
	worktreePath, err := wtService.Setup(projectRoot, defaultBranch, branchName)
	if err != nil {
		return nil, "", fmt.Errorf("worktree setup failed: %w", err)
	}
	wt := &worktreeInfo{
		projectRoot:   projectRoot,
		worktreePath:  worktreePath,
		branchName:    branchName,
		defaultBranch: defaultBranch,
	}
	return wt, worktreePath, nil
}

// stopDrainTimeout bounds how long Stop/StopAll wait for a cancelled runLoop
// goroutine to fully quiesce (pool.Close, venv writes, spawned-process
// SIGTERM/SIGKILL) before giving up and returning anyway.
const stopDrainTimeout = 10 * time.Second

// Stop cancels a running orchestration and waits for its runLoop goroutine to
// fully quiesce before returning (bounded by stopDrainTimeout), so callers
// never observe residual writes to the run's working directory after Stop
// returns. If no in-memory orchestration exists (e.g. after server restart),
// falls back to cleaning up DB state directly.
func (o *Orchestrator) Stop(instanceID string) error {
	o.mu.Lock()
	rs, ok := o.runs[instanceID]
	o.mu.Unlock()

	if ok {
		rs.cancel()
		o.cancelDraftPlan(instanceID)
		if rs.done != nil {
			select {
			case <-rs.done:
			case <-time.After(stopDrainTimeout):
			}
		}
		return nil
	}

	// No in-memory orchestration — clean up orphaned DB state.
	return o.forceStopInstance(instanceID)
}

// cancelDraftPlan best-effort cancels any live draft plan for instanceID so a
// stopped run leaves no live draft (the DYNWF-4 TTL sweep is the backstop,
// not the primary path).
func (o *Orchestrator) cancelDraftPlan(instanceID string) {
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return
	}
	defer pool.Close()
	_ = repo.NewPlanRepo(pool, o.clock).Cancel(instanceID)
}

// forceStopInstance marks an orphaned workflow instance and its running sessions
// as failed directly in the DB. Used when the orchestration is no longer in memory
// (e.g. after server restart).
func (o *Orchestrator) forceStopInstance(instanceID string) error {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)

	wi, err := wfiRepo.Get(instanceID)
	if err != nil {
		return fmt.Errorf("no running orchestration for instance %s", instanceID)
	}
	planSuspended := model.IsPlanSuspended(wi.Status)
	if wi.Status != model.WorkflowInstanceActive && !planSuspended {
		return fmt.Errorf("instance %s is not active (status: %s)", instanceID, wi.Status)
	}

	// A plan-suspended instance is not in o.runs, so it has no live sessions to
	// fail — but it may still hold a live draft plan; leave no draft behind.
	if planSuspended {
		_ = repo.NewPlanRepo(pool, o.clock).Cancel(instanceID)
	} else {
		// Mark running sessions as failed.
		asRepo := repo.NewAgentSessionRepo(pool, o.clock)
		asRepo.FailRunningByInstance(instanceID)
	}

	// Mark workflow instance as failed.
	wfiRepo.UpdateStatus(instanceID, model.WorkflowInstanceFailed)
	o.updateOrchestrationStatus(instanceID, "failed")

	logger.Info(context.Background(), "force-stopped orphaned instance", "instance_id", instanceID)

	// Broadcast so UI updates.
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationFailed, wi.ProjectID, wi.TicketID, wi.WorkflowID, map[string]interface{}{
		"instance_id": instanceID,
		"reason":      "force_stopped",
	}))

	// Purge sensitive trace data when the workflow opted in (orphaned force-stop path).
	o.maybePurgeTrace(instanceID)
	return nil
}

// readRunConsumptionSettings reads consumption-mode and stall-timeout settings from the
// shared pool. Read once at workflow start and retry.
func readRunConsumptionSettings(pool *db.Pool) (lowConsumptionMode bool, globalStallStartTimeout, globalStallRunningTimeout *int) {
	if val, _ := pool.GetConfig("low_consumption_mode"); val == "true" {
		lowConsumptionMode = true
	}
	if val, _ := pool.GetConfig("stall_start_timeout_sec"); val != "" {
		if parsed, parseErr := strconv.Atoi(val); parseErr == nil {
			globalStallStartTimeout = &parsed
		}
	}
	if val, _ := pool.GetConfig("stall_running_timeout_sec"); val != "" {
		if parsed, parseErr := strconv.Atoi(val); parseErr == nil {
			globalStallRunningTimeout = &parsed
		}
	}
	return
}
