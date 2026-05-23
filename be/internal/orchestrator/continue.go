package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// ContinueWorkflow resumes a paused (waiting) workflow instance from its resume layer.
// If instructions is non-empty it is appended to the user_instructions finding.
func (o *Orchestrator) ContinueWorkflow(ctx context.Context, projectID, ticketID, workflowName, instanceID, instructions string) error {
	logger.Info(ctx, "continuing paused workflow", "instance_id", instanceID, "workflow", workflowName)

	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()
	pool := db.WrapAsPool(database)

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	wi, err := wfiRepo.Get(instanceID)
	if err != nil {
		return fmt.Errorf("workflow instance not found: %w", err)
	}
	if wi.Status != model.WorkflowInstanceWaiting {
		return fmt.Errorf("workflow is not in waiting status (current: %s)", wi.Status)
	}

	o.mu.Lock()
	if _, ok := o.runs[wi.ID]; ok {
		o.mu.Unlock()
		return fmt.Errorf("workflow is already running")
	}
	o.mu.Unlock()

	projectRepo := repo.NewProjectRepo(database, o.clock)
	project, err := projectRepo.Get(projectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}
	if !project.RootPath.Valid || project.RootPath.String == "" {
		return fmt.Errorf("project '%s' has no root_path configured", projectID)
	}
	projectRoot := project.RootPath.String

	wfRepo := repo.NewWorkflowRepo(database, o.clock)
	dbWorkflow, err := wfRepo.Get(projectID, workflowName)
	if err != nil {
		return fmt.Errorf("workflow definition '%s' not found: %w", workflowName, err)
	}
	adRepo := repo.NewAgentDefinitionRepo(database, o.clock)
	dbAgentDefs, err := adRepo.ListExecutable(projectID, dbWorkflow.ID)
	if err != nil {
		return fmt.Errorf("failed to load agent definitions: %w", err)
	}

	svcWorkflows, svcAgents := service.BuildSpawnerConfig([]*model.Workflow{dbWorkflow}, dbAgentDefs)
	svcWf := svcWorkflows[workflowName]

	req := RunRequest{
		ProjectID:               projectID,
		TicketID:                ticketID,
		WorkflowName:            workflowName,
		ScopeType:               wi.ScopeType,
		CloseTicketOnComplete:   svcWf.CloseTicketOnComplete,
		FinalizeSuccessCommand:  svcWf.FinalizeSuccessCommand,
		FinalizeSuccessScriptID: svcWf.FinalizeSuccessScriptID,
		FinalizeFailureCommand:  svcWf.FinalizeFailureCommand,
		FinalizeFailureScriptID: svcWf.FinalizeFailureScriptID,
		PauseEventCommand:       svcWf.PauseEventCommand,
		PauseEventScriptID:      svcWf.PauseEventScriptID,
	}

	layerGroups := groupPhasesByLayer(svcWf.Phases)

	// Read resume_layer from _pause finding
	findingRepo := repo.NewFindingRepo(pool, o.clock)
	pauseRaw, findErr := findingRepo.GetOwn("workflow_instance", wi.ID)
	if findErr != nil {
		return fmt.Errorf("failed to read findings: %w", findErr)
	}
	var resumeLayer int
	if pData, ok := pauseRaw["_pause"]; ok {
		var pf map[string]interface{}
		if json.Unmarshal(pData, &pf) == nil {
			switch v := pf["resume_layer"].(type) {
			case float64:
				resumeLayer = int(v)
			case json.Number:
				if n, e := v.Int64(); e == nil {
					resumeLayer = int(n)
				}
			case string:
				resumeLayer, _ = strconv.Atoi(v)
			}
		}
	}
	resumeIdx := layerIndexOf(resumeLayer, layerGroups)
	if resumeIdx < 0 {
		resumeIdx = 0
	}

	// Append instructions to user_instructions finding if provided
	if instructions != "" {
		var existingInstr string
		if iData, ok := pauseRaw["user_instructions"]; ok {
			json.Unmarshal(iData, &existingInstr) //nolint:errcheck
		}
		combined := existingInstr
		if combined != "" {
			combined += "\n---\n"
		}
		combined += instructions
		instrVal, _ := json.Marshal(combined)
		wfiDenorm := repo.Denorm{ProjectID: wi.ProjectID, WorkflowInstanceID: wi.ID}
		_ = findingRepo.Upsert("workflow_instance", wi.ID, "user_instructions", instrVal, wfiDenorm, repo.Actor{Source: "orchestrator"})
	}

	// Reconstruct worktreeInfo from persisted values (re-attach, do NOT create a new branch)
	var wt *worktreeInfo
	if wi.WorktreePath.Valid && wi.WorktreePath.String != "" && wi.BranchName.Valid && wi.BranchName.String != "" {
		defaultBranch := ""
		if project.DefaultBranch.Valid {
			defaultBranch = project.DefaultBranch.String
		}
		wt = &worktreeInfo{
			projectRoot:   projectRoot,
			worktreePath:  wi.WorktreePath.String,
			branchName:    wi.BranchName.String,
			defaultBranch: defaultBranch,
		}
		projectRoot = wt.worktreePath
	}

	layerPolicySvc := service.NewWorkflowLayerPolicyService(pool, o.clock)
	layerPolicies, err := layerPolicySvc.GetLayerPolicies(projectID, dbWorkflow.ID)
	if err != nil {
		return fmt.Errorf("failed to load layer policies: %w", err)
	}
	layerPause, err := layerPolicySvc.GetLayerPauseAfter(projectID, dbWorkflow.ID)
	if err != nil {
		return fmt.Errorf("failed to load layer pause flags: %w", err)
	}

	modelConfigs, err := o.loadModelConfigs(pool)
	if err != nil {
		return err
	}

	apiModelConfigs, err := o.loadAPIModelConfigs(pool)
	if err != nil {
		return err
	}

	claudeSettingsJSON := ""
	if raw, _ := pool.GetProjectConfig(projectID, "claude_safety_hook"); raw != "" {
		claudeSettingsJSON = spawner.BuildSafetySettingsJSON(raw)
	}
	pushAfterMerge := false
	if val, _ := pool.GetProjectConfig(projectID, "push_after_merge"); val == "true" {
		pushAfterMerge = true
	}
	lowConsumptionMode := false
	if val, _ := pool.GetConfig("low_consumption_mode"); val == "true" {
		lowConsumptionMode = true
	}
	contextSaveViaAgent := false
	if val, _ := pool.GetConfig("context_save_via_agent"); val == "true" {
		contextSaveViaAgent = true
	}
	var globalStallStartTimeout, globalStallRunningTimeout *int
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
	projectEnv := loadProjectEnv(ctx, pool, projectID, o.clock)

	spawnWorkflows := convertToSpawnerWorkflows(svcWorkflows)
	spawnAgents := convertToSpawnerAgents(svcAgents)
	agentTags := buildAgentTags(svcAgents)

	parentSession := uuid.New().String()
	if wi.ParentSession.Valid {
		parentSession = wi.ParentSession.String
	}

	wfiRepo.UpdateStatus(wi.ID, model.WorkflowInstanceActive)
	orchVal, _ := json.Marshal(map[string]interface{}{"status": "running"})
	_ = findingRepo.Upsert("workflow_instance", wi.ID, "_orchestration", orchVal,
		repo.Denorm{ProjectID: wi.ProjectID, WorkflowInstanceID: wi.ID},
		repo.Actor{Source: "orchestrator"})

	orchCtx, cancel := context.WithCancel(logger.WithTrx(context.Background(), logger.TrxFromContext(ctx)))
	rs := &runState{
		cancel:   cancel,
		spawners: make(map[string]*spawner.Spawner),
		done:     make(chan struct{}),
	}
	o.mu.Lock()
	o.runs[wi.ID] = rs
	o.mu.Unlock()

	o.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowResumed, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id":  wi.ID,
		"resume_layer": resumeLayer,
	}))

	go o.runLoop(orchCtx, wi.ID, req, parentSession, projectRoot, spawnWorkflows, spawnAgents, svcWf,
		resumeIdx, wt, agentTags, nil, lowConsumptionMode, contextSaveViaAgent,
		globalStallStartTimeout, globalStallRunningTimeout, modelConfigs, apiModelConfigs, claudeSettingsJSON,
		pushAfterMerge, projectEnv, layerPolicies, layerPause)

	return nil
}
