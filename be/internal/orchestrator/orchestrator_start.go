package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/types"
	"be/internal/ws"
)

// Start begins an orchestrated workflow run. It initializes the workflow
// (or reuses existing), then runs all phases sequentially in a goroutine.
func (o *Orchestrator) Start(ctx context.Context, req RunRequest) (*RunResult, error) {
	// Validate
	if req.ProjectID == "" || req.WorkflowName == "" {
		return nil, fmt.Errorf("project_id and workflow are required")
	}
	if !req.IsProjectScope() && req.TicketID == "" {
		return nil, fmt.Errorf("ticket_id is required for ticket-scoped workflows")
	}

	// Check if already running (ticket scope only — project scope allows multiple instances)
	if !req.IsProjectScope() {
		if o.IsRunning(req.ProjectID, req.TicketID, req.WorkflowName) {
			return nil, fmt.Errorf("workflow '%s' is already running on %s", req.WorkflowName, req.TicketID)
		}
	}

	// Load project from DB to get root_path
	database, err := db.Open(o.dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	projectRepo := repo.NewProjectRepo(database, o.clock)
	project, err := projectRepo.Get(req.ProjectID)
	database.Close()
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}
	if !project.RootPath.Valid || project.RootPath.String == "" {
		return nil, fmt.Errorf("project '%s' has no root_path configured", req.ProjectID)
	}
	projectRoot := project.RootPath.String

	// Guard: prevent concurrent ticket workflows when worktrees are disabled
	if !req.IsProjectScope() && !project.UseGitWorktrees && !req.Force {
		if o.HasRunningTicketWorkflows(req.ProjectID) {
			return nil, fmt.Errorf("concurrent ticket workflows without worktrees: use force to override")
		}
	}

	// Setup git worktree if enabled
	branchName := req.TicketID
	wt, projectRoot, err := setupWorktree(project, projectRoot, branchName, req.ScopeType)
	if err != nil {
		return nil, err
	}

	// Clean up worktree if we fail before launching runLoop
	launched := false
	if wt != nil {
		defer func() {
			if !launched {
				wtService := &service.WorktreeService{}
				wtService.Cleanup(wt.projectRoot, wt.branchName, wt.worktreePath)
			}
		}()
	}

	// Load DB workflow definitions and agent definitions
	database, err = db.Open(o.dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	dbWorkflow, dbAgentDefs, defProjectID, err := o.resolveWorkflowDef(database, req.ProjectID, req.WorkflowName)
	if err != nil {
		database.Close()
		return nil, err
	}
	// A plan-driven def is legitimately phase-less: reloadPlanLayers drafts/materializes it at the boundary.
	planDriven, err := service.IsPlanDriven(db.WrapAsPool(database), defProjectID, dbWorkflow.ID)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to check plan-driven: %w", err)
	}
	database.Close()

	// Convert to spawner types
	svcWorkflows, svcAgents := service.BuildSpawnerConfig([]*model.Workflow{dbWorkflow}, dbAgentDefs)

	// Find the requested workflow
	svcWf := svcWorkflows[req.WorkflowName]
	if len(svcWf.Phases) == 0 && !planDriven {
		return nil, fmt.Errorf("workflow '%s' has no phases", req.WorkflowName)
	}
	req.CloseTicketOnComplete = svcWf.CloseTicketOnComplete
	req.FinalizeSuccessCommand = svcWf.FinalizeSuccessCommand
	req.FinalizeSuccessScriptID = svcWf.FinalizeSuccessScriptID
	req.FinalizeFailureCommand = svcWf.FinalizeFailureCommand
	req.FinalizeFailureScriptID = svcWf.FinalizeFailureScriptID
	req.PauseEventCommand = svcWf.PauseEventCommand
	req.PauseEventScriptID = svcWf.PauseEventScriptID

	// Init workflow instance — always creates a fresh instance
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create db pool: %w", err)
	}
	wfService := service.NewWorkflowService(pool, o.clock)

	var wi *model.WorkflowInstance
	if req.IsProjectScope() {
		wi, err = wfService.InitProjectWorkflow(req.ProjectID, &types.ProjectWorkflowRunRequest{
			Workflow:         req.WorkflowName,
			Instructions:     req.Instructions,
			EndlessLoop:      req.EndlessLoop,
			PlanAutoApprove:  req.PlanAutoApprove,
			ScheduledTaskID:  req.ScheduledTaskID,
			ExternalID:       req.ExternalID,
			ExternalContext:  req.ExternalContext,
			SeedFindings:     req.SeedFindings,
			LaunchDepth:      req.LaunchDepth,
			ParentInstanceID: req.ParentInstanceID,
			SubworkflowDepth: req.SubworkflowDepth,
		})
	} else {
		wi, err = wfService.Init(req.ProjectID, req.TicketID, &types.WorkflowInitRequest{
			Workflow:        req.WorkflowName,
			ScheduledTaskID: req.ScheduledTaskID,
			ExternalID:      req.ExternalID,
			ExternalContext: req.ExternalContext,
			SeedFindings:    req.SeedFindings,
		})
	}
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to init workflow: %w", err)
	}

	// Store user instructions and orchestration status in findings
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	findingRepo := repo.NewFindingRepo(pool, o.clock)
	wfiDenorm := repo.Denorm{ProjectID: wi.ProjectID, WorkflowInstanceID: wi.ID}
	orchActor := repo.Actor{Source: "orchestrator"}
	if req.Instructions != "" {
		instrVal, _ := json.Marshal(req.Instructions)
		findingRepo.Upsert("workflow_instance", wi.ID, "user_instructions", instrVal, wfiDenorm, orchActor) //nolint:errcheck
	}
	orchVal, _ := json.Marshal(map[string]interface{}{"status": "running"})
	findingRepo.Upsert("workflow_instance", wi.ID, "_orchestration", orchVal, wfiDenorm, orchActor) //nolint:errcheck

	// Persist worktree info if available
	if wt != nil {
		wfiRepo.UpdateWorktree(wi.ID, wt.worktreePath, wt.branchName)
	}

	// Attach input artifacts if provided; roll back the wfi on failure
	if len(req.InputArtifacts) > 0 {
		artifactSvc := service.NewArtifactService(pool, o.clock, o.wsHub, o.dataPath)
		if err := artifactSvc.AttachInputArtifacts(ctx, req.ProjectID, wi.ID, req.InputArtifacts); err != nil {
			_ = wfiRepo.Delete(wi.ID)
			pool.Close()
			return nil, fmt.Errorf("input artifacts attach failed: %w", err)
		}
	}

	// Read consumption-mode and stall-timeout settings (once at workflow start)
	lowConsumptionMode, contextSaveViaAgent, globalStallStartTimeout, globalStallRunningTimeout := readRunConsumptionSettings(pool)

	// Load CLI model configs from DB (once at workflow start)
	modelConfigs, err := o.loadModelConfigs(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}

	// Load API model configs from DB (once at workflow start)
	apiModelConfigs, err := o.loadAPIModelConfigs(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}

	// Read claude safety hook config (once at workflow start)
	claudeSettingsJSON := ""
	if raw, _ := pool.GetProjectConfig(req.ProjectID, "claude_safety_hook"); raw != "" {
		claudeSettingsJSON = spawner.BuildSafetySettingsJSON(raw)
	}

	// Read push after merge setting (once at workflow start)
	pushAfterMerge := false
	if val, _ := pool.GetProjectConfig(req.ProjectID, "push_after_merge"); val == "true" {
		pushAfterMerge = true
	}

	// Load per-project env vars (once at workflow start; injected into all spawned agents)
	projectEnv := loadProjectEnv(ctx, pool, req.ProjectID, o.clock)

	// Load per-layer pass policies for this workflow
	layerPolicySvc := service.NewWorkflowLayerPolicyService(pool, o.clock)
	layerPolicies, err := layerPolicySvc.GetLayerPolicies(defProjectID, dbWorkflow.ID)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to load layer policies: %w", err)
	}
	layerPause, err := layerPolicySvc.GetLayerPauseAfter(defProjectID, dbWorkflow.ID)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to load layer pause flags: %w", err)
	}

	// Set parent session
	parentSession := uuid.New().String()
	pool.Close()

	// Generate trx for this orchestration run
	trx := logger.NewTrx()
	ctx = logger.WithTrx(ctx, trx)

	logger.Info(ctx, "workflow instance created", "instance_id", wi.ID, "workflow", req.WorkflowName, "scope", req.ScopeType)

	// Set ticket to in_progress if currently open (best-effort, ticket scope only)
	if !req.IsProjectScope() {
		if statusPool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig()); err == nil {
			ticketService := service.NewTicketService(statusPool, o.clock)
			if err := ticketService.SetInProgress(req.ProjectID, req.TicketID); err != nil {
				logger.Warn(ctx, "failed to set ticket in_progress", "ticket", req.TicketID, "err", err)
			} else {
				o.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, req.ProjectID, req.TicketID, "", map[string]interface{}{"status": "in_progress"}))
			}
			statusPool.Close()
		}
	}

	// Build spawner config maps
	spawnWorkflows := convertToSpawnerWorkflows(svcWorkflows)
	spawnAgents := convertToSpawnerAgents(svcAgents)

	// Build agent tag lookup map for layer-skip logic
	agentTags := buildAgentTags(svcAgents)

	// Create orchestration context detached from HTTP request context.
	// Using ctx (r.Context()) as parent would cancel the orchestration when
	// the HTTP handler returns. Propagate the trx for log correlation.
	orchCtx, cancel := context.WithCancel(logger.WithTrx(context.Background(), logger.TrxFromContext(ctx)))

	rs := &runState{cancel: cancel, spawners: make(map[string]*spawner.Spawner), done: make(chan struct{})}
	o.mu.Lock()
	o.runs[wi.ID] = rs
	o.mu.Unlock()

	// Broadcast orchestration started
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationStarted, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wi.ID,
	}))

	// Setup interactive/plan pre-step if requested
	var pre *interactivePreStep
	if req.Interactive || req.PlanMode {
		pre, err = o.setupInteractivePreStep(req, wi, svcWf, svcAgents, spawnWorkflows, spawnAgents, projectRoot, modelConfigs, apiModelConfigs, claudeSettingsJSON)
		if err != nil {
			cancel()
			o.mu.Lock()
			delete(o.runs, wi.ID)
			o.mu.Unlock()
			return nil, fmt.Errorf("failed to setup interactive pre-step: %w", err)
		}
	}

	// Run orchestration loop in goroutine
	launched = true
	go o.runLoop(orchCtx, wi.ID, req, parentSession, projectRoot, spawnWorkflows, spawnAgents, svcWf, 0, wt, agentTags, pre, lowConsumptionMode, contextSaveViaAgent, globalStallStartTimeout, globalStallRunningTimeout, modelConfigs, apiModelConfigs, claudeSettingsJSON, pushAfterMerge, projectEnv, layerPolicies, layerPause)

	status := "started"
	sessionID := ""
	if pre != nil {
		sessionID = pre.sessionID
		if req.PlanMode {
			status = "planning"
		} else {
			status = "interactive"
		}
	}

	return &RunResult{
		InstanceID: wi.ID,
		SessionID:  sessionID,
		Status:     status,
	}, nil
}
