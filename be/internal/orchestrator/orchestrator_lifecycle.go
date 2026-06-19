package orchestrator

import (
	"context"
	"fmt"
	"strconv"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// loadModelConfigs loads CLI model configs from the database and builds a map
// suitable for spawner.Config.ModelConfigs. Called once at workflow start.
func (o *Orchestrator) loadModelConfigs(pool *db.Pool) (map[string]spawner.ModelConfig, error) {
	cliModelSvc := service.NewCLIModelService(pool, o.clock)
	models, err := cliModelSvc.ListEnabled()
	if err != nil {
		return nil, fmt.Errorf("failed to load CLI model configs: %w", err)
	}
	configs := make(map[string]spawner.ModelConfig, len(models))
	for _, m := range models {
		configs[m.ID] = spawner.ModelConfig{
			CLIType:         m.CLIType,
			MappedModel:     m.MappedModel,
			ReasoningEffort: m.ReasoningEffort,
			FallbackModels:  m.FallbackModels,
			ContextLength:   m.ContextLength,
		}
	}
	return configs, nil
}

// cliNameFromModelConfigs resolves the CLI name for a model using DB configs,
// falling back to spawner.DefaultCLIForModel if not found.
func cliNameFromModelConfigs(modelConfigs map[string]spawner.ModelConfig, model string) string {
	if mc, ok := modelConfigs[model]; ok && mc.CLIType != "" {
		return mc.CLIType
	}
	return spawner.DefaultCLIForModel(model)
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

// Stop cancels a running orchestration. If no in-memory orchestration exists
// (e.g. after server restart), falls back to cleaning up DB state directly.
func (o *Orchestrator) Stop(instanceID string) error {
	o.mu.Lock()
	rs, ok := o.runs[instanceID]
	o.mu.Unlock()

	if ok {
		rs.cancel()
		return nil
	}

	// No in-memory orchestration — clean up orphaned DB state.
	return o.forceStopInstance(instanceID)
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
	if wi.Status != model.WorkflowInstanceActive {
		return fmt.Errorf("instance %s is not active (status: %s)", instanceID, wi.Status)
	}

	// Mark running sessions as failed.
	asRepo := repo.NewAgentSessionRepo(pool, o.clock)
	asRepo.FailRunningByInstance(instanceID)

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
func readRunConsumptionSettings(pool *db.Pool) (lowConsumptionMode, contextSaveViaAgent bool, globalStallStartTimeout, globalStallRunningTimeout *int) {
	if val, _ := pool.GetConfig("low_consumption_mode"); val == "true" {
		lowConsumptionMode = true
	}
	if val, _ := pool.GetConfig("context_save_via_agent"); val == "true" {
		contextSaveViaAgent = true
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
