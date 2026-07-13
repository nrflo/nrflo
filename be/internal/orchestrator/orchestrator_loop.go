package orchestrator

import (
	"context"
	"fmt"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/spawner/apirun/provider"
	"be/internal/ws"
)

// runLoop executes workflow phases grouped by layer.
// All agents in the same layer run concurrently. Layers execute in ascending order.
// Fan-in: consults layerPolicies[layer] (default "any"). All-skipped continues.
// All-fail stops the workflow. startLayerIdx skips layers before that index (for retry).
func (o *Orchestrator) runLoop(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	parentSession string,
	projectRoot string,
	workflows map[string]spawner.WorkflowDef,
	agents map[string]spawner.AgentConfig,
	svcWf service.SpawnerWorkflowDef,
	startLayerIdx int,
	wt *worktreeInfo,
	agentTags map[string]string,
	pre *interactivePreStep,
	lowConsumptionMode bool,
	contextSaveViaAgent bool,
	globalStallStartTimeout *int,
	globalStallRunningTimeout *int,
	modelConfigs map[string]spawner.ModelConfig,
	apiModelConfigs map[string]spawner.APIModelConfig,
	claudeSettingsJSON string,
	pushAfterMerge bool,
	projectEnv []string,
	layerPolicies map[int]string,
	layerPause map[int]bool,
) {
	// Grab done channel before any race can occur
	o.mu.Lock()
	doneCh := o.runs[wfiID].done
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		delete(o.runs, wfiID)
		o.mu.Unlock()
		if doneCh != nil {
			close(doneCh)
		}
	}()

	// Create shared pool for spawners in this orchestration run
	pool, poolErr := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if poolErr != nil {
		logger.Error(ctx, "failed to create spawner pool", "err", poolErr)
		o.markFailed(wfiID, req, "pool_init_failed")
		return
	}
	defer pool.Close()

	apiAgentSvc := newAPIAgentSvc(pool, o.clock, o.wsHub)
	findingsSvc := service.NewFindingsService(pool, o.clock)
	projectFindingsSvc := service.NewProjectFindingsService(pool, o.clock)
	agentSvcReal := service.NewAgentService(pool, o.clock)
	workflowSvcReal := service.NewWorkflowService(pool, o.clock)
	ticketSvc := service.NewTicketService(pool, o.clock)
	dispatchRepo := repo.NewDispatchRepo(pool, o.clock)
	artifactSvcRun := service.NewArtifactService(pool, o.clock, o.wsHub, o.dataPath)

	// Worktree cleanup on failure/cancellation (deferred after pool so git commands still work)
	worktreeHandled := false
	if wt != nil {
		defer func() {
			if !worktreeHandled {
				wtService := &service.WorktreeService{}
				if err := wtService.Cleanup(wt.projectRoot, wt.branchName, wt.worktreePath); err != nil {
					logger.Error(ctx, "worktree cleanup failed", "branch", wt.branchName, "err", err)
				} else {
					logger.Info(ctx, "worktree cleaned up on failure/cancel", "branch", wt.branchName)
				}
			}
		}()
	}

	target := req.TicketID
	if req.IsProjectScope() {
		target = "project:" + req.ProjectID
	}
	logger.Info(ctx, "workflow started", "workflow", req.WorkflowName, "target", target, "phases", len(svcWf.Phases))

	// Resolve per-project venv once for all script-mode agents in this run.
	// Non-blocking: failures return "" and agents fall back to PATH python3.
	pythonPath, _ := o.venvMgr.Ensure(ctx, req.ProjectID, projectRoot)

	// Group phases by layer. Merge in any already-materialized plan nodes so a
	// resumed run (retry/continue relaunch of runLoop) keeps its dynamic
	// layers instead of re-deriving only the static graph.
	materializedPhases, materializedPolicies, _ := service.LoadInstanceNodePhases(pool, o.clock, wfiID)
	for layer, policy := range materializedPolicies {
		layerPolicies[layer] = policy
	}
	layerGroups := groupPhasesByLayer(service.EffectivePhases(svcWf.Phases, materializedPhases))
	mergeMaterializedIntoSpawnerWorkflow(workflows, req.WorkflowName, materializedPhases)
	if len(materializedPhases) > 0 {
		if _, _, defProjectID, derr := o.resolveWorkflowDef(pool, req.ProjectID, req.WorkflowName); derr == nil {
			for id, cfg := range service.LoadMaterializedAgentConfigs(pool, o.clock, defProjectID, req.WorkflowName, materializedPhases) {
				agents[id] = spawner.AgentConfig{Model: cfg.Model, Timeout: cfg.Timeout}
			}
		}
	}

	// Interactive/plan pre-step: wait for PTY session to complete before starting layers
	if pre != nil {
		logger.Info(ctx, "waiting for interactive pre-step", "session_id", pre.sessionID, "mode", func() string {
			if req.PlanMode {
				return "plan"
			}
			return "interactive"
		}())
		if !waitForInteractivePreStep(ctx, pre) {
			logger.Warn(ctx, "interactive pre-step cancelled")
			o.markFailed(wfiID, req, reasonCancelled)
			return
		}
		pre.spawner.Close()

		if req.PlanMode {
			if err := handlePlanModePostStep(pre.sessionID, projectRoot, pool, wfiID, o.clock); err != nil {
				logger.Error(ctx, "plan mode post-step failed", "err", err)
				o.markFailed(wfiID, req, fmt.Sprintf("plan_read_failed: %v", err))
				return
			}
		} else {
			// Interactive mode: skip L0 (user already did the work)
			startLayerIdx = 1
		}
		logger.Info(ctx, "interactive pre-step completed", "start_layer", startLayerIdx)
	}

	callbackCount := 0

	// Read api_mode_enabled freshly at spawn time so runtime toggles take effect.
	apiModeSettingsSvc := service.NewGlobalSettingsService(pool, o.clock)
	apiModeSettingVal, _ := apiModeSettingsSvc.Get("api_mode_enabled")
	runAPIMode := apiModeSettingVal == "true"
	runAPIViaCLI, _ := apiModeSettingsSvc.GetAPIViaCLIEnabled()

	// Shared spawner config for all phases; OnSessionRegister/Unregister set per-spawn in spawnPhases.
	baseCfg := spawner.Config{
		Workflows:                 workflows,
		Agents:                    agents,
		DataPath:                  o.dataPath,
		ProjectRoot:               projectRoot,
		WSHub:                     o.wsHub,
		Pool:                      pool,
		Clock:                     o.clock,
		LowConsumptionMode:        lowConsumptionMode,
		ContextSaveViaAgent:       contextSaveViaAgent,
		GlobalStallStartTimeout:   globalStallStartTimeout,
		GlobalStallRunningTimeout: globalStallRunningTimeout,
		ClaudeSettingsJSON:        claudeSettingsJSON,
		ModelConfigs:              modelConfigs,
		APIModelConfigs:           apiModelConfigs,
		ErrorSvc:                  o.errorSvc,
		BuildAPIProvider: func(ctx context.Context, providerName, projectID string) (provider.Provider, error) {
			return buildAPIProvider(ctx, pool, o.clock, providerName, projectID)
		},
		AgentSvc:           apiAgentSvc,
		FindingsSvc:        findingsSvc,
		ProjectFindingsSvc: projectFindingsSvc,
		AgentSvcReal:       agentSvcReal,
		WorkflowSvc:        workflowSvcReal,
		TicketSvc:          ticketSvc,
		APIMode:            runAPIMode,
		APIViaCLI:          runAPIViaCLI,
		PTYManager:         o.PTYManager,
		DispatchRepo:       dispatchRepo,
		ProjectEnv:         projectEnv,
		SDKDir:             o.sdkDir,
		PythonPath:         pythonPath,
		PythonScriptRepo:   repo.NewPythonScriptRepo(pool, o.clock),
		ArtifactSvc:        artifactSvcRun,
		WorkflowControl:    apiWorkflowControl{o: o, pool: pool},
		Subworkflows:       o,
	}

	// Use index-based loop to support plan-driven jumps and forward iteration.
	// planSpliced is true once an approved plan's layers have been merged into
	// layerGroups this run — it guards reloadPlanLayers from re-triggering when
	// the now-extended layerGroups itself runs to completion.
	layerIdx := startLayerIdx
	planSpliced := len(materializedPhases) > 0
	for {
		if layerIdx >= len(layerGroups) {
			if planSpliced {
				break
			}
			newGroups, extended, terminal, wtHandled := o.reloadPlanLayers(ctx, wfiID, req, pool, svcWf, layerGroups, layerPolicies, workflows, agents)
			if extended {
				layerGroups = newGroups
				planSpliced = true
				continue
			}
			if terminal {
				worktreeHandled = wtHandled
				return
			}
			break
		}

		// Cancellation check
		select {
		case <-ctx.Done():
			logger.Warn(ctx, "workflow cancelled", "layer_idx", layerIdx)
			o.markFailed(wfiID, req, o.failReasonOr(wfiID, reasonCancelled))
			return
		default:
		}

		// === Plan execution: drain active plan steps before forward iteration ===
		if hasStep, newIdx, shouldReturn, wtHandled := o.drainCallbackPlanStep(
			ctx, wfiID, req, layerGroups, layerIdx, parentSession, baseCfg, layerPolicies, layerPause, pool, projectRoot, &callbackCount,
		); hasStep {
			layerIdx = newIdx
			if shouldReturn {
				worktreeHandled = wtHandled
				return
			}
			continue
		}

		// === Forward iteration ===
		lg := layerGroups[layerIdx]

		// Per-agent skip on skip_tags: skipped agents get skipped sessions + events;
		// the runnable subset still spawns. Whole-layer skip advances past the layer.
		runnableAgents, wholeLayerSkipped := o.applyLayerSkips(ctx, wfiID, req, lg.phases, agentTags, pool)
		if wholeLayerSkipped {
			if o.maybePauseAfterLayer(ctx, wfiID, req, layerIdx, layerGroups, layerPause, pool, projectRoot) {
				worktreeHandled = true
				return
			}
			layerIdx++
			continue
		}

		logger.Info(ctx, "running layer", "layer_idx", layerIdx+1, "total", len(layerGroups), "agents", len(runnableAgents))

		results := o.spawnPhases(ctx, wfiID, req, runnableAgents, parentSession, baseCfg)

		passCount := 0
		failCount := 0
		var cbErrs []*spawner.CallbackError
		for _, r := range results {
			if r.callbackErr != nil {
				passCount++ // callback counts as pass for layer aggregation
				cbErrs = append(cbErrs, r.callbackErr)
			} else if r.err != nil {
				if ctx.Err() != nil {
					logger.Warn(ctx, "cancelled during layer", "layer", lg.layer)
					o.markFailed(wfiID, req, o.failReasonOr(wfiID, reasonCancelled))
					return
				}
				logger.Error(ctx, "layer agent failed", "layer", lg.layer, "agent", r.agent, "err", r.err)
				failCount++
			} else {
				logger.Info(ctx, "layer agent completed", "layer", lg.layer, "agent", r.agent)
				passCount++
			}
		}

		if len(cbErrs) > 0 {
			if !o.handleCallback(ctx, wfiID, req, layerGroups, layerIdx, cbErrs, &callbackCount) {
				return
			}
			continue
		}

		// Layer aggregation: consult pass_policy (default "any" = at least one pass).
		// denom == 0 means all agents were skipped — continue regardless of policy.
		denom := passCount + failCount
		if denom > 0 {
			policy, _ := service.ParseLayerPolicy(layerPolicies[lg.layer])
			required := policy.Required(denom)
			if passCount < required {
				logger.Error(ctx, "layer pass_policy not satisfied", "layer", lg.layer,
					"policy", policy.String(), "passed", passCount, "total", denom, "required", required)
				o.markFailed(wfiID, req, fmt.Sprintf(
					"layer %d: pass_policy %q not satisfied (%d/%d passed, %d required)",
					lg.layer, policy.String(), passCount, denom, required))
				return
			}
		}

		logger.Info(ctx, "layer completed", "layer", lg.layer, "passed", passCount, "failed", failCount)
		if o.maybePauseAfterLayer(ctx, wfiID, req, layerIdx, layerGroups, layerPause, pool, projectRoot) {
			worktreeHandled = true
			return
		}
		layerIdx++
	}

	// All layers completed
	logger.Info(ctx, "workflow completed", "workflow", req.WorkflowName, "target", target)

	// Merge worktree branch on success
	if wt != nil {
		wtService := &service.WorktreeService{}
		if err := wtService.MergeAndCleanup(wt.projectRoot, wt.defaultBranch, wt.branchName, wt.worktreePath); err != nil {
			// Attempt automatic conflict resolution
			if resolveErr := o.attemptConflictResolution(ctx, wfiID, req, wt, pool, err.Error(), modelConfigs, claudeSettingsJSON, projectEnv); resolveErr != nil {
				// Resolution failed or no resolver configured — fall through to manual resolution
				logger.Error(ctx, "worktree merge failed — branch preserved for manual resolution",
					"branch", wt.branchName, "resolve_err", resolveErr, "merge_err", err)
				o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationCompleted, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
					"instance_id":   wfiID,
					"merge_error":   err.Error(),
					"branch":        wt.branchName,
					"worktree_path": wt.worktreePath,
				}))
			} else {
				logger.Info(ctx, "merge conflict resolved automatically", "branch", wt.branchName)
				o.pushIfEnabled(ctx, pushAfterMerge, wt, wfiID, req)
			}
		} else {
			logger.Info(ctx, "worktree merged and cleaned up", "branch", wt.branchName)
			o.pushIfEnabled(ctx, pushAfterMerge, wt, wfiID, req)
		}
		worktreeHandled = true
	}

	finalResult := o.markCompleted(wfiID, req)
	o.runFinalize(ctx, wfiID, req, outcomeSuccess, finalResult)
	o.maybeStartNextOnSuccess(ctx, req, finalResult)

	// Endless loop: re-run a fresh instance if enabled and not stopped
	if req.IsProjectScope() && req.EndlessLoop && ctx.Err() == nil {
		o.maybeRestartEndlessLoop(wfiID, req)
	}

	// Purge sensitive trace data when the workflow opted in (runs last, after finalize and
	// any next-workflow/endless-loop spawn so those still read the just-completed data).
	o.maybePurgeTrace(wfiID)
}
