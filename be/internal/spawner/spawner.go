package spawner

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// WorkflowDef represents a workflow definition (copied from cli for decoupling)
type WorkflowDef struct {
	Description string     `json:"description"`
	ScopeType   string     `json:"scope_type"` // "ticket" or "project"
	Phases      []PhaseDef `json:"phases"`
}

// PhaseDef represents a phase definition
type PhaseDef struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
	Layer int    `json:"layer"`
}

// AgentConfig holds agent-specific configuration
type AgentConfig struct {
	Model   string `json:"model"`
	Timeout int    `json:"timeout"`
}

// ErrorRecorder records error events. Implemented by service.ErrorService.
type ErrorRecorder interface {
	RecordError(projectID, errorType, instanceID, message string) error
}

const (
	defaultMaxContinuations    = 10
	defaultContextThreshold    = 25
	defaultFailRetryDelay      = 15 * time.Second
	defaultStallStartTimeout   = 2 * time.Minute
	defaultStallRunningTimeout = 8 * time.Minute
	maxStallRestarts           = 15
)

// ModelConfig holds DB-sourced model configuration for the spawner.
// Zero values mean "not configured" — adapters fall back to their hardcoded methods.
type ModelConfig struct {
	CLIType         string // "claude", "opencode", "codex"
	MappedModel     string // actual CLI arg: "opus[1m]", "gpt-5.3-codex"
	ReasoningEffort string // "", "high", "medium"
	ContextLength   int    // 200000, 1000000
}

// Config holds the spawner configuration
type Config struct {
	Workflows   map[string]WorkflowDef
	Agents      map[string]AgentConfig
	DataPath    string
	ProjectRoot string
	// Spawner behavior settings
	TimeoutGraceSec        int // Grace period for SIGTERM before SIGKILL (default: 5)
	MessageFlushIntervalMs int // Interval between message flushes (default: 2000)
	// WebSocket hub for real-time updates (optional)
	WSHub *ws.Hub
	// Shared database connection pool (optional, falls back to DataPath per-call opens)
	Pool *db.Pool
	// Clock for timestamp generation (required)
	Clock clock.Clock
	// LowConsumptionMode enables model override via LowConsumptionModel
	LowConsumptionMode bool
	// ContextSaveViaAgent enables the system-agent context saver instead of resume-based save.
	// false (default) = resume-based save (Claude CLI only, other CLIs skip save)
	// true = spawn context-saver system agent (works for all CLI types)
	ContextSaveViaAgent bool
	// GlobalStallStartTimeout overrides the default stall start timeout when agent def has no value.
	// nil = use hardcoded default, 0 = disabled, >0 = custom seconds.
	GlobalStallStartTimeout *int
	// GlobalStallRunningTimeout overrides the default stall running timeout when agent def has no value.
	// nil = use hardcoded default, 0 = disabled, >0 = custom seconds.
	GlobalStallRunningTimeout *int
	// ClaudeSettingsJSON is the --settings JSON for Claude CLI agents (safety hooks).
	// Empty string means no settings. Read once at workflow start from project config.
	ClaudeSettingsJSON string
	// ModelConfigs maps model name to DB-sourced config. When populated, the spawner
	// uses these for model mapping, reasoning effort, context length, and CLI type
	// instead of hardcoded adapter methods. nil map is safe (lookup returns zero value).
	ModelConfigs map[string]ModelConfig
	// ErrorSvc records agent errors (optional, nil-safe).
	ErrorSvc ErrorRecorder
}

// taskInfo tracks an in-flight Task/Agent tool invocation for tool_result correlation
type taskInfo struct {
	toolName     string // original tool name ("Task" or "Agent")
	description  string
	subagentType string
	background   bool
}

// processInfo tracks a single spawned agent process
type processInfo struct {
	cmd           *exec.Cmd
	agentID       string
	agentType     string
	modelID       string
	sessionID     string
	startTime     time.Time
	timeout       time.Duration
	pendingMessages []repo.MessageEntry // messages not yet flushed to DB
	lastMessage     string              // most recent message (for status display)
	nextSeq         int                 // next sequence number for agent_messages table
	messagesMutex   sync.Mutex
	pendingTasks    map[string]taskInfo  // tool_use_id -> taskInfo for in-flight Task invocations
	finalStatus   string
	elapsed       time.Duration
	// Process lifecycle tracking
	doneCh  chan struct{} // closed when process exits
	waitErr error         // stores Wait() error
	// Message buffering
	messagesDirty     bool
	lastMessagesFlush time.Time
	// Context tracking
	contextLeft int
	maxContext  int
	// Spawn context (for debugging/replay)
	spawnCommand  string
	promptContext string
	// Request context (for broadcasting)
	projectID          string
	ticketID           string
	workflowName       string
	workflowInstanceID string
	// Continuation tracking
	ancestorSessionID string // Root session in a continuation chain
	restartCount      int    // How many times this agent has been restarted for low context
	restartThreshold  int    // Effective context threshold for this agent (percentage remaining)
	maxFailRestarts   int    // Max auto-restarts on failure (0 = disabled)
	failRestartCount  int    // How many times this agent has been auto-restarted on failure
	// Low-context save state
	lowContextSaving bool // True while initiateContextSave is running
	// Stall detection
	lastMessageTime     time.Time     // set on spawn, updated on every trackMessage()
	hasReceivedMessage  bool          // distinguishes "no messages yet" from "had messages, now stalled"
	stallStartTimeout   time.Duration // from agent_definition or default 120s
	stallRunningTimeout time.Duration // from agent_definition or default 480s
	stallRestartCount   int           // incremented on each stall restart
	// External session ID (e.g., codex thread_id) — for logging only
	externalSessionID string
	// Transaction ID for structured logging (from orchestrator context)
	trx string
}

// Spawner manages agent lifecycle
type Spawner struct {
	config           Config
	restartCh        chan string            // carries sessionID of agent to restart
	takeControlCh    chan string            // carries sessionID of agent to take control of
	interactiveWaits map[string]chan struct{} // sessionID → closed when interactive session completes
	mu               sync.Mutex              // protects interactiveWaits
}

// SpawnRequest contains parameters for spawning an agent
type SpawnRequest struct {
	AgentType          string
	TicketID           string
	ProjectID          string
	WorkflowName       string
	ParentSession      string
	CLIName            string
	ScopeType          string            // "ticket" (default) or "project"
	WorkflowInstanceID string            // when set, used directly instead of DB lookup
	ExtraVars          map[string]string  // Additional template variables (e.g., BRANCH_NAME, DEFAULT_BRANCH)
}

// IsProjectScope returns true if this is a project-scoped spawn request
func (r SpawnRequest) IsProjectScope() bool {
	return r.ScopeType == "project"
}

// New creates a new spawner
func New(config Config) *Spawner {
	return &Spawner{
		config:           config,
		restartCh:        make(chan string, 1),
		takeControlCh:    make(chan string, 1),
		interactiveWaits: make(map[string]chan struct{}),
	}
}

// RequestRestart sends a restart signal for the given session ID.
// Non-blocking: if a restart is already pending, this is a no-op.
func (s *Spawner) RequestRestart(sessionID string) {
	select {
	case s.restartCh <- sessionID:
	default:
	}
}

// RequestTakeControl sends a take-control signal for the given session ID.
// Non-blocking: if a take-control is already pending, this is a no-op.
func (s *Spawner) RequestTakeControl(sessionID string) {
	select {
	case s.takeControlCh <- sessionID:
	default:
	}
}

// CompleteInteractive signals that the interactive session has ended,
// unblocking the spawner's monitorAll wait.
func (s *Spawner) CompleteInteractive(sessionID string) {
	s.mu.Lock()
	ch, ok := s.interactiveWaits[sessionID]
	s.mu.Unlock()
	if ok {
		select {
		case <-ch:
			// already closed
		default:
			close(ch)
		}
	}
}

// RegisterInteractiveWait creates and returns a channel that blocks until
// CompleteInteractive is called for the given session ID. Used by the
// orchestrator to wait on interactive/plan PTY sessions before entering
// the layer execution loop.
func (s *Spawner) RegisterInteractiveWait(sessionID string) <-chan struct{} {
	ch := make(chan struct{})
	s.mu.Lock()
	s.interactiveWaits[sessionID] = ch
	s.mu.Unlock()
	return ch
}

// Close is a no-op retained for API compatibility (e.g. orchestrator defer).
func (s *Spawner) Close() {}

// pool returns the shared connection pool, or nil if not configured.
func (s *Spawner) pool() *db.Pool {
	if s.config.Pool != nil {
		return s.config.Pool
	}
	return nil
}

// broadcast sends a WebSocket event via the in-process hub
func (s *Spawner) broadcast(eventType, projectID, ticketID, workflow string, data map[string]interface{}) {
	if s.config.WSHub == nil {
		logger.Warn(context.Background(), "broadcast skipped: no WebSocket hub configured")
		return
	}
	event := ws.NewEvent(eventType, projectID, ticketID, workflow, data)
	s.config.WSHub.Broadcast(event)
}

// logAgent logs an INFO-level agent message with the agent's trx and prefix.
func (s *Spawner) logAgent(proc *processInfo, msg string) {
	ctx := logger.WithTrx(context.Background(), proc.trx)
	logger.Info(ctx, s.formatPrefix(proc)+" "+msg)
}

// warnAgent logs a WARN-level agent message with the agent's trx and prefix.
func (s *Spawner) warnAgent(proc *processInfo, msg string) {
	ctx := logger.WithTrx(context.Background(), proc.trx)
	logger.Warn(ctx, s.formatPrefix(proc)+" "+msg)
}

// errorAgent logs an ERROR-level agent message with the agent's trx and prefix.
func (s *Spawner) errorAgent(proc *processInfo, msg string) {
	ctx := logger.WithTrx(context.Background(), proc.trx)
	logger.Error(ctx, s.formatPrefix(proc)+" "+msg)
}

// waitBeforeRetry waits for defaultFailRetryDelay before retrying a failed/timed-out agent.
// Returns true if the wait completed, false if the context was cancelled (should not retry).
// Broadcasts an agent.retry_waiting event before sleeping.
func (s *Spawner) waitBeforeRetry(ctx context.Context, proc *processInfo) bool {
	s.broadcast(ws.EventAgentRetryWaiting, proc.projectID, proc.ticketID, proc.workflowName, map[string]interface{}{
		"agent_type":         proc.agentType,
		"session_id":         proc.sessionID,
		"model_id":           proc.modelID,
		"delay_seconds":      int(defaultFailRetryDelay.Seconds()),
		"fail_restart_count": proc.failRestartCount,
		"max_fail_restarts":  proc.maxFailRestarts,
	})
	logger.Info(ctx, "waiting before fail-restart", "delay", defaultFailRetryDelay, "model", proc.modelID)
	select {
	case <-ctx.Done():
		return false
	case <-time.After(defaultFailRetryDelay):
		return true
	}
}

// Spawn spawns agents for a phase with context cancellation support.
func (s *Spawner) Spawn(ctx context.Context, req SpawnRequest) error {
	// Validate workflow
	workflow, ok := s.config.Workflows[req.WorkflowName]
	if !ok {
		return fmt.Errorf("unknown workflow: %s", req.WorkflowName)
	}

	// Find phase for agent
	var phase *PhaseDef
	for i := range workflow.Phases {
		if workflow.Phases[i].Agent == req.AgentType {
			phase = &workflow.Phases[i]
			break
		}
	}
	if phase == nil {
		return fmt.Errorf("agent type '%s' not found in workflow '%s'", req.AgentType, req.WorkflowName)
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
	if _, err := s.validateAndAdvancePhase(wi, req.WorkflowName, req.AgentType); err != nil {
		return err
	}

	// Determine model to spawn (single agent per Spawn call)
	model := "opus"
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
	proc, err := s.spawnSingle(req, modelID, phase.ID, wi.ID)
	if err != nil {
		return fmt.Errorf("failed to spawn %s: %w", modelID, err)
	}
	proc.trx = logger.TrxFromContext(ctx)
	logger.Info(ctx, "agent process started", "model", modelID, "pid", proc.cmd.Process.Pid, "session_id", proc.sessionID)
	processes := []*processInfo{proc}

	// Monitor all processes
	return s.monitorAll(ctx, processes, req, phase.ID)
}

// spawnSingle spawns a single agent process using the appropriate CLI adapter
func (s *Spawner) spawnSingle(req SpawnRequest, modelID, phase, wfiID string) (*processInfo, error) {
	agentID := "spawn-" + uuid.New().String()[:8]
	sessionID := uuid.New().String()

	// Parse modelID (cli:model format)
	cliName, model := parseModelID(modelID)
	if cliName == "" {
		cliName = s.cliForModel(model)
		modelID = fmt.Sprintf("%s:%s", cliName, model)
	}

	// Get CLI adapter
	adapter, err := GetCLIAdapter(cliName)
	if err != nil {
		return nil, err
	}

	// Get agent config for timeout lookup
	timeout := 40 // minutes
	if agentCfg, ok := s.config.Agents[req.AgentType]; ok {
		if agentCfg.Timeout > 0 {
			timeout = agentCfg.Timeout
		}
	}

	// Load agent definition to get per-agent restart threshold and fail restart limit
	effectiveThreshold := defaultContextThreshold
	maxFailRestarts := 0
	agentDef := s.loadAgentDefinition(req.AgentType, req.ProjectID, req.WorkflowName)
	if agentDef != nil && agentDef.RestartThreshold != nil {
		effectiveThreshold = *agentDef.RestartThreshold
	}
	if agentDef != nil && agentDef.MaxFailRestarts != nil {
		maxFailRestarts = *agentDef.MaxFailRestarts
	}
	stallStartTimeout := defaultStallStartTimeout
	stallRunningTimeout := defaultStallRunningTimeout
	if agentDef != nil && agentDef.StallStartTimeoutSec != nil {
		if *agentDef.StallStartTimeoutSec == 0 {
			stallStartTimeout = 0
		} else {
			stallStartTimeout = time.Duration(*agentDef.StallStartTimeoutSec) * time.Second
		}
	} else if s.config.GlobalStallStartTimeout != nil {
		if *s.config.GlobalStallStartTimeout == 0 {
			stallStartTimeout = 0
		} else {
			stallStartTimeout = time.Duration(*s.config.GlobalStallStartTimeout) * time.Second
		}
	}
	if agentDef != nil && agentDef.StallRunningTimeoutSec != nil {
		if *agentDef.StallRunningTimeoutSec == 0 {
			stallRunningTimeout = 0
		} else {
			stallRunningTimeout = time.Duration(*agentDef.StallRunningTimeoutSec) * time.Second
		}
	} else if s.config.GlobalStallRunningTimeout != nil {
		if *s.config.GlobalStallRunningTimeout == 0 {
			stallRunningTimeout = 0
		} else {
			stallRunningTimeout = time.Duration(*s.config.GlobalStallRunningTimeout) * time.Second
		}
	}

	// Load agent template
	prompt, err := s.loadTemplate(req.AgentType, req.TicketID, req.ProjectID, req.ParentSession, sessionID, req.WorkflowName, modelID, phase, req.WorkflowInstanceID, req.ExtraVars)
	if err != nil {
		return nil, fmt.Errorf("failed to load template: %w", err)
	}

	// Write prompt to temp file
	filePrefix := req.TicketID
	if req.IsProjectScope() {
		filePrefix = "project-" + req.ProjectID
	}
	safePrefix := strings.ReplaceAll(filePrefix, "/", "_")
	safePrefix = strings.ReplaceAll(safePrefix, "\\", "_")
	promptFile, err := os.CreateTemp("/tmp/nrflow", fmt.Sprintf("%s-%s-*.md", safePrefix, req.AgentType))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := promptFile.WriteString(prompt); err != nil {
		os.Remove(promptFile.Name())
		return nil, fmt.Errorf("failed to write prompt: %w", err)
	}
	promptFile.Close()

	// Initial prompt (skipped for stdin-based adapters — the template IS the full prompt)
	var initialPrompt string
	if !adapter.UsesStdinPrompt() {
		if req.IsProjectScope() {
			initialPrompt = fmt.Sprintf(`Begin working on project %s. Follow the workflow steps in your system prompt.`, req.ProjectID)
		} else {
			initialPrompt = fmt.Sprintf(`Begin working on ticket %s. Follow the workflow steps in your system prompt.`, req.TicketID)
		}
	}

	// Prepare spawn options
	workDir := s.config.ProjectRoot
	if workDir == "" || workDir == "." {
		workDir = ""
	}

	// Look up DB-sourced model config for mapped model and reasoning effort
	var mappedModel, reasoningEffort string
	if cfg, ok := s.config.ModelConfigs[model]; ok {
		mappedModel = cfg.MappedModel
		reasoningEffort = cfg.ReasoningEffort
	}

	opts := SpawnOptions{
		Model:           model,
		SessionID:       sessionID,
		PromptFile:      promptFile.Name(),
		Prompt:          prompt,
		InitialPrompt:   initialPrompt,
		WorkDir:         workDir,
		MappedModel:     mappedModel,
		ReasoningEffort: reasoningEffort,
		SettingsJSON:    s.config.ClaudeSettingsJSON,
		Env: append(filterEnv(os.Environ(), "CLAUDECODE"),
			fmt.Sprintf("NRFLOW_PROJECT=%s", req.ProjectID),
			fmt.Sprintf("NRF_WORKFLOW_INSTANCE_ID=%s", wfiID),
			fmt.Sprintf("NRF_SESSION_ID=%s", sessionID),
			"NRF_SPAWNED=1",
			fmt.Sprintf("NRF_CONTEXT_THRESHOLD=%d", 100-effectiveThreshold),
			fmt.Sprintf("NRF_MAX_CONTEXT=%d", s.maxContextForModel(model)),
		),
	}

	// Build command using adapter
	cmd := adapter.BuildCommand(opts)

	// For stdin-based adapters, pipe the prompt file to stdin
	var stdinFile *os.File
	if adapter.UsesStdinPrompt() {
		stdinFile, err = os.Open(promptFile.Name())
		if err != nil {
			os.Remove(promptFile.Name())
			return nil, fmt.Errorf("failed to open prompt file for stdin: %w", err)
		}
		cmd.Stdin = stdinFile
	}

	// Capture spawn command for debugging/replay — prepend nrflow env vars
	// so the recorded command is fully reproducible.
	var envParts []string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "NRF_") || strings.HasPrefix(e, "NRFLOW_") {
			envParts = append(envParts, e)
		}
	}
	spawnCommand := strings.Join(cmd.Args, " ")
	if adapter.UsesStdinPrompt() {
		spawnCommand += " < " + promptFile.Name()
	}
	if len(envParts) > 0 {
		spawnCommand = strings.Join(envParts, " ") + " " + spawnCommand
	}

	// Create pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.Remove(promptFile.Name())
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		os.Remove(promptFile.Name())
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start process
	if err := cmd.Start(); err != nil {
		if stdinFile != nil {
			stdinFile.Close()
		}
		os.Remove(promptFile.Name())
		return nil, fmt.Errorf("failed to start agent: %w", err)
	}
	// Close stdin file — child process has its own fd copy
	if stdinFile != nil {
		stdinFile.Close()
	}

	// Create process info
	_, modelName := parseModelID(modelID)
	proc := &processInfo{
		cmd:            cmd,
		agentID:        agentID,
		agentType:      req.AgentType,
		modelID:        modelID,
		sessionID:      sessionID,
		startTime:      s.config.Clock.Now(),
		timeout:        time.Duration(timeout) * time.Minute,
		pendingMessages:   make([]repo.MessageEntry, 0),
		pendingTasks:      make(map[string]taskInfo),
		doneCh:            make(chan struct{}),
		lastMessagesFlush: s.config.Clock.Now(),
		spawnCommand:   spawnCommand,
		promptContext:  prompt,
		projectID:     req.ProjectID,
		ticketID:      req.TicketID,
		workflowName:       req.WorkflowName,
		workflowInstanceID: wfiID,
		restartThreshold:    effectiveThreshold,
		maxFailRestarts:     maxFailRestarts,
		lastMessageTime:     s.config.Clock.Now(),
		stallStartTimeout:   stallStartTimeout,
		stallRunningTimeout: stallRunningTimeout,
		maxContext:          s.maxContextForModel(modelName),
	}

	// Register agent start (create agent_sessions row)
	s.registerAgentStart(req.ProjectID, req.TicketID, req.WorkflowName, wfiID, agentID, req.AgentType, cmd.Process.Pid, sessionID, modelID, phase, spawnCommand, prompt, "", 0, effectiveThreshold)

	// Start output monitoring goroutines
	go s.monitorOutput(proc, stdout)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			s.warnAgent(proc, "[stderr] "+line)
			s.trackMessage(proc, "[stderr] "+line, "text")
		}
	}()

	// Single wait goroutine - closes doneCh when process exits.
	// Capture doneCh locally: proc.doneCh may be replaced during low-context save.
	origDoneCh := proc.doneCh
	go func() {
		proc.waitErr = cmd.Wait()
		close(origDoneCh)
		os.Remove(promptFile.Name())
	}()

	return proc, nil
}

// monitorAll monitors all spawned processes until completion.
func (s *Spawner) monitorAll(ctx context.Context, processes []*processInfo, req SpawnRequest, phase string) error {
	const statusInterval = 30 * time.Second
	lastStatusTime := time.Time{}

	running := make([]*processInfo, len(processes))
	copy(running, processes)
	var completed []*processInfo

	for len(running) > 0 {
		// Check for context cancellation or manual restart signal
		select {
		case <-ctx.Done():
			// Kill all running processes
			logger.Warn(ctx, "agents cancelled", "count", len(running))
			for _, proc := range running {
				if proc.cmd.Process != nil {
					proc.cmd.Process.Signal(syscall.SIGTERM)
				}
			}
			// Wait briefly for graceful shutdown
			time.Sleep(2 * time.Second)
			for _, proc := range running {
				select {
				case <-proc.doneCh:
				default:
					if proc.cmd.Process != nil {
						proc.cmd.Process.Kill()
					}
					<-proc.doneCh
				}
				proc.finalStatus = "CANCELLED"
				s.saveMessages(proc)
				s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
					proc.sessionID, proc.agentID, "fail", "cancelled", proc.modelID)
				completed = append(completed, proc)
			}
			return ctx.Err()
		case restartSessionID := <-s.restartCh:
			// Manual restart requested — find matching proc and initiate context save
			for _, proc := range running {
				if proc.sessionID == restartSessionID && !proc.lowContextSaving {
					logger.Info(ctx, "manual restart requested", "session_id", restartSessionID)
					proc.lowContextSaving = true
					oldDoneCh := proc.doneCh
					newDoneCh := make(chan struct{})
					proc.doneCh = newDoneCh
					go s.initiateContextSave(ctx, proc, req, oldDoneCh, newDoneCh)
					break
				}
			}
		case takeControlSessionID := <-s.takeControlCh:
			// Take-control requested — find matching proc, validate, kill, and block
			for i, proc := range running {
				if proc.sessionID != takeControlSessionID {
					continue
				}
				// Validate CLI supports resume
				cliName, _ := parseModelID(proc.modelID)
				adapter, adapterErr := GetCLIAdapter(cliName)
				if adapterErr != nil || !adapter.SupportsResume() {
					logger.Error(ctx, "take-control: CLI does not support resume", "cli", cliName, "session_id", takeControlSessionID)
					break
				}

				logger.Info(ctx, "take-control: killing agent", "session_id", takeControlSessionID)

				// Kill process: SIGTERM → grace → SIGKILL
				if proc.cmd.Process != nil {
					proc.cmd.Process.Signal(syscall.SIGTERM)
				}
				gracePeriod := time.Duration(s.config.TimeoutGraceSec) * time.Second
				if gracePeriod == 0 {
					gracePeriod = 5 * time.Second
				}
				select {
				case <-proc.doneCh:
				case <-time.After(gracePeriod):
					if proc.cmd.Process != nil {
						proc.cmd.Process.Kill()
					}
					<-proc.doneCh
				}

				// Flush messages and register stop
				s.saveMessages(proc)
				s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
					proc.sessionID, proc.agentID, "user_interactive", "take_control", proc.modelID)

				// Broadcast take-control event
				s.broadcast(ws.EventAgentTakeControl, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
					"session_id": proc.sessionID,
					"agent_type": proc.agentType,
					"model_id":   proc.modelID,
				})

				// Remove from running
				running = append(running[:i], running[i+1:]...)

				// Create interactive wait channel and block until interactive session completes
				waitCh := make(chan struct{})
				s.mu.Lock()
				s.interactiveWaits[proc.sessionID] = waitCh
				s.mu.Unlock()

				logger.Info(ctx, "take-control: waiting for interactive session to complete", "session_id", takeControlSessionID)
				select {
				case <-waitCh:
					logger.Info(ctx, "take-control: interactive session completed", "session_id", takeControlSessionID)
				case <-ctx.Done():
					logger.Warn(ctx, "take-control: cancelled while waiting for interactive session", "session_id", takeControlSessionID)
				}

				s.mu.Lock()
				delete(s.interactiveWaits, proc.sessionID)
				s.mu.Unlock()

				// Mark as PASS so finalizePhase proceeds
				proc.finalStatus = "PASS"
				proc.elapsed = time.Since(proc.startTime)
				completed = append(completed, proc)
				break
			}
		default:
		}

		now := time.Now()

		// Print status every interval
		if now.Sub(lastStatusTime) >= statusInterval {
			s.printStatus(running, completed, phase)
			lastStatusTime = now
		}

		// Read context_left from DB once per iteration
		readContextLeftFromDB(s.pool(), running)

		// Check each process using doneCh (no double-wait bug)
		var stillRunning []*processInfo
		for _, proc := range running {
			elapsed := time.Since(proc.startTime)

			// Detect low context and initiate save (works for all CLI types)
			if !proc.lowContextSaving && proc.contextLeft > 0 && proc.contextLeft <= proc.restartThreshold {
				proc.lowContextSaving = true
				// Replace doneCh — initiateContextSave will close the new one when the full flow completes
				oldDoneCh := proc.doneCh
				newDoneCh := make(chan struct{})
				proc.doneCh = newDoneCh
				go s.initiateContextSave(ctx, proc, req, oldDoneCh, newDoneCh)
			}

			select {
			case <-proc.doneCh:
				// Process exited
				proc.elapsed = elapsed
				proc.lowContextSaving = false

				// If context save already set finalStatus, skip handleCompletion
				if proc.finalStatus == "" {
					s.handleCompletion(ctx, proc, req)
				}

				// Instant stall detection (exit 0 but too fast with minimal output)
				instantStallDetected := false
				if proc.finalStatus == "PASS" {
					s.checkInstantStall(ctx, proc, req)
					instantStallDetected = proc.finalStatus == "CONTINUE"
				}

				// Wait before instant stall restart
				if instantStallDetected {
					if !s.waitBeforeStallRetry(ctx, proc, req) {
						completed = append(completed, proc)
						continue
					}
				}

				// Auto-restart failed agent if configured
				if proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts {
					if s.waitBeforeRetry(ctx, proc) {
						logger.Info(ctx, "auto-restarting failed agent", "model", proc.modelID,
							"fail_restart_count", proc.failRestartCount+1, "max", proc.maxFailRestarts)
						// Override the already-registered failed session to continued/fail_restart
						if pool := s.pool(); pool != nil {
							sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
							sessionRepo.UpdateResult(proc.sessionID, "continue", "fail_restart")
							sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionContinued)
						}
						proc.failRestartCount++
						proc.finalStatus = "CONTINUE"
					}
				}

				// Check for continuation
				if proc.finalStatus == "CONTINUE" {
					if proc.restartCount < defaultMaxContinuations {
						logger.Info(ctx, "continuation relaunching", "model", proc.modelID, "count", proc.restartCount+1, "max", defaultMaxContinuations)
						newProc, err := s.relaunchForContinuation(ctx, proc, req, phase)
						if err != nil {
							logger.Error(ctx, "failed to relaunch", "model", proc.modelID, "err", err)
							completed = append(completed, proc)
						} else {
							stillRunning = append(stillRunning, newProc)
						}
					} else {
						logger.Error(ctx, "max continuations reached", "model", proc.modelID, "max", defaultMaxContinuations)
						proc.finalStatus = "FAIL"
						s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
							proc.sessionID, proc.agentID, "fail", "max_continuations", proc.modelID)
						completed = append(completed, proc)
					}
				} else {
					completed = append(completed, proc)
				}
			default:
				// Stall detection — check before timeout
				if s.checkStall(ctx, proc, req) {
					proc.elapsed = elapsed
					// checkStall already killed the process and set finalStatus=CONTINUE
					// Wait before relaunching
					if !s.waitBeforeStallRetry(ctx, proc, req) {
						completed = append(completed, proc)
						continue
					}
					if proc.restartCount < defaultMaxContinuations {
						newProc, err := s.relaunchForContinuation(ctx, proc, req, phase)
						if err != nil {
							logger.Error(ctx, "failed to relaunch after stall", "model", proc.modelID, "err", err)
							completed = append(completed, proc)
						} else {
							stillRunning = append(stillRunning, newProc)
						}
					} else {
						logger.Error(ctx, "max continuations reached after stall", "model", proc.modelID)
						proc.finalStatus = "FAIL"
						completed = append(completed, proc)
					}
					continue
				}
				// Still running - check timeout
				if elapsed > proc.timeout {
					s.handleGracefulTimeout(ctx, proc, req)
					// Auto-restart timed-out agent if configured
					if proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts {
						if !s.waitBeforeRetry(ctx, proc) {
							completed = append(completed, proc)
						} else {
							logger.Info(ctx, "auto-restarting timed-out agent", "model", proc.modelID,
								"fail_restart_count", proc.failRestartCount+1, "max", proc.maxFailRestarts)
							if pool := s.pool(); pool != nil {
								sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
								sessionRepo.UpdateResult(proc.sessionID, "continue", "timeout_restart")
								sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionContinued)
							}
							proc.failRestartCount++
							proc.finalStatus = "CONTINUE"
							newProc, err := s.relaunchForContinuation(ctx, proc, req, phase)
							if err != nil {
								logger.Error(ctx, "failed to relaunch after timeout", "model", proc.modelID, "err", err)
								completed = append(completed, proc)
							} else {
								stillRunning = append(stillRunning, newProc)
							}
						}
					} else {
						completed = append(completed, proc)
					}
				} else {
					stillRunning = append(stillRunning, proc)
					s.maybeFlushMessages(proc)
				}
			}
		}

		running = stillRunning
		if len(running) > 0 {
			time.Sleep(1 * time.Second)
		}
	}

	// Finalize phase
	return s.finalizePhase(ctx, completed, req, phase)
}

// printStatus logs status for all running/completed agents
func (s *Spawner) printStatus(running, completed []*processInfo, phase string) {
	for _, proc := range running {
		elapsed := time.Since(proc.startTime).Round(time.Second)

		proc.messagesMutex.Lock()
		lastMsg := proc.lastMessage
		proc.messagesMutex.Unlock()
		if lastMsg != "" {
			if len(lastMsg) > 80 {
				lastMsg = lastMsg[:77] + "..."
			}
		}

		ctx := logger.WithTrx(context.Background(), proc.trx)
		logger.Info(ctx, "agent status", "phase", phase, "model", proc.modelID, "elapsed", elapsed, "last_msg", lastMsg)
	}

	for _, proc := range completed {
		ctx := logger.WithTrx(context.Background(), proc.trx)
		logger.Info(ctx, "agent status", "phase", phase, "model", proc.modelID, "status", proc.finalStatus, "duration", proc.elapsed.Round(time.Second))
	}
}

// finalizePhase completes the phase after all agents finish.
// Uses pass_count >= 1 semantics: at least one PASS is required for layer success.
// All-skipped counts as success (continue to next layer).
// Returns CallbackError if any agent completed with CALLBACK status.
func (s *Spawner) finalizePhase(ctx context.Context, completed []*processInfo, req SpawnRequest, phase string) error {
	// Clean up coalescing map entries for completed sessions
	cleanupBroadcastCoalescing(completed)

	for _, proc := range completed {
		logger.Info(ctx, "agent result", "phase", phase, "model", proc.modelID, "status", proc.finalStatus, "duration", proc.elapsed.Round(time.Second))
	}

	passCount := 0
	skippedCount := 0
	callbackCount := 0
	var callbackProc *processInfo
	for _, proc := range completed {
		switch proc.finalStatus {
		case "PASS":
			passCount++
		case "SKIPPED":
			skippedCount++
		case "CALLBACK":
			callbackCount++
			// Track the callback proc (if multiple, we'll pick lowest level in orchestrator)
			callbackProc = proc
		}
	}

	// Callback detected — read callback_level from session findings and signal orchestrator
	if callbackCount > 0 {
		level, instructions := s.readCallbackFindings(callbackProc)
		logger.Info(ctx, "phase finalized", "phase", phase, "result", "CALLBACK", "callback_level", level)
		return &CallbackError{Level: level, Instructions: instructions, AgentType: req.AgentType}
	}

	// All skipped = success (continue to next layer)
	if skippedCount == len(completed) {
		logger.Info(ctx, "phase finalized", "phase", phase, "result", "SKIPPED")
		return nil
	}

	// At least one pass = success
	if passCount >= 1 {
		logger.Info(ctx, "phase finalized", "phase", phase, "result", "PASS", "pass_count", passCount, "total", len(completed))
		return nil
	}

	// No passes = fail

	var failedModels []string
	for _, proc := range completed {
		if proc.finalStatus != "PASS" && proc.finalStatus != "SKIPPED" {
			failedModels = append(failedModels, proc.modelID)
		}
	}
	logger.Error(ctx, "phase finalized", "phase", phase, "result", "FAIL", "failed", strings.Join(failedModels, ", "))
	return fmt.Errorf("phase %s failed", phase)
}

// readCallbackFindings reads callback_level and callback_instructions from agent session findings.
func (s *Spawner) readCallbackFindings(proc *processInfo) (int, string) {
	pool := s.pool()
	if pool == nil {
		return 0, ""
	}

	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	session, err := sessionRepo.Get(proc.sessionID)
	if err != nil {
		return 0, ""
	}

	findings := session.GetFindings()
	level := 0
	if lvl, ok := findings["callback_level"]; ok {
		switch v := lvl.(type) {
		case float64:
			level = int(v)
		case int:
			level = v
		}
	}

	instructions := ""
	if instr, ok := findings["callback_instructions"]; ok {
		if str, ok := instr.(string); ok {
			instructions = str
		}
	}

	return level, instructions
}

// filterEnv returns a copy of env with the named variable removed.
func filterEnv(env []string, name string) []string {
	prefix := name + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}

func (s *Spawner) maxContextForModel(model string) int {
	if cfg, ok := s.config.ModelConfigs[model]; ok && cfg.ContextLength > 0 {
		return cfg.ContextLength
	}
	if model == "opus_1m" {
		return 1000000
	}
	return 200000
}

// cliForModel returns the CLI name for a model, checking DB config first.
func (s *Spawner) cliForModel(model string) string {
	if cfg, ok := s.config.ModelConfigs[model]; ok && cfg.CLIType != "" {
		return cfg.CLIType
	}
	return DefaultCLIForModel(model)
}

func parseModelID(modelID string) (cli, model string) {
	if modelID == "" || !strings.Contains(modelID, ":") {
		return "claude", modelID
	}
	parts := strings.SplitN(modelID, ":", 2)
	return parts[0], parts[1]
}
