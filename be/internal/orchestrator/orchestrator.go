// Package orchestrator provides server-side workflow orchestration.
// It runs entire workflows automatically by sequencing spawner calls
// for each phase, driven from the HTTP API (web UI "Run Workflow" button).
package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/types"
	"be/internal/ws"
)

// RunRequest contains parameters for starting an orchestrated workflow run.
type RunRequest struct {
	ProjectID    string `json:"project_id"`
	TicketID     string `json:"ticket_id"`
	WorkflowName string `json:"workflow"`
	Instructions string `json:"instructions"` // User-provided instructions
	ScopeType    string `json:"scope_type"`   // "ticket" (default) or "project"
}

// IsProjectScope returns true if this is a project-scoped run request
func (r RunRequest) IsProjectScope() bool {
	return r.ScopeType == "project"
}

// RunResult contains the result of starting an orchestrated workflow.
type RunResult struct {
	InstanceID string `json:"instance_id"`
	Status     string `json:"status"`
}

// worktreeInfo stores git worktree metadata for a workflow run.
type worktreeInfo struct {
	projectRoot   string // original project root (for merge commands)
	worktreePath  string
	branchName    string
	defaultBranch string
}

// runState tracks a running orchestration's cancel func and active spawner.
type runState struct {
	cancel  context.CancelFunc
	spawner *spawner.Spawner // nil between phases
	done    chan struct{}     // closed when runLoop goroutine exits
}

// Orchestrator manages server-side workflow runs.
type Orchestrator struct {
	mu       sync.Mutex
	runs     map[string]*runState // wfi_id → state
	dataPath string
	wsHub    *ws.Hub
	clock    clock.Clock
}

// New creates a new Orchestrator.
func New(dataPath string, wsHub *ws.Hub, clk clock.Clock) *Orchestrator {
	return &Orchestrator{
		runs:     make(map[string]*runState),
		dataPath: dataPath,
		wsHub:    wsHub,
		clock:    clk,
	}
}

// setupWorktree creates a git worktree for a workflow run if the project has
// worktrees enabled. Returns worktreeInfo (nil if disabled) and the effective
// projectRoot (worktree path if enabled, original path if disabled).
func setupWorktree(project *model.Project, projectRoot, branchName string) (*worktreeInfo, string, error) {
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

	// Setup git worktree if enabled
	branchName := req.TicketID
	if req.IsProjectScope() {
		branchName = "project-" + uuid.New().String()[:8]
	}
	wt, projectRoot, err := setupWorktree(project, projectRoot, branchName)
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
	dbWorkflows, err := wfRepo.List(req.ProjectID)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to load workflows: %w", err)
	}
	var dbAgentDefs []*model.AgentDefinition
	adRepo := repo.NewAgentDefinitionRepo(database, o.clock)
	for _, wf := range dbWorkflows {
		defs, loadErr := adRepo.List(req.ProjectID, wf.ID)
		if loadErr == nil {
			dbAgentDefs = append(dbAgentDefs, defs...)
		}
	}
	database.Close()

	// Convert to spawner types
	svcWorkflows, svcAgents := service.BuildSpawnerConfig(dbWorkflows, dbAgentDefs)

	// Find the requested workflow
	svcWf, ok := svcWorkflows[req.WorkflowName]
	if !ok {
		return nil, fmt.Errorf("workflow definition '%s' not found", req.WorkflowName)
	}
	if len(svcWf.Phases) == 0 {
		return nil, fmt.Errorf("workflow '%s' has no phases", req.WorkflowName)
	}

	// Init workflow instance — always creates a fresh instance
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create db pool: %w", err)
	}
	wfService := service.NewWorkflowService(pool, o.clock)

	var wi *model.WorkflowInstance
	if req.IsProjectScope() {
		wi, err = wfService.InitProjectWorkflow(req.ProjectID, &types.ProjectWorkflowRunRequest{
			Workflow:     req.WorkflowName,
			Instructions: req.Instructions,
		})
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("failed to init workflow: %w", err)
		}
	} else {
		// Ticket scope: try to create, if already exists get existing and reset
		initErr := wfService.Init(req.ProjectID, req.TicketID, &types.WorkflowInitRequest{
			Workflow: req.WorkflowName,
		})
		if initErr == nil {
			wi, err = wfService.GetWorkflowInstance(req.ProjectID, req.TicketID, req.WorkflowName)
		} else {
			// Already exists — get existing instance and reset if completed/failed
			wi, err = wfService.GetWorkflowInstance(req.ProjectID, req.TicketID, req.WorkflowName)
			if err == nil && (wi.Status == model.WorkflowInstanceCompleted || wi.Status == model.WorkflowInstanceFailed) {
				wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
				wfiRepo.UpdateStatus(wi.ID, model.WorkflowInstanceActive)

				// Rebuild fresh phases from workflow definition
				phaseOrder := make([]string, len(svcWf.Phases))
				phases := make(map[string]model.PhaseStatus)
				var firstPhase string
				for i, p := range svcWf.Phases {
					phaseOrder[i] = p.ID
					phases[p.ID] = model.PhaseStatus{Status: "pending"}
					if i == 0 {
						firstPhase = p.ID
					}
				}
				phaseOrderJSON, _ := json.Marshal(phaseOrder)
				phasesJSON, _ := json.Marshal(phases)
				wfiRepo.UpdatePhases(wi.ID, string(phasesJSON))
				wfiRepo.UpdateCurrentPhase(wi.ID, firstPhase)
				wfiRepo.UpdateFindings(wi.ID, "{}")
				wfiRepo.UpdateRetryCount(wi.ID, wi.RetryCount+1)

				// Update in-memory copy
				wi.Status = model.WorkflowInstanceActive
				wi.PhaseOrder = string(phaseOrderJSON)
				wi.Phases = string(phasesJSON)
				wi.CurrentPhase = sql.NullString{String: firstPhase, Valid: firstPhase != ""}
				wi.Findings = "{}"
			}
		}
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("failed to init workflow: %w", err)
		}
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

	// Create orchestration context detached from HTTP request context.
	// Using ctx (r.Context()) as parent would cancel the orchestration when
	// the HTTP handler returns. Propagate the trx for log correlation.
	orchCtx, cancel := context.WithCancel(logger.WithTrx(context.Background(), logger.TrxFromContext(ctx)))

	rs := &runState{cancel: cancel, done: make(chan struct{})}
	o.mu.Lock()
	o.runs[wi.ID] = rs
	o.mu.Unlock()

	// Broadcast orchestration started
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationStarted, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wi.ID,
	}))

	// Run orchestration loop in goroutine
	launched = true
	go o.runLoop(orchCtx, wi.ID, req, parentSession, projectRoot, spawnWorkflows, spawnAgents, svcWf, 0, wt)

	return &RunResult{
		InstanceID: wi.ID,
		Status:     "started",
	}, nil
}

// Stop cancels a running orchestration.
func (o *Orchestrator) Stop(instanceID string) error {
	o.mu.Lock()
	rs, ok := o.runs[instanceID]
	o.mu.Unlock()

	if !ok {
		return fmt.Errorf("no running orchestration for instance %s", instanceID)
	}

	rs.cancel()
	return nil
}

// StopByTicket stops any running orchestration for a ticket+workflow.
func (o *Orchestrator) StopByTicket(projectID, ticketID, workflowName string) error {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)

	if workflowName != "" {
		wi, err := wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
		if err != nil {
			return fmt.Errorf("workflow not found: %w", err)
		}
		return o.Stop(wi.ID)
	}

	// Stop first running orchestration for this ticket
	instances, err := wfiRepo.ListByTicket(projectID, ticketID)
	if err != nil {
		return err
	}

	for _, wi := range instances {
		o.mu.Lock()
		_, running := o.runs[wi.ID]
		o.mu.Unlock()
		if running {
			return o.Stop(wi.ID)
		}
	}

	return fmt.Errorf("no running orchestration found for %s", ticketID)
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
	} else if scopeType == "project" {
		// Fallback: session lookup will validate the instance
		return fmt.Errorf("instance_id is required for project-scoped workflow retry")
	} else {
		wi, err = wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
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
	if scopeType == "project" {
		branchName = "project-" + uuid.New().String()[:8]
	}
	wt, projectRoot, wtErr := setupWorktree(project, projectRoot, branchName)
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
	dbWorkflows, err := wfRepo.List(projectID)
	if err != nil {
		return fmt.Errorf("failed to load workflows: %w", err)
	}
	var dbAgentDefs []*model.AgentDefinition
	adRepo := repo.NewAgentDefinitionRepo(database, o.clock)
	for _, wf := range dbWorkflows {
		defs, loadErr := adRepo.List(projectID, wf.ID)
		if loadErr == nil {
			dbAgentDefs = append(dbAgentDefs, defs...)
		}
	}

	svcWorkflows, svcAgents := service.BuildSpawnerConfig(dbWorkflows, dbAgentDefs)
	svcWf, ok := svcWorkflows[workflowName]
	if !ok {
		return fmt.Errorf("workflow definition '%s' not found", workflowName)
	}

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

	// Reset phases in the failed layer back to pending
	for _, p := range layerGroups[startLayerIdx].phases {
		wfiRepo.ResetPhaseStatus(wi.ID, p.Agent)
	}

	// Increment retry count
	wfiRepo.UpdateRetryCount(wi.ID, wi.RetryCount+1)

	// Update orchestration status in findings
	findings := wi.GetFindings()
	findings["_orchestration"] = map[string]interface{}{
		"status": "running",
	}
	findingsJSON, _ := json.Marshal(findings)
	wfiRepo.UpdateFindings(wi.ID, string(findingsJSON))

	// Build spawner config
	spawnWorkflows := convertToSpawnerWorkflows(svcWorkflows)
	spawnAgents := convertToSpawnerAgents(svcAgents)

	parentSession := uuid.New().String()

	// Build run request
	req := RunRequest{
		ProjectID:    projectID,
		TicketID:     ticketID,
		WorkflowName: workflowName,
		ScopeType:    scopeType,
	}

	// Create orchestration context detached from HTTP request context
	orchCtx, cancel := context.WithCancel(logger.WithTrx(context.Background(), logger.TrxFromContext(ctx)))
	rs := &runState{cancel: cancel, done: make(chan struct{})}
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
	go o.runLoop(orchCtx, wi.ID, req, parentSession, projectRoot, spawnWorkflows, spawnAgents, svcWf, startLayerIdx, wt)

	return nil
}

// RestartAgent sends a manual restart signal to the active spawner for a workflow.
func (o *Orchestrator) RestartAgent(projectID, ticketID, workflowName, sessionID string) error {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	wi, err := wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	if err != nil {
		return fmt.Errorf("workflow not found: %w", err)
	}

	return o.restartAgentByInstance(wi.ID, workflowName, ticketID, sessionID)
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
	sp := rs.spawner
	o.mu.Unlock()
	if sp == nil {
		return fmt.Errorf("no active spawner (agent may be between phases)")
	}

	sp.RequestRestart(sessionID)
	return nil
}

// IsRunning checks if an orchestration is running for a ticket+workflow.
func (o *Orchestrator) IsRunning(projectID, ticketID, workflowName string) bool {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return false
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	wi, err := wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	if err != nil {
		return false
	}

	o.mu.Lock()
	_, running := o.runs[wi.ID]
	o.mu.Unlock()
	return running
}

// IsInstanceRunning checks if a specific instance ID has an active orchestration.
func (o *Orchestrator) IsInstanceRunning(instanceID string) bool {
	o.mu.Lock()
	_, running := o.runs[instanceID]
	o.mu.Unlock()
	return running
}

// StopAll cancels all running orchestrations (for server shutdown).
func (o *Orchestrator) StopAll() {
	o.mu.Lock()
	logger.Warn(context.Background(), "stopping all orchestrations", "count", len(o.runs))
	for id, rs := range o.runs {
		rs.cancel()
		delete(o.runs, id)
	}
	o.mu.Unlock()
}

// runLoop executes workflow phases grouped by layer.
// All agents in the same layer run concurrently. Layers execute in ascending order.
// Fan-in: proceed to next layer if pass_count >= 1. All-skipped continues.
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

	// Group phases by layer
	layerGroups := groupPhasesByLayer(svcWf.Phases)

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
					Workflows:   workflows,
					Agents:      agents,
					DataPath:    o.dataPath,
					ProjectRoot: projectRoot,
					WSHub:       o.wsHub,
					Pool:        pool,
					Clock:       o.clock,
				})

				// Store spawner ref so RestartAgent can reach it
				o.mu.Lock()
				if rs, ok := o.runs[wfiID]; ok {
					rs.spawner = sp
				}
				o.mu.Unlock()

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
				passCount++ // callback counts as pass for fan-in
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

		// Clear spawner ref (layer done)
		o.mu.Lock()
		if rs, ok := o.runs[wfiID]; ok {
			rs.spawner = nil
		}
		o.mu.Unlock()

		// Handle callback before fan-in failure check
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

		// Fan-in: at least one pass required to proceed
		if passCount == 0 {
			logger.Error(ctx, "all agents failed in layer", "layer", lg.layer, "fail_count", failCount)
			o.markFailed(wfiID, req, fmt.Sprintf("layer %d: all agents failed", lg.layer))
			return
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
			logger.Error(ctx, "worktree merge failed — branch preserved for manual resolution", "branch", wt.branchName, "err", err)
			o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationCompleted, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
				"instance_id":    wfiID,
				"merge_error":    err.Error(),
				"branch":         wt.branchName,
				"worktree_path":  wt.worktreePath,
			}))
		} else {
			logger.Info(ctx, "worktree merged and cleaned up", "branch", wt.branchName)
		}
		worktreeHandled = true
	}

	o.markCompleted(wfiID, req)
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

	// Reset phases for all layers from targetIdx to currentIdx (inclusive)
	var resetPhases []string
	for i := targetIdx; i <= currentIdx; i++ {
		for _, p := range layerGroups[i].phases {
			wfiRepo.ResetPhaseStatus(wfiID, p.Agent)
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
func (o *Orchestrator) markCompleted(wfiID string, req RunRequest) {
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
		ticketService := service.NewTicketService(pool, o.clock)
		reason := fmt.Sprintf("Workflow '%s' completed successfully", req.WorkflowName)
		if err := ticketService.Close(req.ProjectID, req.TicketID, reason); err != nil {
			logger.Error(context.Background(), "failed to close ticket", "ticket", req.TicketID, "err", err)
		} else {
			o.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, req.ProjectID, req.TicketID, "", map[string]interface{}{"status": "closed"}))
		}
	}

	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationCompleted, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wfiID,
	}))
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

	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationFailed, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wfiID,
		"reason":      reason,
	}))
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
