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

const (
	defaultMaxContinuations = 10
	defaultContextThreshold = 25
)

// Config holds the spawner configuration
type Config struct {
	Workflows   map[string]WorkflowDef
	Agents      map[string]AgentConfig
	DefaultCLI  string
	DataPath    string
	ProjectRoot string
	// Spawner behavior settings
	TimeoutGraceSec        int // Grace period for SIGTERM before SIGKILL (default: 5)
	CompletionGraceSec     int // Wait for explicit completion after exit 0 (default: 60)
	MessageFlushIntervalMs int // Interval between message flushes (default: 2000)
	// WebSocket hub for real-time updates (optional)
	WSHub *ws.Hub
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
	pendingMessages []string   // messages not yet flushed to DB
	lastMessage     string     // most recent message (for status display)
	nextSeq         int        // next sequence number for agent_messages table
	messagesMutex   sync.Mutex
	finalStatus   string
	elapsed       time.Duration
	// Process lifecycle tracking
	doneCh  chan struct{} // closed when process exits
	waitErr error         // stores Wait() error
	// Message buffering
	messagesDirty     bool
	lastMessagesFlush time.Time
	// Context tracking
	contextLeft      int
	contextLeftDirty bool
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
	// Low-context save state
	lowContextSaving bool // True while initiateContextSave is running
}

// Spawner manages agent lifecycle
type Spawner struct {
	config    Config
	restartCh chan string // carries sessionID of agent to restart
}

// SpawnRequest contains parameters for spawning an agent
type SpawnRequest struct {
	AgentType          string
	TicketID           string
	ProjectID          string
	WorkflowName       string
	ParentSession      string
	CLIName            string
	ScopeType          string // "ticket" (default) or "project"
	WorkflowInstanceID string // when set, used directly instead of DB lookup
}

// IsProjectScope returns true if this is a project-scoped spawn request
func (r SpawnRequest) IsProjectScope() bool {
	return r.ScopeType == "project"
}

// New creates a new spawner
func New(config Config) *Spawner {
	return &Spawner{
		config:    config,
		restartCh: make(chan string, 1),
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

// Close is a no-op retained for API compatibility (e.g. orchestrator defer).
func (s *Spawner) Close() {}

// broadcast sends a WebSocket event via the in-process hub
func (s *Spawner) broadcast(eventType, projectID, ticketID, workflow string, data map[string]interface{}) {
	if s.config.WSHub == nil {
		fmt.Fprintf(os.Stderr, "[ws] broadcast skipped: no WebSocket hub configured\n")
		return
	}
	event := ws.NewEvent(eventType, projectID, ticketID, workflow, data)
	s.config.WSHub.Broadcast(event)
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
	cliName := req.CLIName
	if cliName == "" {
		if s.config.DefaultCLI != "" {
			cliName = s.config.DefaultCLI
		} else {
			cliName = "claude"
		}
	}
	if agentCfg, ok := s.config.Agents[req.AgentType]; ok && agentCfg.Model != "" {
		model = agentCfg.Model
	}
	modelID := fmt.Sprintf("%s:%s", cliName, model)

	// Log spawn info
	spawnTarget := req.TicketID
	if req.IsProjectScope() {
		spawnTarget = "project:" + req.ProjectID
	}
	logger.Info(ctx, "spawning agent", "agent_type", req.AgentType, "target", spawnTarget, "model", modelID, "workflow", req.WorkflowName, "layer", phase.Layer)

	// Start phase
	s.startPhase(ctx, wi.ID, req.ProjectID, req.TicketID, req.WorkflowName, phase.ID)

	// Spawn agent
	proc, err := s.spawnSingle(req, modelID, phase.ID, wi.ID)
	if err != nil {
		s.completePhase(ctx, wi.ID, req.ProjectID, req.TicketID, req.WorkflowName, phase.ID, "fail")
		return fmt.Errorf("failed to spawn %s: %w", modelID, err)
	}
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
		if s.config.DefaultCLI != "" {
			cliName = s.config.DefaultCLI
		} else {
			cliName = "claude"
		}
		modelID = fmt.Sprintf("%s:%s", cliName, model)
	}

	// Get CLI adapter
	adapter, err := GetCLIAdapter(cliName)
	if err != nil {
		return nil, err
	}

	// Get agent config
	timeout := 40 // minutes
	if agentCfg, ok := s.config.Agents[req.AgentType]; ok {
		if agentCfg.Timeout > 0 {
			timeout = agentCfg.Timeout
		}
	}

	// Load agent definition to get per-agent restart threshold
	effectiveThreshold := defaultContextThreshold
	agentDef := s.loadAgentDefinition(req.AgentType, req.ProjectID, req.WorkflowName)
	if agentDef != nil && agentDef.RestartThreshold != nil {
		effectiveThreshold = *agentDef.RestartThreshold
	}

	// Load agent template
	prompt, err := s.loadTemplate(req.AgentType, req.TicketID, req.ProjectID, req.ParentSession, sessionID, req.WorkflowName, modelID, phase, req.WorkflowInstanceID)
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
	promptFile, err := os.CreateTemp("", fmt.Sprintf("%s-%s-*.md", safePrefix, req.AgentType))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := promptFile.WriteString(prompt); err != nil {
		os.Remove(promptFile.Name())
		return nil, fmt.Errorf("failed to write prompt: %w", err)
	}
	promptFile.Close()

	// Initial prompt
	var initialPrompt string
	if req.IsProjectScope() {
		initialPrompt = fmt.Sprintf(`Begin working on project %s. Follow the workflow steps in your system prompt.`, req.ProjectID)
	} else {
		initialPrompt = fmt.Sprintf(`Begin working on ticket %s. Follow the workflow steps in your system prompt.`, req.TicketID)
	}

	// Prepare spawn options
	workDir := s.config.ProjectRoot
	if workDir == "" || workDir == "." {
		workDir = ""
	}

	opts := SpawnOptions{
		Model:         model,
		SessionID:     sessionID,
		PromptFile:    promptFile.Name(),
		Prompt:        prompt,
		InitialPrompt: initialPrompt,
		WorkDir:       workDir,
		Env: append(os.Environ(),
			fmt.Sprintf("NRWORKFLOW_PROJECT=%s", req.ProjectID),
			"NRWF_SPAWNED=1",
			fmt.Sprintf("NRWF_CONTEXT_THRESHOLD=%d", 100-effectiveThreshold),
		),
	}

	// Build command using adapter
	cmd := adapter.BuildCommand(opts)

	// Capture spawn command for debugging/replay
	spawnCommand := strings.Join(cmd.Args, " ")

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
		os.Remove(promptFile.Name())
		return nil, fmt.Errorf("failed to start agent: %w", err)
	}

	// Create process info
	proc := &processInfo{
		cmd:            cmd,
		agentID:        agentID,
		agentType:      req.AgentType,
		modelID:        modelID,
		sessionID:      sessionID,
		startTime:      time.Now(),
		timeout:        time.Duration(timeout) * time.Minute,
		pendingMessages:   make([]string, 0),
		doneCh:            make(chan struct{}),
		lastMessagesFlush: time.Now(),
		spawnCommand:   spawnCommand,
		promptContext:  prompt,
		projectID:     req.ProjectID,
		ticketID:      req.TicketID,
		workflowName:       req.WorkflowName,
		workflowInstanceID: wfiID,
		restartThreshold:   effectiveThreshold,
	}

	// Register agent start (create agent_sessions row)
	s.registerAgentStart(req.ProjectID, req.TicketID, req.WorkflowName, wfiID, agentID, req.AgentType, cmd.Process.Pid, sessionID, modelID, phase, spawnCommand, prompt, "", 0, effectiveThreshold)

	// Start output monitoring goroutines
	go s.monitorOutput(proc, stdout)
	go func() {
		scanner := bufio.NewScanner(stderr)
		prefix := s.formatPrefix(proc)
		for scanner.Scan() {
			line := scanner.Text()
			// Display and track stderr for debugging
			fmt.Printf("  %s [stderr] %s\n", prefix, line)
			s.trackMessage(proc, "[stderr] "+line)
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
			wfiID := ""
			if len(completed) > 0 {
				wfiID = completed[0].workflowInstanceID
			}
			s.completePhase(ctx, wfiID, req.ProjectID, req.TicketID, req.WorkflowName, phase, "fail")
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
		default:
		}

		now := time.Now()

		// Print status every interval
		if now.Sub(lastStatusTime) >= statusInterval {
			s.printStatus(running, completed, phase)
			lastStatusTime = now
		}

		// Read context file once per iteration
		contextData := readContextFile()

		// Check each process using doneCh (no double-wait bug)
		var stillRunning []*processInfo
		for _, proc := range running {
			elapsed := time.Since(proc.startTime)

			// Update context tracking
			updateContextLeft(proc, contextData)

			// Detect low context and initiate save (only for CLIs that support resume)
			if !proc.lowContextSaving && proc.contextLeft > 0 && proc.contextLeft <= proc.restartThreshold {
				cliName, _ := parseModelID(proc.modelID)
				adapter, _ := GetCLIAdapter(cliName)
				if adapter != nil && adapter.SupportsResume() {
					proc.lowContextSaving = true
					// Replace doneCh — initiateContextSave will close the new one when the full flow completes
					oldDoneCh := proc.doneCh
					newDoneCh := make(chan struct{})
					proc.doneCh = newDoneCh
					go s.initiateContextSave(ctx, proc, req, oldDoneCh, newDoneCh)
				}
			}

			select {
			case <-proc.doneCh:
				// Process exited
				proc.elapsed = elapsed
				s.saveContextLeft(proc)
				proc.lowContextSaving = false

				// If context save already set finalStatus, skip handleCompletion
				if proc.finalStatus == "" {
					s.handleCompletion(ctx, proc, req)
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
				// Still running - check timeout
				if elapsed > proc.timeout {
					s.saveContextLeft(proc)
					s.handleGracefulTimeout(ctx, proc, req)
					completed = append(completed, proc)
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

// printStatus prints status for all running/completed agents
func (s *Spawner) printStatus(running, completed []*processInfo, phase string) {
	fmt.Printf("[%s] %d agent(s) running:\n", phase, len(running))

	for _, proc := range running {
		elapsed := time.Since(proc.startTime).Round(time.Second)

		proc.messagesMutex.Lock()
		lastMsg := proc.lastMessage
		proc.messagesMutex.Unlock()
		if lastMsg != "" {
			if len(lastMsg) > 80 {
				lastMsg = lastMsg[:77] + "..."
			}
			lastMsg = " | " + lastMsg
		}

		fmt.Printf("  %s: %v%s\n", proc.modelID, elapsed, lastMsg)
	}

	for _, proc := range completed {
		fmt.Printf("  (%s completed - %s, %v)\n", proc.modelID, proc.finalStatus, proc.elapsed.Round(time.Second))
	}

	fmt.Println()
}

// finalizePhase completes the phase after all agents finish.
// Uses pass_count >= 1 semantics: at least one PASS is required for layer success.
// All-skipped counts as success (continue to next layer).
// Returns CallbackError if any agent completed with CALLBACK status.
func (s *Spawner) finalizePhase(ctx context.Context, completed []*processInfo, req SpawnRequest, phase string) error {
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

	wfiID := ""
	if len(completed) > 0 {
		wfiID = completed[0].workflowInstanceID
	}

	// Callback detected — read callback_level from session findings and signal orchestrator
	if callbackCount > 0 {
		level, instructions := s.readCallbackFindings(callbackProc)
		s.completePhase(ctx, wfiID, req.ProjectID, req.TicketID, req.WorkflowName, phase, "callback")
		logger.Info(ctx, "phase finalized", "phase", phase, "result", "CALLBACK", "callback_level", level)
		return &CallbackError{Level: level, Instructions: instructions, AgentType: req.AgentType}
	}

	// All skipped = success (continue to next layer)
	if skippedCount == len(completed) {
		s.completePhase(ctx, wfiID, req.ProjectID, req.TicketID, req.WorkflowName, phase, "skipped")
		logger.Info(ctx, "phase finalized", "phase", phase, "result", "SKIPPED")
		return nil
	}

	// At least one pass = success
	if passCount >= 1 {
		s.completePhase(ctx, wfiID, req.ProjectID, req.TicketID, req.WorkflowName, phase, "pass")
		logger.Info(ctx, "phase finalized", "phase", phase, "result", "PASS", "pass_count", passCount, "total", len(completed))
		return nil
	}

	// No passes = fail
	s.completePhase(ctx, wfiID, req.ProjectID, req.TicketID, req.WorkflowName, phase, "fail")

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
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return 0, ""
	}
	defer database.Close()

	sessionRepo := repo.NewAgentSessionRepo(database)
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

func parseModelID(modelID string) (cli, model string) {
	if modelID == "" || !strings.Contains(modelID, ":") {
		return "claude", modelID
	}
	parts := strings.SplitN(modelID, ":", 2)
	return parts[0], parts[1]
}
