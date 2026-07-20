package spawner

import (
	"context"
	"fmt"
	"sync"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/tools_python"
)

// Spawner manages agent lifecycle
type Spawner struct {
	config               Config
	restartCh            chan string                    // carries sessionID of agent to restart
	takeControlCh        chan string                    // carries sessionID of agent to take control of
	takeControlReadies   map[string]chan struct{}       // sessionID → closed when status is user_interactive (or take-control rejected/attached)
	takeControlReadiesMu sync.Mutex                     // protects takeControlReadies
	terminalSignals      map[string]chan terminalSignal // sessionID → its monitorAll's receive channel
	terminalSignalsMu    sync.Mutex                     // protects terminalSignals
	bumpMessageCh        chan string                    // carries sessionID to bump lastMessageTime (hook events)
	nudgeRequestCh       chan nudgeRequest              // carries in-band Notification-hook idle/permission nudge requests
	interactiveWaits     map[string]chan struct{}       // sessionID → closed when interactive session completes
	killedInteractive    map[string]struct{}            // sessionID → killed by KillInteractive (checked after wait)
	mu                   sync.Mutex                     // protects interactiveWaits and killedInteractive
	sessionProcsMu       sync.Mutex                     // protects sessionProcs
	sessionProcs         map[string]*processInfo        // sessionID → live proc for RecordUserInput lookups
}

// SpawnRequest contains parameters for spawning an agent
type SpawnRequest struct {
	AgentType          string
	NodeID             string // execution identity (which slot in the run); AgentType stays the agent_definitions template key
	TicketID           string
	ProjectID          string
	WorkflowName       string
	ParentSession      string
	CLIName            string
	ScopeType          string            // "ticket" (default) or "project"
	WorkflowInstanceID string            // when set, used directly instead of DB lookup
	ExtraVars          map[string]string // Additional template variables (e.g., BRANCH_NAME, DEFAULT_BRANCH)
}

// IsProjectScope returns true if this is a project-scoped spawn request
func (r SpawnRequest) IsProjectScope() bool {
	return r.ScopeType == "project"
}

// New creates a new spawner
func New(config Config) *Spawner {
	return &Spawner{
		config:             config,
		restartCh:          make(chan string, 1),
		takeControlCh:      make(chan string, 1),
		takeControlReadies: make(map[string]chan struct{}),
		terminalSignals:    make(map[string]chan terminalSignal),
		bumpMessageCh:      make(chan string, 16), // buffered so concurrent heartbeats/hook bumps aren't dropped
		nudgeRequestCh:     make(chan nudgeRequest, 16),
		interactiveWaits:   make(map[string]chan struct{}),
		killedInteractive:  make(map[string]struct{}),
		sessionProcs:       make(map[string]*processInfo),
	}
}

// loadProjectPythonTools loads python_scripts rows with kind=tool for a project
// and constructs apirun.ToolHandler instances for each. Returns nil slice (no error)
// when PythonScriptRepo is not configured or no tool rows exist.
func (s *Spawner) loadProjectPythonTools(projectID, _ string) ([]apirun.ToolHandler, error) {
	if s.config.PythonScriptRepo == nil {
		return nil, nil
	}
	rows, err := s.config.PythonScriptRepo.ListByKind(projectID, "tool")
	if err != nil {
		return nil, err
	}
	handlers := make([]apirun.ToolHandler, 0, len(rows))
	for _, row := range rows {
		handlers = append(handlers, tools_python.New(row, s.config.PythonPath, s.config.SDKDir, s.config.ProjectEnv))
	}
	return handlers, nil
}

// Spawn spawns agents for a phase with context cancellation support.
func (s *Spawner) Spawn(ctx context.Context, req SpawnRequest) error {
	// Validate workflow
	workflow, ok := s.config.Workflows[req.WorkflowName]
	if !ok {
		return fmt.Errorf("unknown workflow: %s", req.WorkflowName)
	}

	// Find phase by node id (execution identity). AgentType alone cannot
	// disambiguate multiple nodes sharing the same template.
	nodeID := req.NodeID
	if nodeID == "" {
		nodeID = req.AgentType
	}
	var phase *PhaseDef
	for i := range workflow.Phases {
		if workflow.Phases[i].NodeID == nodeID {
			phase = &workflow.Phases[i]
			break
		}
	}
	if phase == nil {
		return fmt.Errorf("node '%s' not found in workflow '%s'", nodeID, req.WorkflowName)
	}

	// Validate workflow is initialized
	var wi *model.WorkflowInstance
	var err error
	if req.WorkflowInstanceID != "" {
		wi, err = s.getWorkflowInstanceByID(req.WorkflowInstanceID)
	} else if req.IsProjectScope() {
		wi, err = s.getProjectWorkflowInstance(req.ProjectID, req.WorkflowName)
	} else {
		wi, err = s.getWorkflowInstance(req.ProjectID, req.TicketID, req.WorkflowName)
	}
	if err != nil {
		return err
	}

	// Validate phase order
	if _, err := s.validateAndAdvancePhase(wi, req.WorkflowName, nodeID); err != nil {
		return err
	}

	// Determine model to spawn (single agent per Spawn call)
	model := "opus-4-8"
	if agentCfg, ok := s.config.Agents[req.AgentType]; ok && agentCfg.Model != "" {
		model = agentCfg.Model
	}
	cliName := req.CLIName
	if cliName == "" {
		cliName = s.cliForModel(model)
	}
	modelID := fmt.Sprintf("%s:%s", cliName, model)

	// Low consumption mode: override model if configured
	if s.config.LowConsumptionMode {
		def := s.loadAgentDefinition(req.AgentType, req.ProjectID, req.WorkflowName)
		if def != nil && def.LowConsumptionModel != "" {
			model = def.LowConsumptionModel
			cliName = s.cliForModel(model)
			modelID = fmt.Sprintf("%s:%s", cliName, model)
			logger.Info(ctx, "low consumption model override", "agent", req.AgentType, "model", modelID)
		}
	}

	// Log spawn info
	spawnTarget := req.TicketID
	if req.IsProjectScope() {
		spawnTarget = "project:" + req.ProjectID
	}
	logger.Info(ctx, "spawning agent", "agent_type", req.AgentType, "target", spawnTarget, "model", modelID, "workflow", req.WorkflowName, "layer", phase.Layer)

	// Spawn agent
	proc, err := s.spawnSingle(ctx, req, modelID, phase.NodeID, wi.ID)
	if err != nil {
		return fmt.Errorf("failed to spawn %s: %w", modelID, err)
	}
	if proc.backend == nil {
		return fmt.Errorf("internal: spawned proc has nil backend")
	}
	proc.trx = logger.TrxFromContext(ctx)
	pid := proc.pid
	if proc.cmd != nil && proc.cmd.Process != nil {
		pid = proc.cmd.Process.Pid
	}
	logger.Info(ctx, "agent process started", "model", modelID, "pid", pid, "session_id", proc.sessionID, "backend", proc.backend.Name())
	processes := []*processInfo{proc}

	// Monitor all processes
	return s.monitorAll(ctx, processes, req, phase.NodeID)
}

// spawnSingle spawns a single agent: prep -> backend.Start -> register.
func (s *Spawner) spawnSingle(ctx context.Context, req SpawnRequest, modelID, phase, wfiID string) (*processInfo, error) {
	proc, prep, err := s.prepareSpawn(ctx, req, modelID, phase, wfiID)
	if err != nil {
		return nil, err
	}
	if err := s.startBackend(proc, prep); err != nil {
		return nil, err
	}
	return proc, nil
}
