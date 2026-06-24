package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

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

	// Group phases by layer
	layerGroups := groupPhasesByLayer(svcWf.Phases)

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
		DeepResearch:       o,
	}

	// Use index-based loop to support plan-driven jumps and forward iteration.
	layerIdx := startLayerIdx
	for layerIdx < len(layerGroups) {
		// Cancellation check
		select {
		case <-ctx.Done():
			logger.Warn(ctx, "workflow cancelled", "layer_idx", layerIdx)
			o.markFailed(wfiID, req, o.failReasonOr(wfiID, reasonCancelled))
			return
		default:
		}

		// === Plan execution: drain active plan steps before forward iteration ===
		o.mu.Lock()
		var planStep callbackPlanStep
		hasPlanStep := false
		if rs := o.runs[wfiID]; rs != nil && rs.callbackPlanIdx < len(rs.callbackPlan.steps) {
			planStep = rs.callbackPlan.steps[rs.callbackPlanIdx]
			hasPlanStep = true
		}
		o.mu.Unlock()

		if hasPlanStep {
			stepIdx := layerIndexOf(planStep.layer, layerGroups)
			var stepPhases []service.SpawnerPhaseDef
			if stepIdx >= 0 {
				if planStep.wholeLayer {
					stepPhases = layerGroups[stepIdx].phases
				} else {
					agentSet := make(map[string]bool, len(planStep.agents))
					for _, a := range planStep.agents {
						agentSet[a] = true
					}
					for _, p := range layerGroups[stepIdx].phases {
						if agentSet[p.Agent] {
							stepPhases = append(stepPhases, p)
						}
					}
				}
			}

			logger.Info(ctx, "executing plan step", "layer", planStep.layer, "whole_layer", planStep.wholeLayer, "agents", len(stepPhases))
			stepResults := o.spawnPhases(ctx, wfiID, req, stepPhases, parentSession, baseCfg)

			passCount, failCount := 0, 0
			var stepCBErrs []*spawner.CallbackError
			for _, r := range stepResults {
				switch {
				case r.callbackErr != nil:
					if !planStep.wholeLayer {
						o.markFailed(wfiID, req, "callback within agent/chain plan step is not supported in v1")
						return
					}
					passCount++
					stepCBErrs = append(stepCBErrs, r.callbackErr)
				case r.err != nil:
					if ctx.Err() != nil {
						o.markFailed(wfiID, req, o.failReasonOr(wfiID, reasonCancelled))
						return
					}
					logger.Error(ctx, "plan step agent failed", "layer", planStep.layer, "agent", r.agent, "err", r.err)
					failCount++
				default:
					logger.Info(ctx, "plan step agent completed", "layer", planStep.layer, "agent", r.agent)
					passCount++
				}
			}

			if len(stepCBErrs) > 0 {
				// Whole-layer plan step triggered another callback: re-enter handleCallback
				if !o.handleCallback(ctx, wfiID, req, layerGroups, stepIdx, stepCBErrs, &callbackCount) {
					return
				}
				continue
			}

			denom := passCount + failCount
			if denom > 0 {
				policy, _ := service.ParseLayerPolicy(layerPolicies[planStep.layer])
				required := policy.Required(denom)
				if passCount < required {
					logger.Error(ctx, "plan step pass_policy not satisfied", "layer", planStep.layer,
						"policy", policy.String(), "passed", passCount, "total", denom, "required", required)
					o.markFailed(wfiID, req, fmt.Sprintf(
						"plan step layer %d: pass_policy %q not satisfied (%d/%d passed, %d required)",
						planStep.layer, policy.String(), passCount, denom, required))
					return
				}
			}

			// Advance plan index; finalize plan when all steps are done
			o.mu.Lock()
			rs := o.runs[wfiID]
			if rs != nil {
				rs.callbackPlanIdx++
				if rs.callbackPlanIdx >= len(rs.callbackPlan.steps) {
					resumeLayer := rs.callbackPlan.resumeLayer
					rs.callbackPlan = callbackPlan{}
					rs.callbackPlanIdx = 0
					o.mu.Unlock()
					o.clearCallbackMetadata(ctx, wfiID)
					newIdx := layerIndexOf(resumeLayer, layerGroups)
					if newIdx < 0 {
						newIdx = len(layerGroups) // resumeLayer past end → exit loop
					}
					layerIdx = newIdx
					if newIdx > 0 && o.maybePauseAfterLayer(ctx, wfiID, req, newIdx-1, layerGroups, layerPause, pool, projectRoot) {
						worktreeHandled = true
						return
					}
				} else {
					o.mu.Unlock()
				}
			} else {
				o.mu.Unlock()
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
// or when ChainDepth >= maxNextWorkflowOnSuccessDepth.
func (o *Orchestrator) maybeStartNextOnSuccess(ctx context.Context, req RunRequest, finalResult string) {
	if finalResult == "" {
		return
	}
	if ctx.Err() != nil {
		return
	}
	if req.ChainDepth >= maxNextWorkflowOnSuccessDepth {
		logger.Warn(ctx, "next_workflow_on_success depth cap reached, skipping",
			"workflow", req.WorkflowName, "depth", req.ChainDepth)
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
	nextDepth := req.ChainDepth + 1
	logger.Info(context.Background(), "next_workflow_on_success: spawning next workflow",
		"source", req.WorkflowName, "next", nextWorkflow, "depth", nextDepth)

	go func() {
		nextReq := RunRequest{
			ProjectID:    req.ProjectID,
			WorkflowName: nextWorkflow,
			ScopeType:    "project",
			Instructions: finalResult,
			ChainDepth:   nextDepth,
		}
		if _, err := o.Start(context.Background(), nextReq); err != nil {
			logger.Error(context.Background(), "next_workflow_on_success: auto-start failed",
				"workflow", nextWorkflow, "err", err)
		}
	}()
}

// handleCallback builds a callback plan from reqs, enforces the agent-spawn cap,
// resets sessions, persists findings, broadcasts, and stores the plan on runState.
// Returns false if the workflow was marked failed.
func (o *Orchestrator) handleCallback(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	layerGroups []layerGroup,
	originatorLayerIdx int,
	reqs []*spawner.CallbackError,
	callbackCount *int,
) bool {
	originatorLayer := layerGroups[originatorLayerIdx].layer

	// Validate all requests before touching any state
	for _, r := range reqs {
		if err := validateCallbackRequest(r, originatorLayer, layerGroups); err != nil {
			o.markFailed(wfiID, req, fmt.Sprintf("invalid callback from %s: %v", r.AgentType, err))
			return false
		}
	}

	// Decompose and merge into a single plan
	parts := make([]decomposedRequest, len(reqs))
	for i, r := range reqs {
		parts[i] = decomposeCallback(r, originatorLayer, layerGroups)
	}
	plan := mergeCallbackPlans(parts)

	// Enforce cumulative agent-spawn cap
	agentCount := cumulativeAgentCount(plan, layerGroups)
	if *callbackCount+agentCount > maxCallbacks {
		o.markFailed(wfiID, req, fmt.Sprintf(
			"max callbacks (%d) exceeded: cumulative=%d, limit=%d",
			maxCallbacks, *callbackCount+agentCount, maxCallbacks))
		return false
	}
	*callbackCount += agentCount

	logger.Info(ctx, "callback detected",
		"from_layer", originatorLayer,
		"resume_layer", plan.resumeLayer,
		"plan_size", len(plan.steps),
		"reset_scope_size", len(plan.resetScope))

	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(ctx, "failed to open DB for callback", "err", err)
		o.markFailed(wfiID, req, "db_open_failed")
		return false
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	asRepo := repo.NewAgentSessionRepo(database, o.clock)

	// Reset all sessions in the plan's scope (single call)
	asRepo.ResetAgentSessionsInWorkflow(wfiID, plan.resetScope)

	// Build serialisable summaries for findings and broadcast
	planData := make([]map[string]interface{}, len(plan.steps))
	for i, s := range plan.steps {
		sd := map[string]interface{}{"layer": s.layer, "whole_layer": s.wholeLayer}
		if !s.wholeLayer {
			sd["agents"] = s.agents
		}
		planData[i] = sd
	}
	reqData := make([]map[string]interface{}, len(reqs))
	for i, r := range reqs {
		reqData[i] = map[string]interface{}{"agent": r.AgentType, "mode": r.Mode}
	}

	// Legacy fields for backward-compat (first plan step)
	legacyToLayer := originatorLayer
	legacyInstr := ""
	if len(plan.steps) > 0 {
		legacyToLayer = plan.steps[0].layer
		legacyInstr = plan.steps[0].layerInstr
	}

	// Persist _callback finding
	cbFindingRepo := repo.NewFindingRepo(pool, o.clock)
	cbData := map[string]interface{}{
		"plan":         planData,
		"resume_layer": plan.resumeLayer,
		"requests":     reqData,
		"from_layer":   originatorLayer,
		"instructions": legacyInstr, // template variable ${CALLBACK_INSTRUCTIONS}
	}
	cbVal, _ := json.Marshal(cbData)
	cbFindingRepo.Upsert("workflow_instance", wfiID, "_callback", cbVal, //nolint:errcheck
		repo.Denorm{ProjectID: req.ProjectID, WorkflowInstanceID: wfiID},
		repo.Actor{Source: "orchestrator"})

	// Broadcast single enriched event
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationCallback, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id":  wfiID,
		"from_layer":   originatorLayer,
		"to_layer":     legacyToLayer,
		"instructions": legacyInstr,
		"plan":         planData,
		"resume_layer": plan.resumeLayer,
		"requests":     reqData,
	}))

	// Store plan on runState for the execution loop
	o.mu.Lock()
	if rs, ok := o.runs[wfiID]; ok {
		rs.callbackPlan = plan
		rs.callbackPlanIdx = 0
	}
	o.mu.Unlock()

	return true
}

// clearCallbackMetadata removes the _callback key from workflow instance findings
// after the callback target layer completes successfully.
func (o *Orchestrator) clearCallbackMetadata(ctx context.Context, wfiID string) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(ctx, "failed to open DB to clear callback metadata", "err", err)
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	findingRepo := repo.NewFindingRepo(pool, o.clock)
	findingRepo.DeleteKeys("workflow_instance", wfiID, []string{"_callback"}, //nolint:errcheck
		repo.Actor{Source: "orchestrator"})
}

// layerGroup holds phases that share the same layer number.
type layerGroup struct {
	layer  int
	phases []service.SpawnerPhaseDef
}

// groupPhasesByLayer groups phases by layer number, sorted ascending.
func groupPhasesByLayer(phases []service.SpawnerPhaseDef) []layerGroup {
	groups := make(map[int][]service.SpawnerPhaseDef)
	for _, p := range phases {
		groups[p.Layer] = append(groups[p.Layer], p)
	}

	var layers []int
	for l := range groups {
		layers = append(layers, l)
	}
	sort.Ints(layers)

	result := make([]layerGroup, len(layers))
	for i, l := range layers {
		result[i] = layerGroup{layer: l, phases: groups[l]}
	}
	return result
}
