// Package orchestrator provides server-side workflow orchestration.
// It runs entire workflows automatically by sequencing spawner calls
// for each phase, driven from the HTTP API (web UI "Run Workflow" button).
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/manifest/python"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/types"
	"be/internal/venv"
	"be/internal/ws"
)

const maxNextWorkflowOnSuccessDepth = 10

// RunRequest contains parameters for starting an orchestrated workflow run.
type RunRequest struct {
	ProjectID             string            `json:"project_id"`
	TicketID              string            `json:"ticket_id"`
	WorkflowName          string            `json:"workflow"`
	Instructions          string            `json:"instructions"` // User-provided instructions
	ScopeType             string            `json:"scope_type"`   // "ticket" (default) or "project"
	Interactive           bool              `json:"interactive"`  // If true, start with interactive PTY session before layer execution
	PlanMode              bool              `json:"plan_mode"`    // If true, start with planning PTY session, read plan file after
	CloseTicketOnComplete bool              `json:"close_ticket_on_complete"`
	Force                 bool              `json:"force"`                    // If true, bypass concurrent ticket workflow guard
	EndlessLoop           bool              `json:"endless_loop"`             // Project-scope only: auto re-run on successful completion until stopped or failed
	ScheduledTaskID       string            `json:"scheduled_task_id,omitempty"` // Set by scheduler; empty for UI/API-triggered runs
	SeedFindings          map[string]string `json:"seed_findings,omitempty"`  // Pre-populate workflow_instances.findings at create time
	ChainDepth            int               `json:"-"`                        // next_workflow_on_success recursion depth (not persisted)
}

// IsProjectScope returns true if this is a project-scoped run request
func (r RunRequest) IsProjectScope() bool {
	return r.ScopeType == "project"
}

// RunResult contains the result of starting an orchestrated workflow.
type RunResult struct {
	InstanceID string `json:"instance_id"`
	SessionID  string `json:"session_id,omitempty"`
	Status     string `json:"status"`
}

// worktreeInfo stores git worktree metadata for a workflow run.
type worktreeInfo struct {
	projectRoot   string // original project root (for merge commands)
	worktreePath  string
	branchName    string
	defaultBranch string
}

// runState tracks a running orchestration's cancel func and active spawners.
// spawners is a sessionID→*Spawner index maintained via spawner-side
// OnSessionRegister/OnSessionUnregister callbacks.
type runState struct {
	cancel   context.CancelFunc
	spawners map[string]*spawner.Spawner
	done     chan struct{} // closed when runLoop goroutine exits
}

// Orchestrator manages server-side workflow runs.
type Orchestrator struct {
	mu       sync.Mutex
	runs     map[string]*runState // wfi_id → state
	dataPath string
	sdkDir   string
	venvMgr  *venv.Manager
	wsHub    *ws.Hub
	clock    clock.Clock
	errorSvc spawner.ErrorRecorder
	apiMode  bool

	// OnRegisterPtyCommand is called when interactive/plan mode needs to register
	// a PTY command for a session. The API server wires this to ptyManager.RegisterCommand.
	OnRegisterPtyCommand func(sessionID string, cmd string, args []string)

	// OnClosePtySession is called when an interactive session is killed to close and
	// remove its PTY session. The API server wires this to ptyManager.Get+Close+Remove.
	OnClosePtySession func(sessionID string)

	// PTYManager is the shared PTY session manager. Passed to spawner.Config.PTYManager
	// so the interactive CLI backend can create and manage PTY sessions directly.
	PTYManager *ptyPkg.Manager
}

// New creates a new Orchestrator.
func New(dataPath string, wsHub *ws.Hub, clk clock.Clock, errorSvc spawner.ErrorRecorder, apiMode bool, sdkDir string) *Orchestrator {
	return &Orchestrator{
		runs:     make(map[string]*runState),
		dataPath: dataPath,
		sdkDir:   sdkDir,
		venvMgr:  venv.New(filepath.Dir(dataPath), clk),
		wsHub:    wsHub,
		clock:    clk,
		errorSvc: errorSvc,
		apiMode:  apiMode,
	}
}

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
	wfRepo := repo.NewWorkflowRepo(database, o.clock)
	dbWorkflow, err := wfRepo.Get(req.ProjectID, req.WorkflowName)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("workflow definition '%s' not found: %w", req.WorkflowName, err)
	}
	adRepo := repo.NewAgentDefinitionRepo(database, o.clock)
	dbAgentDefs, err := adRepo.List(req.ProjectID, dbWorkflow.ID)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to load agent definitions: %w", err)
	}
	database.Close()

	// Convert to spawner types
	svcWorkflows, svcAgents := service.BuildSpawnerConfig([]*model.Workflow{dbWorkflow}, dbAgentDefs)

	// Find the requested workflow
	svcWf := svcWorkflows[req.WorkflowName]
	if len(svcWf.Phases) == 0 {
		return nil, fmt.Errorf("workflow '%s' has no phases", req.WorkflowName)
	}
	req.CloseTicketOnComplete = svcWf.CloseTicketOnComplete

	// Init workflow instance — always creates a fresh instance
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create db pool: %w", err)
	}
	wfService := service.NewWorkflowService(pool, o.clock)

	var wi *model.WorkflowInstance
	if req.IsProjectScope() {
		wi, err = wfService.InitProjectWorkflow(req.ProjectID, &types.ProjectWorkflowRunRequest{
			Workflow:        req.WorkflowName,
			Instructions:    req.Instructions,
			EndlessLoop:     req.EndlessLoop,
			ScheduledTaskID: req.ScheduledTaskID,
			SeedFindings:    req.SeedFindings,
		})
	} else {
		wi, err = wfService.Init(req.ProjectID, req.TicketID, &types.WorkflowInitRequest{
			Workflow:        req.WorkflowName,
			ScheduledTaskID: req.ScheduledTaskID,
			SeedFindings:    req.SeedFindings,
		})
	}
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to init workflow: %w", err)
	}

	// Store user instructions and orchestration status in findings
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	findings := wi.GetFindings()
	if req.Instructions != "" {
		findings["user_instructions"] = req.Instructions
	}
	findings["_orchestration"] = map[string]interface{}{
		"status": "running",
	}
	findingsJSON, _ := json.Marshal(findings)
	wfiRepo.UpdateFindings(wi.ID, string(findingsJSON))

	// Persist worktree info if available
	if wt != nil {
		wfiRepo.UpdateWorktree(wi.ID, wt.worktreePath, wt.branchName)
	}

	// Read low consumption mode setting (once at workflow start)
	lowConsumptionMode := false
	if val, _ := pool.GetConfig("low_consumption_mode"); val == "true" {
		lowConsumptionMode = true
	}

	// Read context save via agent setting (once at workflow start)
	contextSaveViaAgent := false
	if val, _ := pool.GetConfig("context_save_via_agent"); val == "true" {
		contextSaveViaAgent = true
	}

	// Read global stall timeout settings (once at workflow start)
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

	// Load CLI model configs from DB (once at workflow start)
	modelConfigs, err := o.loadModelConfigs(pool)
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

	// Read customer config dir (once at workflow start; used by api-mode manifest tools)
	customerConfigDir, _ := pool.GetProjectConfig(req.ProjectID, "customer_config_dir")

	// Load per-project env vars (once at workflow start; injected into all spawned agents)
	projectEnv := loadProjectEnv(ctx, pool, req.ProjectID, o.clock)

	// Load per-layer pass policies for this workflow
	layerPolicySvc := service.NewWorkflowLayerPolicyService(pool, o.clock)
	layerPolicies, err := layerPolicySvc.GetLayerPolicies(req.ProjectID, dbWorkflow.ID)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to load layer policies: %w", err)
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
		pre, err = o.setupInteractivePreStep(req, wi, svcWf, svcAgents, spawnWorkflows, spawnAgents, projectRoot, modelConfigs, claudeSettingsJSON)
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
	go o.runLoop(orchCtx, wi.ID, req, parentSession, projectRoot, spawnWorkflows, spawnAgents, svcWf, 0, wt, agentTags, pre, lowConsumptionMode, contextSaveViaAgent, globalStallStartTimeout, globalStallRunningTimeout, modelConfigs, claudeSettingsJSON, pushAfterMerge, customerConfigDir, projectEnv, layerPolicies)

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
	return nil
}

// StopByTicket stops a running orchestration for a ticket.
// If instanceID is provided, stops that specific instance.
// If workflowName is provided, stops the first running instance for that workflow.
// Otherwise, stops the first running instance for the ticket.
func (o *Orchestrator) StopByTicket(projectID, ticketID, workflowName, instanceID string) error {
	if instanceID != "" {
		return o.Stop(instanceID)
	}

	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)

	// Stop first running orchestration for this ticket (optionally filtered by workflow)
	instances, err := wfiRepo.ListByTicket(projectID, ticketID)
	if err != nil {
		return err
	}

	for _, wi := range instances {
		if workflowName != "" && !strings.EqualFold(wi.WorkflowID, workflowName) {
			continue
		}
		o.mu.Lock()
		_, running := o.runs[wi.ID]
		o.mu.Unlock()
		if running {
			return o.Stop(wi.ID)
		}
		// Fallback: active instance with no in-memory orchestration (orphaned after restart).
		if wi.Status == model.WorkflowInstanceActive {
			return o.forceStopInstance(wi.ID)
		}
	}

	return fmt.Errorf("no running orchestration found for %s", ticketID)
}

// StopByInstance stops a workflow instance by ID, optionally validating project ownership.
func (o *Orchestrator) StopByInstance(projectID, instanceID string) error {
	return o.Stop(instanceID)
}

// StopByProject stops a running project-scoped orchestration.
// If instanceID is provided, stops that specific instance.
// Otherwise, stops all running instances for the given workflow (or all project workflows if workflowName is empty).
func (o *Orchestrator) StopByProject(projectID, workflowName, instanceID string) error {
	// If instance ID provided, stop directly
	if instanceID != "" {
		return o.Stop(instanceID)
	}

	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)

	instances, err := wfiRepo.ListByProjectScope(projectID)
	if err != nil {
		return err
	}

	stopped := 0
	for _, wi := range instances {
		if workflowName != "" && wi.WorkflowID != workflowName {
			continue
		}
		o.mu.Lock()
		_, running := o.runs[wi.ID]
		o.mu.Unlock()
		if running {
			if err := o.Stop(wi.ID); err == nil {
				stopped++
			}
		} else if wi.Status == model.WorkflowInstanceActive {
			if err := o.forceStopInstance(wi.ID); err == nil {
				stopped++
			}
		}
	}

	if stopped == 0 {
		return fmt.Errorf("no running project orchestration found")
	}
	return nil
}

// RetryFailedAgent resets a failed workflow instance and re-runs from the failed layer.
func (o *Orchestrator) RetryFailedAgent(ctx context.Context, projectID, ticketID, workflowName, sessionID string) error {
	return o.retryFailed(ctx, projectID, ticketID, workflowName, sessionID, "ticket", "")
}

// RetryFailedProjectAgent resets a failed project-scoped workflow and re-runs from the failed layer.
func (o *Orchestrator) RetryFailedProjectAgent(ctx context.Context, projectID, workflowName, sessionID, instanceID string) error {
	return o.retryFailed(ctx, projectID, "", workflowName, sessionID, "project", instanceID)
}

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

	// Check not already running
	o.mu.Lock()
	if _, ok := o.runs[wi.ID]; ok {
		o.mu.Unlock()
		return fmt.Errorf("workflow is already running")
	}
	o.mu.Unlock()

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

	// Load workflow/agent definitions
	wfRepo := repo.NewWorkflowRepo(database, o.clock)
	dbWorkflow, err := wfRepo.Get(projectID, workflowName)
	if err != nil {
		return fmt.Errorf("workflow definition '%s' not found: %w", workflowName, err)
	}
	adRepo := repo.NewAgentDefinitionRepo(database, o.clock)
	dbAgentDefs, err := adRepo.List(projectID, dbWorkflow.ID)
	if err != nil {
		return fmt.Errorf("failed to load agent definitions: %w", err)
	}

	svcWorkflows, svcAgents := service.BuildSpawnerConfig([]*model.Workflow{dbWorkflow}, dbAgentDefs)
	svcWf := svcWorkflows[workflowName]

	// Determine which layer the failed phase belongs to
	layerGroups := groupPhasesByLayer(svcWf.Phases)
	startLayerIdx := -1
	for i, lg := range layerGroups {
		for _, p := range lg.phases {
			if p.Agent == failedPhase {
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
	findings := wi.GetFindings()
	findings["_orchestration"] = map[string]interface{}{
		"status": "running",
	}
	findingsJSON, _ := json.Marshal(findings)
	wfiRepo.UpdateFindings(wi.ID, string(findingsJSON))

	// Persist worktree info if available
	if wt != nil {
		wfiRepo.UpdateWorktree(wi.ID, wt.worktreePath, wt.branchName)
	}

	// Build spawner config
	spawnWorkflows := convertToSpawnerWorkflows(svcWorkflows)
	spawnAgents := convertToSpawnerAgents(svcAgents)

	// Build agent tag lookup map for layer-skip logic
	agentTags := buildAgentTags(svcAgents)

	// Read low consumption mode setting (once at workflow retry)
	lowConsumptionMode := false
	if val, _ := pool.GetConfig("low_consumption_mode"); val == "true" {
		lowConsumptionMode = true
	}

	// Read context save via agent setting (once at workflow retry)
	contextSaveViaAgent := false
	if val, _ := pool.GetConfig("context_save_via_agent"); val == "true" {
		contextSaveViaAgent = true
	}

	// Read global stall timeout settings (once at workflow retry)
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

	// Load CLI model configs from DB (once at workflow retry)
	modelConfigs, err := o.loadModelConfigs(pool)
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

	// Read customer config dir (once at workflow retry)
	customerConfigDir, _ := pool.GetProjectConfig(projectID, "customer_config_dir")

	// Load per-project env vars (once at workflow retry; injected into all spawned agents)
	projectEnv := loadProjectEnv(ctx, pool, projectID, o.clock)

	// Load per-layer pass policies for this workflow
	layerPolicySvc := service.NewWorkflowLayerPolicyService(pool, o.clock)
	layerPolicies, err := layerPolicySvc.GetLayerPolicies(projectID, dbWorkflow.ID)
	if err != nil {
		return fmt.Errorf("failed to load layer policies: %w", err)
	}

	parentSession := uuid.New().String()

	// Build run request
	req := RunRequest{
		ProjectID:             projectID,
		TicketID:              ticketID,
		WorkflowName:          workflowName,
		ScopeType:             scopeType,
		CloseTicketOnComplete: svcWf.CloseTicketOnComplete,
	}

	// Create orchestration context detached from HTTP request context
	orchCtx, cancel := context.WithCancel(logger.WithTrx(context.Background(), logger.TrxFromContext(ctx)))
	rs := &runState{cancel: cancel, spawners: make(map[string]*spawner.Spawner), done: make(chan struct{})}
	o.mu.Lock()
	o.runs[wi.ID] = rs
	o.mu.Unlock()

	// Broadcast retry event
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationRetried, projectID, ticketID, workflowName, map[string]interface{}{
		"instance_id":      wi.ID,
		"start_layer":      startLayerIdx,
		"failed_phase":     failedPhase,
		"failed_session_id": sessionID,
	}))

	launched = true
	go o.runLoop(orchCtx, wi.ID, req, parentSession, projectRoot, spawnWorkflows, spawnAgents, svcWf, startLayerIdx, wt, agentTags, nil, lowConsumptionMode, contextSaveViaAgent, globalStallStartTimeout, globalStallRunningTimeout, modelConfigs, claudeSettingsJSON, pushAfterMerge, customerConfigDir, projectEnv, layerPolicies)

	return nil
}

// RestartAgent sends a manual restart signal to the active spawner for a workflow.
// Looks up the instance from the session's workflow_instance_id.
func (o *Orchestrator) RestartAgent(projectID, ticketID, workflowName, sessionID string) error {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return fmt.Errorf("agent session not found: %w", err)
	}

	return o.restartAgentByInstance(session.WorkflowInstanceID, workflowName, ticketID, sessionID)
}

// RestartProjectAgent sends a restart signal for a project-scoped workflow agent.
// instanceID is required to identify which instance to restart.
func (o *Orchestrator) RestartProjectAgent(projectID, workflowName, sessionID, instanceID string) error {
	if instanceID == "" {
		return fmt.Errorf("instance_id is required for project-scoped workflow restart")
	}
	return o.restartAgentByInstance(instanceID, workflowName, projectID, sessionID)
}

func (o *Orchestrator) restartAgentByInstance(wfiID, workflowName, target, sessionID string) error {
	logger.Info(context.Background(), "agent restart requested", "session_id", sessionID, "workflow", workflowName)
	o.mu.Lock()
	rs, ok := o.runs[wfiID]
	o.mu.Unlock()
	if !ok {
		return fmt.Errorf("no running orchestration for workflow '%s' on %s", workflowName, target)
	}

	o.mu.Lock()
	sp := rs.spawners[sessionID]
	o.mu.Unlock()
	if sp == nil {
		return fmt.Errorf("no active spawner (agent may be between phases)")
	}

	sp.RequestRestart(sessionID)
	return nil
}

// TakeControl sends a take-control signal to the active spawner for a ticket-scoped workflow.
// Looks up the instance from the session's workflow_instance_id.
func (o *Orchestrator) TakeControl(projectID, ticketID, workflowName, sessionID string) (string, error) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return "", fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return "", fmt.Errorf("agent session not found: %w", err)
	}

	return o.takeControlByInstance(session.WorkflowInstanceID, workflowName, ticketID, sessionID)
}

// TakeControlProject sends a take-control signal for a project-scoped workflow.
func (o *Orchestrator) TakeControlProject(projectID, workflowName, sessionID, instanceID string) (string, error) {
	if instanceID == "" {
		return "", fmt.Errorf("instance_id is required for project-scoped workflow take-control")
	}
	return o.takeControlByInstance(instanceID, workflowName, projectID, sessionID)
}

func (o *Orchestrator) takeControlByInstance(wfiID, workflowName, target, sessionID string) (string, error) {
	logger.Info(context.Background(), "take-control requested", "session_id", sessionID, "workflow", workflowName)
	o.mu.Lock()
	rs, ok := o.runs[wfiID]
	o.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("no running orchestration for workflow '%s' on %s", workflowName, target)
	}

	o.mu.Lock()
	sp := rs.spawners[sessionID]
	o.mu.Unlock()
	if sp == nil {
		return "", fmt.Errorf("no active spawner (agent may be between phases)")
	}

	sp.RequestTakeControl(sessionID)
	return sessionID, nil
}

// WaitTakeControlReady blocks until the spawner has finished the synchronous
// portion of a previously-requested take-control (kill + status flip to
// user_interactive, viewer-attach broadcast, or rejection), or until timeout.
// Returns true if the ready signal fired before timeout. Best-effort: returns
// false when the session/spawner can't be located.
func (o *Orchestrator) WaitTakeControlReady(sessionID string, timeout time.Duration) bool {
	o.mu.Lock()
	var sp *spawner.Spawner
	for _, rs := range o.runs {
		if rs == nil {
			continue
		}
		if found, ok := rs.spawners[sessionID]; ok {
			sp = found
			break
		}
	}
	o.mu.Unlock()
	if sp == nil {
		return false
	}
	return sp.WaitForTakeControlReady(sessionID, timeout)
}

// SignalSessionReady marks the matching running proc as TUI-ready, releasing
// its prompt-delivery wait. Best-effort: returns nil when session or run is
// not found. Idempotent on the spawner side.
func (o *Orchestrator) SignalSessionReady(sessionID string) error {
	o.mu.Lock()
	seen := make(map[*spawner.Spawner]struct{})
	for _, rs := range o.runs {
		if rs == nil {
			continue
		}
		for _, sp := range rs.spawners {
			if _, ok := seen[sp]; ok {
				continue
			}
			seen[sp] = struct{}{}
			sp.MarkSessionReady(sessionID)
		}
	}
	o.mu.Unlock()
	return nil
}

// BumpLastMessage resets stall-detection state for the matching running agent.
// Best-effort: returns nil when session or run not found.
func (o *Orchestrator) BumpLastMessage(projectID, ticketID, workflow, sessionID string) error {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return nil // session may have already ended
	}

	o.mu.Lock()
	rs, ok := o.runs[session.WorkflowInstanceID]
	o.mu.Unlock()
	if !ok {
		return nil // run finished; no-op
	}

	o.mu.Lock()
	sp := rs.spawners[sessionID]
	o.mu.Unlock()
	if sp == nil {
		return nil // between phases
	}

	sp.BumpLastMessage(sessionID)
	return nil
}

// SetLastMessage updates proc.lastMessage on the running agent so the
// "agent status" log line and any in-memory status surface show the most
// recent agent output. Best-effort: returns nil when session/run/spawner
// not found, or when content is empty.
func (o *Orchestrator) SetLastMessage(projectID, ticketID, workflow, sessionID, content string) error {
	if content == "" {
		return nil
	}
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return nil
	}

	o.mu.Lock()
	rs, ok := o.runs[session.WorkflowInstanceID]
	o.mu.Unlock()
	if !ok {
		return nil
	}

	o.mu.Lock()
	sp := rs.spawners[sessionID]
	o.mu.Unlock()
	if sp == nil {
		return nil
	}

	sp.SetLastMessage(sessionID, content)
	return nil
}

// RequestTerminalSignal kills the active agent for the given session so
// monitorAll exits and handleCompletion reads the DB result already written
// by the socket handler. Best-effort: returns nil when session or run not found.
func (o *Orchestrator) RequestTerminalSignal(projectID, ticketID, workflow, sessionID, result string) error {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return nil // session may have already ended
	}

	o.mu.Lock()
	rs, ok := o.runs[session.WorkflowInstanceID]
	o.mu.Unlock()
	if !ok {
		return nil // run finished; no-op
	}

	o.mu.Lock()
	sp := rs.spawners[sessionID]
	o.mu.Unlock()
	if sp == nil {
		return nil // between phases
	}

	sp.RequestTerminalSignal(sessionID, result)
	return nil
}

// CompleteInteractive signals that the interactive session has ended.
// It updates the agent session in DB and unblocks the spawner's wait.
func (o *Orchestrator) CompleteInteractive(sessionID string) error {
	logger.Info(context.Background(), "interactive session completing", "session_id", sessionID)

	// Update agent session in DB
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	if err := asRepo.UpdateStatusToInteractiveCompleted(sessionID); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	// Find the runState that has this session's spawner and call CompleteInteractive
	o.mu.Lock()
	seen := make(map[*spawner.Spawner]struct{})
	for _, rs := range o.runs {
		for _, sp := range rs.spawners {
			if _, ok := seen[sp]; ok {
				continue
			}
			seen[sp] = struct{}{}
			sp.CompleteInteractive(sessionID)
		}
	}
	o.mu.Unlock()

	return nil
}

// KillInteractive marks the interactive session as failed and unblocks the spawner wait.
func (o *Orchestrator) KillInteractive(sessionID string) error {
	logger.Info(context.Background(), "killing interactive session", "session_id", sessionID)

	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return fmt.Errorf("agent session not found: %s", sessionID)
	}
	if session.Status != model.AgentSessionUserInteractive {
		return fmt.Errorf("session %s is not user_interactive (status=%s)", sessionID, session.Status)
	}

	if err := asRepo.UpdateStatusToFailedWithReason(sessionID, "user_killed"); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	if o.OnClosePtySession != nil {
		o.OnClosePtySession(sessionID)
	}

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	workflowName := ""
	if wfi, err := wfiRepo.Get(session.WorkflowInstanceID); err == nil {
		workflowName = wfi.WorkflowID
	}

	o.mu.Lock()
	seen := make(map[*spawner.Spawner]struct{})
	for _, rs := range o.runs {
		for _, sp := range rs.spawners {
			if _, ok := seen[sp]; ok {
				continue
			}
			seen[sp] = struct{}{}
			sp.KillInteractive(sessionID)
		}
	}
	o.mu.Unlock()

	o.wsHub.Broadcast(ws.NewEvent(ws.EventAgentKilled, session.ProjectID, session.TicketID, workflowName, map[string]interface{}{
		"session_id": sessionID,
		"agent_type": session.AgentType,
		"model_id":   session.ModelID.String,
		"reason":     "user_killed",
	}))

	return nil
}

// IsRunning checks if any orchestration is running for a ticket+workflow.
func (o *Orchestrator) IsRunning(projectID, ticketID, workflowName string) bool {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return false
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	instances, err := wfiRepo.ListByTicket(projectID, ticketID)
	if err != nil {
		return false
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	for _, wi := range instances {
		if strings.EqualFold(wi.WorkflowID, workflowName) {
			if _, running := o.runs[wi.ID]; running {
				return true
			}
		}
	}
	return false
}

// HasRunningTicketWorkflows checks if any ticket-scoped workflow is currently
// running for the given project. Uses in-memory o.runs for accuracy.
func (o *Orchestrator) HasRunningTicketWorkflows(projectID string) bool {
	// Collect running instance IDs under lock
	o.mu.Lock()
	ids := make([]string, 0, len(o.runs))
	for id := range o.runs {
		ids = append(ids, id)
	}
	o.mu.Unlock()

	if len(ids) == 0 {
		return false
	}

	database, err := db.Open(o.dataPath)
	if err != nil {
		return false
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	for _, id := range ids {
		wi, err := wfiRepo.Get(id)
		if err != nil {
			continue
		}
		if wi.TicketID != "" && strings.EqualFold(wi.ProjectID, projectID) {
			return true
		}
	}
	return false
}

// IsInstanceRunning checks if a specific instance ID has an active orchestration.
func (o *Orchestrator) IsInstanceRunning(instanceID string) bool {
	o.mu.Lock()
	_, running := o.runs[instanceID]
	o.mu.Unlock()
	return running
}

// StopAll cancels all running orchestrations and waits for them to exit (for server shutdown).
func (o *Orchestrator) StopAll() {
	o.mu.Lock()
	logger.Warn(context.Background(), "stopping all orchestrations", "count", len(o.runs))
	doneChans := make([]chan struct{}, 0, len(o.runs))
	for _, rs := range o.runs {
		rs.cancel()
		if rs.done != nil {
			doneChans = append(doneChans, rs.done)
		}
	}
	o.mu.Unlock()

	deadline := time.After(10 * time.Second)
	for _, done := range doneChans {
		select {
		case <-done:
		case <-deadline:
			return
		}
	}
}

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
	claudeSettingsJSON string,
	pushAfterMerge bool,
	customerConfigDir string,
	projectEnv []string,
	layerPolicies map[int]string,
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

	// Build API-mode wiring once per workflow run. Provider is nil if no
	// anthropic key is configured — api-mode agents would then fail at
	// prepareSpawn with a clear error (CLI agents are unaffected).
	apiProvider := buildAPIProvider(ctx, pool, req.ProjectID, o.clock)
	apiAgentSvc := newAPIAgentSvc(pool, o.clock, o.wsHub)
	apiCredRepo := repo.NewAPICredentialRepo(pool, o.clock)
	findingsSvc := service.NewFindingsService(pool, o.clock)
	projectFindingsSvc := service.NewProjectFindingsService(pool, o.clock)
	agentSvcReal := service.NewAgentService(pool, o.clock)
	workflowSvcReal := service.NewWorkflowService(pool, o.clock)
	toolDefRepo := repo.NewToolDefinitionRepo(pool, o.clock)
	dispatchRepo := repo.NewDispatchRepo(pool, o.clock)
	reviewRepo := repo.NewReviewRepo(pool, o.clock)
	pythonRunner := python.NewOSRunner()

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
			o.markFailed(wfiID, req, "cancelled")
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

	const maxCallbacks = 3
	callbackCount := 0
	wasCallback := false // tracks if current layer is a callback re-run

	// Use index-based loop to support backward jumps on callback
	layerIdx := startLayerIdx
	for layerIdx < len(layerGroups) {
		lg := layerGroups[layerIdx]

		// Check cancellation
		select {
		case <-ctx.Done():
			logger.Warn(ctx, "workflow cancelled", "layer", lg.layer)
			o.markFailed(wfiID, req, "cancelled")
			return
		default:
		}

		runnableAgents := lg.phases

		// Check if layer should be skipped based on workflow instance skip_tags
		if shouldSkip, matchingTag := o.shouldSkipLayer(ctx, wfiID, runnableAgents, agentTags); shouldSkip {
			agentNames := make([]string, len(runnableAgents))
			for i, p := range runnableAgents {
				agentNames[i] = p.Agent
			}
			logger.Info(ctx, "layer skipped due to tag", "layer", lg.layer, "tag", matchingTag, "agents", agentNames)

			o.createSkippedSessions(ctx, wfiID, req, runnableAgents, pool)

			for _, phase := range runnableAgents {
				o.wsHub.Broadcast(ws.NewEvent(ws.EventAgentCompleted, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
					"agent_id":   phase.Agent,
					"agent_type": phase.Agent,
					"result":     "skipped",
				}))
			}
			o.wsHub.Broadcast(ws.NewEvent(ws.EventLayerSkipped, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
				"instance_id": wfiID,
				"layer":       lg.layer,
				"skip_tag":    matchingTag,
				"agents":      agentNames,
			}))

			layerIdx++
			continue
		}

		logger.Info(ctx, "running layer", "layer_idx", layerIdx+1, "total", len(layerGroups), "agents", len(runnableAgents))

		// Spawn all agents in this layer concurrently
		type spawnResult struct {
			agent           string
			err             error
			isCallback      bool
			cbLevel         int
			cbInstructions  string
		}
		results := make(chan spawnResult, len(runnableAgents))

		for _, phase := range runnableAgents {
			phase := phase // capture for goroutine
			go func() {
				sp := spawner.New(spawner.Config{
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
					ErrorSvc:                  o.errorSvc,
					Provider:                  apiProvider,
					AgentSvc:                  apiAgentSvc,
					APICredentialRepo:         apiCredRepo,
					FindingsSvc:               findingsSvc,
					ProjectFindingsSvc:        projectFindingsSvc,
					AgentSvcReal:              agentSvcReal,
					WorkflowSvc:               workflowSvcReal,
					ToolDefRepo:               toolDefRepo,
					APIMode:                   o.apiMode,
					PTYManager:                o.PTYManager,
					DispatchRepo:              dispatchRepo,
					ReviewRepo:                reviewRepo,
					PythonRunner:              pythonRunner,
					CustomerConfigDir:         customerConfigDir,
					ProjectEnv:                projectEnv,
					SDKDir:                    o.sdkDir,
					PythonPath:                pythonPath,
					PythonScriptRepo:          repo.NewPythonScriptRepo(pool, o.clock),
					OnSessionRegister: func(sid string, s *spawner.Spawner) {
						o.mu.Lock()
						if rs, ok := o.runs[wfiID]; ok {
							rs.spawners[sid] = s
						}
						o.mu.Unlock()
					},
					OnSessionUnregister: func(sid string) {
						o.mu.Lock()
						if rs, ok := o.runs[wfiID]; ok {
							delete(rs.spawners, sid)
						}
						o.mu.Unlock()
					},
				})

				err := sp.Spawn(ctx, spawner.SpawnRequest{
					AgentType:          phase.Agent,
					TicketID:           req.TicketID,
					ProjectID:          req.ProjectID,
					WorkflowName:       req.WorkflowName,
					ParentSession:      parentSession,
					ScopeType:          req.ScopeType,
					WorkflowInstanceID: wfiID,
				})

				sp.Close()

				sr := spawnResult{agent: phase.Agent, err: err}
				var cbErr *spawner.CallbackError
				if errors.As(err, &cbErr) {
					sr.isCallback = true
					sr.cbLevel = cbErr.Level
					sr.cbInstructions = cbErr.Instructions
					sr.err = nil // callback is not a failure
				}
				results <- sr
			}()
		}

		// Wait for all agents in this layer to finish
		passCount := 0
		failCount := 0
		callbackDetected := false
		callbackLevel := -1
		callbackAgent := ""
		callbackInstructions := ""
		for range runnableAgents {
			result := <-results
			if result.isCallback {
				passCount++ // callback counts as pass for layer aggregation
				callbackDetected = true
				// Use lowest callback level if multiple agents request callback
				if callbackLevel < 0 || result.cbLevel < callbackLevel {
					callbackLevel = result.cbLevel
					callbackAgent = result.agent
					callbackInstructions = result.cbInstructions
				}
			} else if result.err != nil {
				if ctx.Err() != nil {
					logger.Warn(ctx, "cancelled during layer", "layer", lg.layer)
					o.markFailed(wfiID, req, "cancelled")
					return
				}
				logger.Error(ctx, "layer agent failed", "layer", lg.layer, "agent", result.agent, "err", result.err)
				failCount++
			} else {
				logger.Info(ctx, "layer agent completed", "layer", lg.layer, "agent", result.agent)
				passCount++
			}
		}

		// Handle callback before layer aggregation failure check
		if callbackDetected {
			callbackCount++
			if callbackCount > maxCallbacks {
				logger.Error(ctx, "max callbacks exceeded", "max", maxCallbacks)
				o.markFailed(wfiID, req, fmt.Sprintf("max callbacks (%d) exceeded", maxCallbacks))
				return
			}

			targetIdx := o.handleCallback(ctx, wfiID, req, layerGroups, layerIdx, callbackLevel, callbackAgent, callbackInstructions)
			if targetIdx < 0 {
				o.markFailed(wfiID, req, fmt.Sprintf("callback target layer %d not found", callbackLevel))
				return
			}
			wasCallback = true
			layerIdx = targetIdx
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

		// Clear callback metadata after the callback target layer completes successfully
		if wasCallback {
			o.clearCallbackMetadata(ctx, wfiID)
			wasCallback = false
		}

		logger.Info(ctx, "layer completed", "layer", lg.layer, "passed", passCount, "failed", failCount)
		layerIdx++
	}

	// All layers completed
	logger.Info(ctx, "workflow completed", "workflow", req.WorkflowName, "target", target)

	// Merge worktree branch on success
	if wt != nil {
		wtService := &service.WorktreeService{}
		if err := wtService.MergeAndCleanup(wt.projectRoot, wt.defaultBranch, wt.branchName, wt.worktreePath); err != nil {
			// Attempt automatic conflict resolution
			if resolveErr := o.attemptConflictResolution(ctx, wfiID, req, wt, pool, err.Error(), modelConfigs, claudeSettingsJSON, customerConfigDir, projectEnv); resolveErr != nil {
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
	o.maybeStartNextOnSuccess(ctx, req, finalResult)

	// Endless loop: re-run a fresh instance if enabled and not stopped
	if req.IsProjectScope() && req.EndlessLoop && ctx.Err() == nil {
		o.maybeRestartEndlessLoop(wfiID, req)
	}
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
		"instance_id":          wfiID,
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

// handleCallback processes a callback: resets phases and sessions for layers between
// target and current (inclusive), saves callback metadata to WFI findings, and broadcasts.
// Returns the target layer index, or -1 if the target layer number is not found.
func (o *Orchestrator) handleCallback(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	layerGroups []layerGroup,
	currentIdx int,
	callbackLevel int,
	callbackAgent string,
	callbackInstructions string,
) int {
	// Map callback_level (layer field value) to layerGroups index
	targetIdx := -1
	for i, lg := range layerGroups {
		if lg.layer == callbackLevel {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		logger.Error(ctx, "callback target layer not found", "target_layer", callbackLevel)
		return -1
	}

	logger.Info(ctx, "callback detected", "from_layer", layerGroups[currentIdx].layer, "to_layer", callbackLevel, "agent", callbackAgent)

	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(ctx, "failed to open DB for callback", "err", err)
		return -1
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	asRepo := repo.NewAgentSessionRepo(database, o.clock)

	// Save _callback metadata to workflow instance findings
	wi, err := wfiRepo.Get(wfiID)
	if err == nil {
		findings := wi.GetFindings()
		findings["_callback"] = map[string]interface{}{
			"level":        callbackLevel,
			"instructions": callbackInstructions,
			"from_layer":   layerGroups[currentIdx].layer,
			"from_agent":   callbackAgent,
		}
		findingsJSON, _ := json.Marshal(findings)
		wfiRepo.UpdateFindings(wfiID, string(findingsJSON))
	}

	// Collect phase names for agent session reset
	var resetPhases []string
	for i := targetIdx; i <= currentIdx; i++ {
		for _, p := range layerGroups[i].phases {
			resetPhases = append(resetPhases, p.Agent)
		}
	}

	// Reset agent sessions for those phases
	asRepo.ResetSessionsForCallback(wfiID, resetPhases)

	// Broadcast callback event
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationCallback, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id":  wfiID,
		"from_layer":   layerGroups[currentIdx].layer,
		"to_layer":     callbackLevel,
		"instructions": callbackInstructions,
	}))

	return targetIdx
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
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	wi, err := wfiRepo.Get(wfiID)
	if err != nil {
		logger.Error(ctx, "failed to load WFI to clear callback metadata", "err", err)
		return
	}

	findings := wi.GetFindings()
	delete(findings, "_callback")
	findingsJSON, _ := json.Marshal(findings)
	wfiRepo.UpdateFindings(wfiID, string(findingsJSON))
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

// markCompleted marks the workflow instance as completed and broadcasts.
// Returns the workflow_final_result finding value (empty string if not set).
func (o *Orchestrator) markCompleted(wfiID string, req RunRequest) (finalResult string) {
	o.updateOrchestrationStatus(wfiID, "completed")

	database, err := db.Open(o.dataPath)
	if err != nil {
		return
	}
	defer database.Close()
	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)

	if req.IsProjectScope() {
		wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceProjectCompleted)
		asRepo := repo.NewAgentSessionRepo(database, o.clock)
		asRepo.UpdateStatusByWorkflowInstance(wfiID, model.AgentSessionProjectCompleted)
	} else {
		wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceCompleted)
		if req.CloseTicketOnComplete {
			ticketService := service.NewTicketService(pool, o.clock)
			ticket, err := ticketService.Get(req.ProjectID, req.TicketID)
			if err != nil {
				logger.Error(context.Background(), "failed to fetch ticket for auto-close", "ticket", req.TicketID, "err", err)
			} else if ticket.Status == model.StatusClosed {
				logger.Info(context.Background(), "skipping auto-close: ticket already closed", "ticket", req.TicketID)
			} else {
				reason := fmt.Sprintf("Workflow '%s' completed successfully", req.WorkflowName)
				if err := ticketService.Close(req.ProjectID, req.TicketID, reason); err != nil {
					logger.Error(context.Background(), "failed to close ticket", "ticket", req.TicketID, "err", err)
				} else {
					o.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, req.ProjectID, req.TicketID, "", map[string]interface{}{"status": "closed"}))
					// Best-effort: auto-close parent epic if all children are now closed
					if epic, err := ticketService.TryCloseParentEpic(req.ProjectID, req.TicketID); err != nil {
						logger.Error(context.Background(), "failed to auto-close parent epic", "ticket", req.TicketID, "err", err)
					} else if epic != nil {
						o.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, req.ProjectID, epic.ID, "", map[string]interface{}{"status": "closed"}))
					}
				}
			}
		}
	}

	finalResult = service.ExtractWorkflowFinalResultByInstanceID(pool, wfiID)
	data := map[string]interface{}{"instance_id": wfiID}
	if finalResult != "" {
		data["workflow_final_result"] = finalResult
	}
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationCompleted, req.ProjectID, req.TicketID, req.WorkflowName, data))
	if req.WorkflowName == ws.SpecImportWorkflowID {
		o.wsHub.Broadcast(ws.NewEvent(ws.EventSpecImportReady, req.ProjectID, "", req.WorkflowName, map[string]interface{}{
			"instance_id": wfiID,
		}))
	}
	return
}

// markFailed marks the workflow instance as failed and broadcasts.
func (o *Orchestrator) markFailed(wfiID string, req RunRequest, reason string) {
	o.updateOrchestrationStatus(wfiID, "failed")

	database, err := db.Open(o.dataPath)
	if err != nil {
		return
	}
	defer database.Close()
	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceFailed)

	// Revert ticket from in_progress to open so it's not stuck (ticket scope only)
	if !req.IsProjectScope() {
		ticketService := service.NewTicketService(pool, o.clock)
		if err := ticketService.Reopen(req.ProjectID, req.TicketID); err != nil {
			logger.Error(context.Background(), "failed to reopen ticket after failure", "ticket", req.TicketID, "err", err)
		} else {
			o.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, req.ProjectID, req.TicketID, "", map[string]interface{}{"status": "open"}))
		}
	}

	if o.errorSvc != nil {
		o.errorSvc.RecordError(req.ProjectID, "workflow", wfiID, reason)
	}

	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationFailed, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wfiID,
		"reason":      reason,
	}))
	if req.WorkflowName == ws.SpecImportWorkflowID {
		o.wsHub.Broadcast(ws.NewEvent(ws.EventSpecImportFailed, req.ProjectID, "", req.WorkflowName, map[string]interface{}{
			"instance_id": wfiID,
			"error":       reason,
		}))
	}
}

// updateOrchestrationStatus updates the _orchestration key in findings.
func (o *Orchestrator) updateOrchestrationStatus(wfiID, status string) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	wi, err := wfiRepo.Get(wfiID)
	if err != nil {
		return
	}

	findings := wi.GetFindings()
	findings["_orchestration"] = map[string]interface{}{
		"status": status,
	}
	findingsJSON, _ := json.Marshal(findings)
	wfiRepo.UpdateFindings(wfiID, string(findingsJSON))
}

// convertToSpawnerWorkflows converts service types to spawner types.
func convertToSpawnerWorkflows(svc map[string]service.SpawnerWorkflowDef) map[string]spawner.WorkflowDef {
	result := make(map[string]spawner.WorkflowDef, len(svc))
	for name, swf := range svc {
		var phases []spawner.PhaseDef
		for _, sp := range swf.Phases {
			phases = append(phases, spawner.PhaseDef{
				ID:    sp.ID,
				Agent: sp.Agent,
				Layer: sp.Layer,
			})
		}
		result[name] = spawner.WorkflowDef{
			Description: swf.Description,
			ScopeType:   swf.ScopeType,
			Phases:      phases,
		}
	}
	return result
}

// convertToSpawnerAgents converts service types to spawner types.
func convertToSpawnerAgents(svc map[string]service.SpawnerAgentConfig) map[string]spawner.AgentConfig {
	result := make(map[string]spawner.AgentConfig, len(svc))
	for name, sa := range svc {
		result[name] = spawner.AgentConfig{
			Model:   sa.Model,
			Timeout: sa.Timeout,
		}
	}
	return result
}

// RecordUserInput persists a user-typed line for the given session.
// Delegates to the active spawner if the session belongs to a live proc;
// falls back to a direct DB insert when no active spawner owns the session
// (user_interactive / resume-session cases).
func (o *Orchestrator) RecordUserInput(sessionID, text string) {
	o.mu.Lock()
	seen := make(map[*spawner.Spawner]struct{})
	var uniqueSpawners []*spawner.Spawner
	for _, rs := range o.runs {
		for _, sp := range rs.spawners {
			if _, ok := seen[sp]; ok {
				continue
			}
			seen[sp] = struct{}{}
			uniqueSpawners = append(uniqueSpawners, sp)
		}
	}
	o.mu.Unlock()

	for _, sp := range uniqueSpawners {
		if sp.RecordUserInput(sessionID, text) {
			return
		}
	}

	// No active spawner owns this session — insert directly into the DB.
	recordUserInputFallback(o.dataPath, o.clock, o.wsHub, sessionID, text)
}

// recordUserInputFallback inserts a user_input message row and broadcasts
// EventMessagesUpdated. Used when no live spawner proc is tracking the session
// (e.g. user_interactive take-control or resume-session flows).
func recordUserInputFallback(dataPath string, clk clock.Clock, hub *ws.Hub, sessionID, text string) {
	database, err := db.Open(dataPath)
	if err != nil {
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	msgRepo := repo.NewAgentMessageRepo(pool, clk)
	count, err := msgRepo.CountBySession(sessionID)
	if err != nil {
		logger.Warn(context.Background(), "user input fallback: count failed", "session_id", sessionID, "err", err)
		return
	}
	if err := msgRepo.InsertBatch(sessionID, count, []repo.MessageEntry{{Content: text, Category: "user_input"}}); err != nil {
		logger.Warn(context.Background(), "user input fallback: insert failed", "session_id", sessionID, "err", err)
		return
	}

	if hub == nil {
		return
	}

	asRepo := repo.NewAgentSessionRepo(database, clk)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return
	}
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, clk)
	wfi, err := wfiRepo.Get(session.WorkflowInstanceID)
	if err != nil {
		return
	}

	modelID := ""
	if session.ModelID.Valid {
		modelID = session.ModelID.String
	}
	hub.Broadcast(ws.NewEvent(ws.EventMessagesUpdated, session.ProjectID, session.TicketID, wfi.WorkflowID, map[string]interface{}{
		"session_id": sessionID,
		"agent_type": session.AgentType,
		"model_id":   modelID,
	}))
}
