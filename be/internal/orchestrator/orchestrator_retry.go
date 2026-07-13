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
	"be/internal/ws"
)

func (o *Orchestrator) retryFailed(ctx context.Context, projectID, ticketID, workflowName, sessionID, scopeType, instanceID string) error {
	logger.Info(ctx, "retrying failed workflow", "workflow", workflowName, "session_id", sessionID, "scope", scopeType)
	// Look up the workflow instance
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)

	var wi *model.WorkflowInstance
	if instanceID != "" {
		wi, err = wfiRepo.Get(instanceID)
	} else {
		// Look up instance from session's workflow_instance_id
		asRepo := repo.NewAgentSessionRepo(database, o.clock)
		session, sessErr := asRepo.Get(sessionID)
		if sessErr != nil {
			return fmt.Errorf("agent session not found: %w", sessErr)
		}
		wi, err = wfiRepo.Get(session.WorkflowInstanceID)
	}
	if err != nil {
		return fmt.Errorf("workflow not found: %w", err)
	}

	if wi.Status != model.WorkflowInstanceFailed {
		return fmt.Errorf("workflow is not in failed status (current: %s)", wi.Status)
	}

	// A just-failed instance can still hold its o.runs slot: the runLoop goroutine
	// publishes the failed status (DB + WS) before its deferred cleanup releases the
	// slot, and that cleanup trails behind an up-to-hookTimeout finalize hook. Wait for
	// the teardown to finish so a retry racing the failure broadcast doesn't see the
	// instance as "already running".
	if err := o.waitForRunToSettle(ctx, wi.ID); err != nil {
		return err
	}

	// Look up the failed session to get its phase
	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return fmt.Errorf("agent session not found: %w", err)
	}
	if session.WorkflowInstanceID != wi.ID {
		return fmt.Errorf("session does not belong to this workflow instance")
	}
	failedPhase := session.Phase
	failedNode := session.NodeID

	// Load project root
	projectRepo := repo.NewProjectRepo(database, o.clock)
	project, err := projectRepo.Get(projectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}
	if !project.RootPath.Valid || project.RootPath.String == "" {
		return fmt.Errorf("project '%s' has no root_path configured", projectID)
	}
	projectRoot := project.RootPath.String

	// Setup git worktree if enabled
	branchName := ticketID
	wt, projectRoot, wtErr := setupWorktree(project, projectRoot, branchName, scopeType)
	if wtErr != nil {
		return wtErr
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

	// Load workflow/agent definitions (with global fallback)
	dbWorkflow, dbAgentDefs, defProjectID, err := o.resolveWorkflowDef(database, projectID, workflowName)
	if err != nil {
		return err
	}

	svcWorkflows, svcAgents := service.BuildSpawnerConfig([]*model.Workflow{dbWorkflow}, dbAgentDefs)
	svcWf := svcWorkflows[workflowName]

	// Determine which layer the failed phase belongs to
	layerGroups := groupPhasesByLayer(svcWf.Phases)
	startLayerIdx := -1
	for i, lg := range layerGroups {
		for _, p := range lg.phases {
			if p.NodeID == failedNode {
				startLayerIdx = i
				break
			}
		}
		if startLayerIdx >= 0 {
			break
		}
	}
	if startLayerIdx < 0 {
		return fmt.Errorf("failed phase '%s' not found in workflow definition", failedPhase)
	}

	// Reset workflow instance status to active
	wfiRepo.UpdateStatus(wi.ID, model.WorkflowInstanceActive)

	// Increment retry count
	wfiRepo.UpdateRetryCount(wi.ID, wi.RetryCount+1)

	// Update orchestration status in findings
	retryFindingRepo := repo.NewFindingRepo(pool, o.clock)
	retryOrchVal, _ := json.Marshal(map[string]interface{}{"status": "running"})
	retryFindingRepo.Upsert("workflow_instance", wi.ID, "_orchestration", retryOrchVal, //nolint:errcheck
		repo.Denorm{ProjectID: wi.ProjectID, WorkflowInstanceID: wi.ID},
		repo.Actor{Source: "orchestrator"})

	// Persist worktree info if available
	if wt != nil {
		wfiRepo.UpdateWorktree(wi.ID, wt.worktreePath, wt.branchName)
	}

	// Build spawner config
	spawnWorkflows := convertToSpawnerWorkflows(svcWorkflows)
	spawnAgents := convertToSpawnerAgents(svcAgents)

	// Build agent tag lookup map for layer-skip logic
	agentTags := buildAgentTags(svcAgents)

	// Read consumption-mode and stall-timeout settings (once at workflow retry)
	lowConsumptionMode, contextSaveViaAgent, globalStallStartTimeout, globalStallRunningTimeout := readRunConsumptionSettings(pool)

	// Load CLI model configs from DB (once at workflow retry)
	modelConfigs, err := o.loadModelConfigs(pool)
	if err != nil {
		return err
	}

	// Load API model configs from DB (once at workflow retry)
	apiModelConfigs, err := o.loadAPIModelConfigs(pool)
	if err != nil {
		return err
	}

	// Read claude safety hook config (once at workflow retry)
	claudeSettingsJSON := ""
	if raw, _ := pool.GetProjectConfig(projectID, "claude_safety_hook"); raw != "" {
		claudeSettingsJSON = spawner.BuildSafetySettingsJSON(raw)
	}

	// Read push after merge setting (once at workflow retry)
	pushAfterMerge := false
	if val, _ := pool.GetProjectConfig(projectID, "push_after_merge"); val == "true" {
		pushAfterMerge = true
	}

	// Load per-project env vars (once at workflow retry; injected into all spawned agents)
	projectEnv := loadProjectEnv(ctx, pool, projectID, o.clock)

	// Load per-layer pass policies for this workflow
	layerPolicySvc := service.NewWorkflowLayerPolicyService(pool, o.clock)
	layerPolicies, err := layerPolicySvc.GetLayerPolicies(defProjectID, dbWorkflow.ID)
	if err != nil {
		return fmt.Errorf("failed to load layer policies: %w", err)
	}
	layerPause, err := layerPolicySvc.GetLayerPauseAfter(defProjectID, dbWorkflow.ID)
	if err != nil {
		return fmt.Errorf("failed to load layer pause flags: %w", err)
	}

	parentSession := uuid.New().String()

	// Build run request
	req := RunRequest{
		ProjectID:               projectID,
		TicketID:                ticketID,
		WorkflowName:            workflowName,
		ScopeType:               scopeType,
		LaunchDepth:             wi.LaunchDepth,
		ParentInstanceID:        wi.ParentInstanceID,
		SubworkflowDepth:        wi.SubworkflowDepth,
		CloseTicketOnComplete:   svcWf.CloseTicketOnComplete,
		FinalizeSuccessCommand:  svcWf.FinalizeSuccessCommand,
		FinalizeSuccessScriptID: svcWf.FinalizeSuccessScriptID,
		FinalizeFailureCommand:  svcWf.FinalizeFailureCommand,
		FinalizeFailureScriptID: svcWf.FinalizeFailureScriptID,
		PauseEventCommand:       svcWf.PauseEventCommand,
		PauseEventScriptID:      svcWf.PauseEventScriptID,
	}

	// Create orchestration context detached from HTTP request context
	orchCtx, cancel := context.WithCancel(logger.WithTrx(context.Background(), logger.TrxFromContext(ctx)))
	rs := &runState{cancel: cancel, spawners: make(map[string]*spawner.Spawner), done: make(chan struct{})}
	o.mu.Lock()
	o.runs[wi.ID] = rs
	o.mu.Unlock()
	o.rearmSubworkflowWatcher(wi)

	// Broadcast retry event
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationRetried, projectID, ticketID, workflowName, map[string]interface{}{
		"instance_id":       wi.ID,
		"start_layer":       startLayerIdx,
		"failed_phase":      failedPhase,
		"failed_node_id":    failedNode,
		"failed_session_id": sessionID,
	}))

	launched = true
	go o.runLoop(orchCtx, wi.ID, req, parentSession, projectRoot, spawnWorkflows, spawnAgents, svcWf, startLayerIdx, wt, agentTags, nil, lowConsumptionMode, contextSaveViaAgent, globalStallStartTimeout, globalStallRunningTimeout, modelConfigs, apiModelConfigs, claudeSettingsJSON, pushAfterMerge, projectEnv, layerPolicies, layerPause)

	return nil
}
