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
	"log"
	"sort"
	"sync"

	"github.com/google/uuid"

	"be/internal/db"
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
	Category     string `json:"category"`     // "full", "simple", "docs"
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

// runState tracks a running orchestration's cancel func and active spawner.
type runState struct {
	cancel  context.CancelFunc
	spawner *spawner.Spawner // nil between phases
}

// Orchestrator manages server-side workflow runs.
type Orchestrator struct {
	mu       sync.Mutex
	runs     map[string]*runState // wfi_id → state
	dataPath string
	wsHub    *ws.Hub
}

// New creates a new Orchestrator.
func New(dataPath string, wsHub *ws.Hub) *Orchestrator {
	return &Orchestrator{
		runs:     make(map[string]*runState),
		dataPath: dataPath,
		wsHub:    wsHub,
	}
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

	// Check if already running
	if req.IsProjectScope() {
		if o.IsProjectRunning(req.ProjectID, req.WorkflowName) {
			return nil, fmt.Errorf("project workflow '%s' is already running", req.WorkflowName)
		}
	} else {
		if o.IsRunning(req.ProjectID, req.TicketID, req.WorkflowName) {
			return nil, fmt.Errorf("workflow '%s' is already running on %s", req.WorkflowName, req.TicketID)
		}
	}

	// Load project from DB to get root_path
	database, err := db.Open(o.dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	projectRepo := repo.NewProjectRepo(database)
	project, err := projectRepo.Get(req.ProjectID)
	database.Close()
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}
	if !project.RootPath.Valid || project.RootPath.String == "" {
		return nil, fmt.Errorf("project '%s' has no root_path configured", req.ProjectID)
	}
	projectRoot := project.RootPath.String

	// Load DB workflow definitions and agent definitions
	database, err = db.Open(o.dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	wfRepo := repo.NewWorkflowRepo(database)
	dbWorkflows, err := wfRepo.List(req.ProjectID)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to load workflows: %w", err)
	}
	var dbAgentDefs []*model.AgentDefinition
	adRepo := repo.NewAgentDefinitionRepo(database)
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

	// Init workflow instance (or get existing)
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create db pool: %w", err)
	}
	wfService := service.NewWorkflowService(pool)

	// Try to init; if already exists, get the existing instance
	var initErr error
	if req.IsProjectScope() {
		initErr = wfService.InitProjectWorkflow(req.ProjectID, &types.ProjectWorkflowRunRequest{
			Workflow:     req.WorkflowName,
			Category:     req.Category,
			Instructions: req.Instructions,
		})
	} else {
		initErr = wfService.Init(req.ProjectID, req.TicketID, &types.WorkflowInitRequest{
			Workflow: req.WorkflowName,
		})
	}
	pool.Close()

	// Get the instance (whether just created or already existed)
	database, err = db.Open(o.dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	dbPool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(dbPool)

	var wi *model.WorkflowInstance
	if req.IsProjectScope() {
		wi, err = wfiRepo.GetByProjectAndWorkflow(req.ProjectID, req.WorkflowName)
	} else {
		wi, err = wfiRepo.GetByTicketAndWorkflow(req.ProjectID, req.TicketID, req.WorkflowName)
	}
	if err != nil {
		database.Close()
		if initErr != nil {
			return nil, fmt.Errorf("failed to init workflow: %w", initErr)
		}
		return nil, fmt.Errorf("failed to get workflow instance: %w", err)
	}

	// If re-running a completed project workflow, reset it to active with fresh phases
	if wi.Status == model.WorkflowInstanceProjectCompleted {
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

		// Update in-memory copy for downstream use
		wi.Status = model.WorkflowInstanceActive
		wi.PhaseOrder = string(phaseOrderJSON)
		wi.Phases = string(phasesJSON)
		wi.CurrentPhase = sql.NullString{String: firstPhase, Valid: firstPhase != ""}
		wi.Findings = "{}"
	}

	// Set category if specified
	if req.Category != "" {
		wfiRepo.UpdateCategory(wi.ID, req.Category)
	}

	// Store user instructions and orchestration status in findings (single write)
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
	database.Close()

	// Set ticket to in_progress if currently open (best-effort, ticket scope only)
	if !req.IsProjectScope() {
		if statusPool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig()); err == nil {
			ticketService := service.NewTicketService(statusPool)
			if err := ticketService.SetInProgress(req.ProjectID, req.TicketID); err != nil {
				log.Printf("[orchestrator] Failed to set ticket %s to in_progress: %v", req.TicketID, err)
			}
			statusPool.Close()
		}
	}

	// Build spawner config maps
	spawnWorkflows := convertToSpawnerWorkflows(svcWorkflows)
	spawnAgents := convertToSpawnerAgents(svcAgents)

	// Create orchestration context with cancel
	orchCtx, cancel := context.WithCancel(ctx)

	rs := &runState{cancel: cancel}
	o.mu.Lock()
	o.runs[wi.ID] = rs
	o.mu.Unlock()

	// Broadcast orchestration started
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationStarted, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wi.ID,
		"category":    req.Category,
	}))

	// Run orchestration loop in goroutine
	go o.runLoop(orchCtx, wi.ID, req, parentSession, projectRoot, spawnWorkflows, spawnAgents, svcWf, 0)

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
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)

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

// StopByProject stops any running orchestration for a project-scoped workflow.
func (o *Orchestrator) StopByProject(projectID, workflowName string) error {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)

	if workflowName != "" {
		wi, err := wfiRepo.GetByProjectAndWorkflow(projectID, workflowName)
		if err != nil {
			return fmt.Errorf("project workflow not found: %w", err)
		}
		return o.Stop(wi.ID)
	}

	instances, err := wfiRepo.ListByProjectScope(projectID)
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

	return fmt.Errorf("no running project orchestration found")
}

// RetryFailedAgent resets a failed workflow instance and re-runs from the failed layer.
func (o *Orchestrator) RetryFailedAgent(ctx context.Context, projectID, ticketID, workflowName, sessionID string) error {
	return o.retryFailed(ctx, projectID, ticketID, workflowName, sessionID, "ticket")
}

// RetryFailedProjectAgent resets a failed project-scoped workflow and re-runs from the failed layer.
func (o *Orchestrator) RetryFailedProjectAgent(ctx context.Context, projectID, workflowName, sessionID string) error {
	return o.retryFailed(ctx, projectID, "", workflowName, sessionID, "project")
}

func (o *Orchestrator) retryFailed(ctx context.Context, projectID, ticketID, workflowName, sessionID, scopeType string) error {
	// Look up the workflow instance
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)

	var wi *model.WorkflowInstance
	if scopeType == "project" {
		wi, err = wfiRepo.GetByProjectAndWorkflow(projectID, workflowName)
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
	asRepo := repo.NewAgentSessionRepo(database)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return fmt.Errorf("agent session not found: %w", err)
	}
	if session.WorkflowInstanceID != wi.ID {
		return fmt.Errorf("session does not belong to this workflow instance")
	}
	failedPhase := session.Phase

	// Load project root
	projectRepo := repo.NewProjectRepo(database)
	project, err := projectRepo.Get(projectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}
	if !project.RootPath.Valid || project.RootPath.String == "" {
		return fmt.Errorf("project '%s' has no root_path configured", projectID)
	}
	projectRoot := project.RootPath.String

	// Load workflow/agent definitions
	wfRepo := repo.NewWorkflowRepo(database)
	dbWorkflows, err := wfRepo.List(projectID)
	if err != nil {
		return fmt.Errorf("failed to load workflows: %w", err)
	}
	var dbAgentDefs []*model.AgentDefinition
	adRepo := repo.NewAgentDefinitionRepo(database)
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
		Category:     wi.Category.String,
		ScopeType:    scopeType,
	}

	// Create orchestration context
	orchCtx, cancel := context.WithCancel(ctx)
	rs := &runState{cancel: cancel}
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

	go o.runLoop(orchCtx, wi.ID, req, parentSession, projectRoot, spawnWorkflows, spawnAgents, svcWf, startLayerIdx)

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
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	wi, err := wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	if err != nil {
		return fmt.Errorf("workflow not found: %w", err)
	}

	return o.restartAgentByInstance(wi.ID, workflowName, ticketID, sessionID)
}

// RestartProjectAgent sends a restart signal for a project-scoped workflow agent.
func (o *Orchestrator) RestartProjectAgent(projectID, workflowName, sessionID string) error {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	wi, err := wfiRepo.GetByProjectAndWorkflow(projectID, workflowName)
	if err != nil {
		return fmt.Errorf("project workflow not found: %w", err)
	}

	return o.restartAgentByInstance(wi.ID, workflowName, projectID, sessionID)
}

func (o *Orchestrator) restartAgentByInstance(wfiID, workflowName, target, sessionID string) error {
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
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	wi, err := wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	if err != nil {
		return false
	}

	o.mu.Lock()
	_, running := o.runs[wi.ID]
	o.mu.Unlock()
	return running
}

// IsProjectRunning checks if an orchestration is running for a project-scoped workflow.
func (o *Orchestrator) IsProjectRunning(projectID, workflowName string) bool {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return false
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	wi, err := wfiRepo.GetByProjectAndWorkflow(projectID, workflowName)
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
) {
	defer func() {
		o.mu.Lock()
		delete(o.runs, wfiID)
		o.mu.Unlock()
	}()

	target := req.TicketID
	if req.IsProjectScope() {
		target = "project:" + req.ProjectID
	}
	log.Printf("[orchestrator] Starting workflow '%s' on %s (%d phases)",
		req.WorkflowName, target, len(svcWf.Phases))

	category := req.Category

	// Group phases by layer
	layerGroups := groupPhasesByLayer(svcWf.Phases)

	const maxCallbacks = 3
	callbackCount := 0

	// Use index-based loop to support backward jumps on callback
	layerIdx := startLayerIdx
	for layerIdx < len(layerGroups) {
		lg := layerGroups[layerIdx]

		// Check cancellation
		select {
		case <-ctx.Done():
			log.Printf("[orchestrator] Cancelled at layer %d", lg.layer)
			o.markFailed(wfiID, req, "cancelled")
			return
		default:
		}

		// Filter out skipped agents
		var runnableAgents []service.SpawnerPhaseDef
		for _, phase := range lg.phases {
			if shouldSkipPhase(category, phase.SkipFor) {
				log.Printf("[orchestrator] Skipping agent %s in layer %d (category=%s)", phase.Agent, lg.layer, category)
				continue
			}
			runnableAgents = append(runnableAgents, phase)
		}

		// If all agents in layer are skipped, continue to next layer
		if len(runnableAgents) == 0 {
			log.Printf("[orchestrator] Layer %d: all agents skipped, continuing", lg.layer)
			layerIdx++
			continue
		}

		log.Printf("[orchestrator] Running layer %d/%d: %d agent(s)", layerIdx+1, len(layerGroups), len(runnableAgents))

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
					DefaultCLI:  "claude",
					DataPath:    o.dataPath,
					ProjectRoot: projectRoot,
					WSHub:       o.wsHub,
				})

				// Store spawner ref so RestartAgent can reach it
				o.mu.Lock()
				if rs, ok := o.runs[wfiID]; ok {
					rs.spawner = sp
				}
				o.mu.Unlock()

				err := sp.Spawn(ctx, spawner.SpawnRequest{
					AgentType:     phase.Agent,
					TicketID:      req.TicketID,
					ProjectID:     req.ProjectID,
					WorkflowName:  req.WorkflowName,
					ParentSession: parentSession,
					ScopeType:     req.ScopeType,
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
					log.Printf("[orchestrator] Cancelled during layer %d", lg.layer)
					o.markFailed(wfiID, req, "cancelled")
					return
				}
				log.Printf("[orchestrator] Layer %d agent %s failed: %v", lg.layer, result.agent, result.err)
				failCount++
			} else {
				log.Printf("[orchestrator] Layer %d agent %s completed successfully", lg.layer, result.agent)
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
				log.Printf("[orchestrator] Max callbacks (%d) exceeded, failing workflow", maxCallbacks)
				o.markFailed(wfiID, req, fmt.Sprintf("max callbacks (%d) exceeded", maxCallbacks))
				return
			}

			targetIdx := o.handleCallback(wfiID, req, layerGroups, layerIdx, callbackLevel, callbackAgent, callbackInstructions)
			if targetIdx < 0 {
				o.markFailed(wfiID, req, fmt.Sprintf("callback target layer %d not found", callbackLevel))
				return
			}
			layerIdx = targetIdx
			continue
		}

		// Fan-in: at least one pass required to proceed
		if passCount == 0 {
			log.Printf("[orchestrator] Layer %d: all agents failed (%d), stopping workflow", lg.layer, failCount)
			o.markFailed(wfiID, req, fmt.Sprintf("layer %d: all agents failed", lg.layer))
			return
		}

		log.Printf("[orchestrator] Layer %d completed: %d passed, %d failed", lg.layer, passCount, failCount)
		layerIdx++
	}

	// All layers completed
	log.Printf("[orchestrator] Workflow '%s' completed on %s", req.WorkflowName, target)
	o.markCompleted(wfiID, req)
}

// handleCallback processes a callback: resets phases and sessions for layers between
// target and current (inclusive), saves callback metadata to WFI findings, and broadcasts.
// Returns the target layer index, or -1 if the target layer number is not found.
func (o *Orchestrator) handleCallback(
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
		log.Printf("[orchestrator] Callback target layer %d not found in layer groups", callbackLevel)
		return -1
	}

	log.Printf("[orchestrator] Callback detected: returning to layer %d (idx %d) from layer %d (idx %d), agent=%s",
		callbackLevel, targetIdx, layerGroups[currentIdx].layer, currentIdx, callbackAgent)

	database, err := db.Open(o.dataPath)
	if err != nil {
		log.Printf("[orchestrator] Failed to open DB for callback: %v", err)
		return -1
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	asRepo := repo.NewAgentSessionRepo(database)

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
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)

	if req.IsProjectScope() {
		wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceProjectCompleted)
		asRepo := repo.NewAgentSessionRepo(database)
		asRepo.UpdateStatusByWorkflowInstance(wfiID, model.AgentSessionProjectCompleted)
	} else {
		wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceCompleted)
		ticketService := service.NewTicketService(pool)
		reason := fmt.Sprintf("Workflow '%s' completed successfully", req.WorkflowName)
		if err := ticketService.Close(req.ProjectID, req.TicketID, reason); err != nil {
			log.Printf("[orchestrator] Failed to close ticket %s: %v", req.TicketID, err)
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
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceFailed)

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
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
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

// shouldSkipPhase checks if a phase should be skipped for the given category.
func shouldSkipPhase(category string, skipFor []string) bool {
	if category == "" || len(skipFor) == 0 {
		return false
	}
	for _, skip := range skipFor {
		if skip == category {
			return true
		}
	}
	return false
}

// convertToSpawnerWorkflows converts service types to spawner types.
func convertToSpawnerWorkflows(svc map[string]service.SpawnerWorkflowDef) map[string]spawner.WorkflowDef {
	result := make(map[string]spawner.WorkflowDef, len(svc))
	for name, swf := range svc {
		var phases []spawner.PhaseDef
		for _, sp := range swf.Phases {
			phases = append(phases, spawner.PhaseDef{
				ID:      sp.ID,
				Agent:   sp.Agent,
				Layer:   sp.Layer,
				SkipFor: sp.SkipFor,
			})
		}
		result[name] = spawner.WorkflowDef{
			Description: swf.Description,
			ScopeType:   swf.ScopeType,
			Categories:  swf.Categories,
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
