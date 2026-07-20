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

// ResumeAfterPlanApproval is the ContinueWorkflow twin for plan-suspended
// instances: service.PlanService.Approve already materialized the plan, so
// this rebuilds the run's spawner config via EffectivePhases, flips the
// instance back to active, re-arms the subworkflow watcher, and relaunches
// runLoop at the first materialized layer. A no-op (no error) when the
// instance is already active — the plan boundary inside that active run's own
// runLoop will materialize inline instead.
func (o *Orchestrator) ResumeAfterPlanApproval(ctx context.Context, instanceID string) error {
	logger.Info(ctx, "resuming plan-suspended workflow", "instance_id", instanceID)

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
	if wi.Status == model.WorkflowInstanceActive {
		return nil
	}
	if !model.IsPlanSuspended(wi.Status) {
		return fmt.Errorf("workflow is not plan-suspended (current: %s)", wi.Status)
	}

	if err := o.waitForRunToSettle(ctx, wi.ID); err != nil {
		return err
	}

	projectRepo := repo.NewProjectRepo(database, o.clock)
	project, err := projectRepo.Get(wi.ProjectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}
	if !project.RootPath.Valid || project.RootPath.String == "" {
		return fmt.Errorf("project '%s' has no root_path configured", wi.ProjectID)
	}
	projectRoot := project.RootPath.String

	dbWorkflow, dbAgentDefs, defProjectID, err := o.resolveWorkflowDef(database, wi.ProjectID, wi.WorkflowID)
	if err != nil {
		return err
	}

	svcWorkflows, svcAgents := service.BuildSpawnerConfig([]*model.Workflow{dbWorkflow}, dbAgentDefs)
	svcWf := svcWorkflows[wi.WorkflowID]

	req := RunRequest{
		ProjectID:               wi.ProjectID,
		TicketID:                wi.TicketID,
		WorkflowName:            wi.WorkflowID,
		ScopeType:               wi.ScopeType,
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

	materializedPhases, materializedPolicies, err := service.LoadInstanceNodePhases(pool, o.clock, wi.ID)
	if err != nil {
		return fmt.Errorf("failed to load materialized plan nodes: %w", err)
	}
	if len(materializedPhases) == 0 {
		return fmt.Errorf("plan resume: instance %s has no materialized nodes", wi.ID)
	}

	layerGroups := groupPhasesByLayer(service.EffectivePhases(svcWf.Phases, materializedPhases))
	minLayer := materializedPhases[0].Layer
	for _, p := range materializedPhases {
		if p.Layer < minLayer {
			minLayer = p.Layer
		}
	}
	resumeIdx := layerIndexOf(minLayer, layerGroups)
	if resumeIdx < 0 {
		resumeIdx = 0
	}

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
	layerPolicies, err := layerPolicySvc.GetLayerPolicies(defProjectID, dbWorkflow.ID)
	if err != nil {
		return fmt.Errorf("failed to load layer policies: %w", err)
	}
	for layer, policy := range materializedPolicies {
		layerPolicies[layer] = policy
	}
	layerPause, err := layerPolicySvc.GetLayerPauseAfter(defProjectID, dbWorkflow.ID)
	if err != nil {
		return fmt.Errorf("failed to load layer pause flags: %w", err)
	}

	modelConfigs, err := o.loadModelConfigs(pool)
	if err != nil {
		return err
	}
	claudeSettingsJSON := ""
	if raw, _ := pool.GetProjectConfig(wi.ProjectID, "claude_safety_hook"); raw != "" {
		claudeSettingsJSON = spawner.BuildSafetySettingsJSON(raw)
	}
	pushAfterMerge := false
	if val, _ := pool.GetProjectConfig(wi.ProjectID, "push_after_merge"); val == "true" {
		pushAfterMerge = true
	}
	lowConsumptionMode, globalStallStartTimeout, globalStallRunningTimeout := readRunConsumptionSettings(pool)
	projectEnv := loadProjectEnv(ctx, pool, wi.ProjectID, o.clock)

	spawnWorkflows := convertToSpawnerWorkflows(svcWorkflows)
	spawnAgents := convertToSpawnerAgents(svcAgents)
	agentTags := buildAgentTags(svcAgents)

	parentSession := uuid.New().String()
	if wi.ParentSession.Valid {
		parentSession = wi.ParentSession.String
	}

	wfiRepo.UpdateStatus(wi.ID, model.WorkflowInstanceActive)
	findingRepo := repo.NewFindingRepo(pool, o.clock)
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
	o.rearmSubworkflowWatcher(wi)

	o.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowResumed, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id":  wi.ID,
		"resume_layer": minLayer,
	}))

	go o.runLoop(orchCtx, wi.ID, req, parentSession, projectRoot, spawnWorkflows, spawnAgents, svcWf,
		resumeIdx, wt, agentTags, nil, lowConsumptionMode,
		globalStallStartTimeout, globalStallRunningTimeout, modelConfigs, claudeSettingsJSON,
		pushAfterMerge, projectEnv, layerPolicies, layerPause)

	return nil
}
