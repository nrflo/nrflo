// Package orchestrator provides server-side workflow orchestration.
// It runs entire workflows automatically by sequencing spawner calls
// for each phase, driven from the HTTP API (web UI "Run Workflow" button).
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"

	"nrworkflow/internal/db"
	"nrworkflow/internal/model"
	"nrworkflow/internal/repo"
	"nrworkflow/internal/service"
	"nrworkflow/internal/spawner"
	"nrworkflow/internal/types"
	"nrworkflow/internal/ws"
)

// RunRequest contains parameters for starting an orchestrated workflow run.
type RunRequest struct {
	ProjectID    string `json:"project_id"`
	TicketID     string `json:"ticket_id"`
	WorkflowName string `json:"workflow"`
	Category     string `json:"category"`     // "full", "simple", "docs"
	Instructions string `json:"instructions"` // User-provided instructions
}

// RunResult contains the result of starting an orchestrated workflow.
type RunResult struct {
	InstanceID string `json:"instance_id"`
	Status     string `json:"status"`
}

// Orchestrator manages server-side workflow runs.
type Orchestrator struct {
	mu       sync.Mutex
	runs     map[string]context.CancelFunc // wfi_id → cancel
	dataPath string
	wsHub    *ws.Hub
}

// New creates a new Orchestrator.
func New(dataPath string, wsHub *ws.Hub) *Orchestrator {
	return &Orchestrator{
		runs:     make(map[string]context.CancelFunc),
		dataPath: dataPath,
		wsHub:    wsHub,
	}
}

// Start begins an orchestrated workflow run. It initializes the workflow
// (or reuses existing), then runs all phases sequentially in a goroutine.
func (o *Orchestrator) Start(ctx context.Context, req RunRequest) (*RunResult, error) {
	// Validate
	if req.ProjectID == "" || req.TicketID == "" || req.WorkflowName == "" {
		return nil, fmt.Errorf("project_id, ticket_id, and workflow are required")
	}

	// Check if already running
	if o.IsRunning(req.ProjectID, req.TicketID, req.WorkflowName) {
		return nil, fmt.Errorf("workflow '%s' is already running on %s", req.WorkflowName, req.TicketID)
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

	// Load project config
	projConfig, err := service.LoadProjectConfig(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to load project config: %w", err)
	}

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
	svcWorkflows, svcAgents := service.BuildSpawnerConfig(projConfig, dbWorkflows, dbAgentDefs)

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
	initErr := wfService.Init(req.ProjectID, req.TicketID, &types.WorkflowInitRequest{
		Workflow: req.WorkflowName,
	})
	pool.Close()

	// Get the instance (whether just created or already existed)
	database, err = db.Open(o.dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	dbPool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(dbPool)
	wi, err := wfiRepo.GetByTicketAndWorkflow(req.ProjectID, req.TicketID, req.WorkflowName)
	if err != nil {
		database.Close()
		if initErr != nil {
			return nil, fmt.Errorf("failed to init workflow: %w", initErr)
		}
		return nil, fmt.Errorf("failed to get workflow instance: %w", err)
	}

	// Set category if specified
	if req.Category != "" {
		wfiRepo.UpdateCategory(wi.ID, req.Category)
	}

	// Store user instructions as workflow-level findings
	if req.Instructions != "" {
		findings := wi.GetFindings()
		findings["user_instructions"] = req.Instructions
		findingsJSON, _ := json.Marshal(findings)
		wfiRepo.UpdateFindings(wi.ID, string(findingsJSON))
	}

	// Store orchestration status in findings
	findings := wi.GetFindings()
	findings["_orchestration"] = map[string]interface{}{
		"status": "running",
	}
	findingsJSON, _ := json.Marshal(findings)
	wfiRepo.UpdateFindings(wi.ID, string(findingsJSON))

	// Set parent session
	parentSession := uuid.New().String()
	database.Close()

	// Build spawner config maps
	spawnWorkflows := convertToSpawnerWorkflows(svcWorkflows)
	spawnAgents := convertToSpawnerAgents(svcAgents)

	// Create orchestration context with cancel
	orchCtx, cancel := context.WithCancel(ctx)

	o.mu.Lock()
	o.runs[wi.ID] = cancel
	o.mu.Unlock()

	// Broadcast orchestration started
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationStarted, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wi.ID,
		"category":    req.Category,
	}))

	// Run orchestration loop in goroutine
	go o.runLoop(orchCtx, wi.ID, req, parentSession, projectRoot, projConfig, spawnWorkflows, spawnAgents, svcWf)

	return &RunResult{
		InstanceID: wi.ID,
		Status:     "started",
	}, nil
}

// Stop cancels a running orchestration.
func (o *Orchestrator) Stop(instanceID string) error {
	o.mu.Lock()
	cancel, ok := o.runs[instanceID]
	o.mu.Unlock()

	if !ok {
		return fmt.Errorf("no running orchestration for instance %s", instanceID)
	}

	cancel()
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
	for id, cancel := range o.runs {
		cancel()
		delete(o.runs, id)
	}
	o.mu.Unlock()
}

// runLoop executes workflow phases sequentially.
func (o *Orchestrator) runLoop(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	parentSession string,
	projectRoot string,
	projConfig *service.ProjectConfig,
	workflows map[string]spawner.WorkflowDef,
	agents map[string]spawner.AgentConfig,
	svcWf service.SpawnerWorkflowDef,
) {
	defer func() {
		o.mu.Lock()
		delete(o.runs, wfiID)
		o.mu.Unlock()
	}()

	log.Printf("[orchestrator] Starting workflow '%s' on %s (%d phases)",
		req.WorkflowName, req.TicketID, len(svcWf.Phases))

	category := req.Category

	for i, phase := range svcWf.Phases {
		// Check cancellation
		select {
		case <-ctx.Done():
			log.Printf("[orchestrator] Cancelled during phase %s", phase.ID)
			o.markFailed(wfiID, req, "cancelled")
			return
		default:
		}

		// Check skip rules
		if shouldSkipPhase(category, phase.SkipFor) {
			log.Printf("[orchestrator] Skipping phase %s (category=%s)", phase.ID, category)
			continue
		}

		log.Printf("[orchestrator] Running phase %d/%d: %s (agent=%s)",
			i+1, len(svcWf.Phases), phase.ID, phase.Agent)

		// Create spawner for this phase
		sp := spawner.New(spawner.Config{
			Workflows:        workflows,
			Agents:           agents,
			DefaultCLI:       projConfig.CLI.Default,
			DataPath:         o.dataPath,
			ProjectRoot:      projectRoot,
			MaxContinuations: projConfig.Spawner.MaxContinuations,
			ContextThreshold: projConfig.Spawner.ContextThreshold,
			WSHub:            o.wsHub,
		})

		err := sp.Spawn(ctx, spawner.SpawnRequest{
			AgentType:     phase.Agent,
			TicketID:      req.TicketID,
			ProjectID:     req.ProjectID,
			WorkflowName:  req.WorkflowName,
			ParentSession: parentSession,
		})
		sp.Close()

		if err != nil {
			if ctx.Err() != nil {
				log.Printf("[orchestrator] Cancelled during phase %s", phase.ID)
				o.markFailed(wfiID, req, "cancelled")
				return
			}
			log.Printf("[orchestrator] Phase %s failed: %v", phase.ID, err)
			o.markFailed(wfiID, req, fmt.Sprintf("phase %s failed: %v", phase.ID, err))
			return
		}

		log.Printf("[orchestrator] Phase %s completed successfully", phase.ID)
	}

	// All phases completed
	log.Printf("[orchestrator] Workflow '%s' completed on %s", req.WorkflowName, req.TicketID)
	o.markCompleted(wfiID, req)
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
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceCompleted)

	// Close the ticket (best-effort)
	ticketService := service.NewTicketService(pool)
	reason := fmt.Sprintf("Workflow '%s' completed successfully", req.WorkflowName)
	if err := ticketService.Close(req.ProjectID, req.TicketID, reason); err != nil {
		log.Printf("[orchestrator] Failed to close ticket %s: %v", req.TicketID, err)
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
			pd := spawner.PhaseDef{
				ID:      sp.ID,
				Agent:   sp.Agent,
				SkipFor: sp.SkipFor,
			}
			if sp.Parallel != nil {
				pd.Parallel = &struct {
					Enabled bool     `json:"enabled"`
					Models  []string `json:"models"`
				}{Enabled: sp.Parallel.Enabled, Models: sp.Parallel.Models}
			}
			phases = append(phases, pd)
		}
		result[name] = spawner.WorkflowDef{
			Description: swf.Description,
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
