package spawner

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"nrworkflow/internal/db"
	"nrworkflow/internal/model"
	"nrworkflow/internal/repo"
	"nrworkflow/internal/service"
	"nrworkflow/internal/types"
	"nrworkflow/internal/ws"
)

// WorkflowDef represents a workflow definition (copied from cli for decoupling)
type WorkflowDef struct {
	Description string     `json:"description"`
	Categories  []string   `json:"categories"`
	Phases      []PhaseDef `json:"phases"`
}

// PhaseDef represents a phase definition
type PhaseDef struct {
	ID       string   `json:"id"`
	Agent    string   `json:"agent"`
	SkipFor  []string `json:"skip_for,omitempty"`
	Parallel *struct {
		Enabled bool     `json:"enabled"`
		Models  []string `json:"models"`
	} `json:"parallel,omitempty"`
}

// AgentConfig holds agent-specific configuration
type AgentConfig struct {
	Model   string `json:"model"`
	Timeout int    `json:"timeout"`
}

// FullConfig represents the complete config.json structure
type FullConfig struct {
	CLI struct {
		Default string `json:"default"`
	} `json:"cli"`
	Agents    map[string]AgentConfig `json:"agents"`
	Workflows map[string]WorkflowDef `json:"workflows"`
	Spawner   struct {
		MaxContinuations int `json:"max_continuations,omitempty"`
		ContextThreshold int `json:"context_threshold,omitempty"`
	} `json:"spawner,omitempty"`
}

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
	// Context continuation settings
	MaxContinuations int // Max times an agent can be continued (default: 3)
	ContextThreshold int // Context usage % at which to suggest continuation (default: 85)
	// WebSocket hub for real-time updates (optional)
	WSHub *ws.Hub
}

// getMaxContinuations returns the configured max continuations or default
func (c *Config) getMaxContinuations() int {
	if c.MaxContinuations > 0 {
		return c.MaxContinuations
	}
	return 3
}

// getContextThreshold returns the configured context threshold or default
func (c *Config) getContextThreshold() int {
	if c.ContextThreshold > 0 {
		return c.ContextThreshold
	}
	return 85
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
	continuationCount int    // How many times this agent has been continued
}

// Spawner manages agent lifecycle
type Spawner struct {
	config Config
}

// SpawnRequest contains parameters for spawning an agent
type SpawnRequest struct {
	AgentType     string
	TicketID      string
	ProjectID     string
	WorkflowName  string
	ParentSession string
	CLIName       string
}

// New creates a new spawner
func New(config Config) *Spawner {
	return &Spawner{
		config: config,
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
// If parallel is enabled, spawns all configured models concurrently.
// The context is checked during the monitor loop; on cancellation, running
// agent processes are killed and the phase is marked as failed.
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

	// Validate workflow is initialized on ticket
	wi, err := s.getWorkflowInstance(req.ProjectID, req.TicketID, req.WorkflowName)
	if err != nil {
		return err
	}

	// Validate phase order and auto-skip phases with matching skip_for category
	validatedPhaseID, shouldSkip, err := s.validateAndAdvancePhase(wi, req.WorkflowName, req.AgentType)
	if err != nil {
		return err
	}
	if shouldSkip {
		fmt.Printf("  Phase '%s' skipped due to category rules\n", validatedPhaseID)
		return nil
	}

	// Determine models to spawn
	var models []string
	if phase.Parallel != nil && phase.Parallel.Enabled && len(phase.Parallel.Models) > 0 {
		// Parallel enabled - spawn all configured models
		models = phase.Parallel.Models
	} else {
		// Single agent - use default model
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
		models = []string{fmt.Sprintf("%s:%s", cliName, model)}
	}

	// Print spawn info
	fmt.Printf("Spawning %s for %s...\n", req.AgentType, req.TicketID)
	if len(models) > 1 {
		fmt.Printf("  Parallel mode: %d models configured\n", len(models))
		for _, m := range models {
			fmt.Printf("    - %s\n", m)
		}
	} else {
		fmt.Printf("  Model: %s\n", models[0])
	}
	fmt.Printf("  Workflow: %s\n", req.WorkflowName)
	fmt.Println()

	// Start phase
	s.startPhase(wi.ID, req.ProjectID, req.TicketID, req.WorkflowName, phase.ID)

	// Spawn all models
	var processes []*processInfo
	for _, modelID := range models {
		proc, err := s.spawnSingle(req, modelID, phase.ID, wi.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: Failed to spawn %s: %v\n", modelID, err)
			continue
		}
		processes = append(processes, proc)
		fmt.Printf("  Started %s (PID: %d, Session: %s)\n", modelID, proc.cmd.Process.Pid, proc.sessionID)
	}

	if len(processes) == 0 {
		s.completePhase(wi.ID, req.ProjectID, req.TicketID, req.WorkflowName, phase.ID, "fail")
		return fmt.Errorf("no agents were spawned")
	}

	fmt.Println()

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

	// Load agent template
	prompt, err := s.loadTemplate(req.AgentType, req.TicketID, req.ProjectID, req.ParentSession, sessionID, req.WorkflowName, modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to load template: %w", err)
	}

	// Write prompt to temp file (needed for CLIs that support system prompt files)
	// Sanitize ticket ID for use in filename (replace path separators and other special chars)
	safeTicketID := strings.ReplaceAll(req.TicketID, "/", "_")
	safeTicketID = strings.ReplaceAll(safeTicketID, "\\", "_")
	promptFile, err := os.CreateTemp("", fmt.Sprintf("%s-%s-*.md", safeTicketID, req.AgentType))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := promptFile.WriteString(prompt); err != nil {
		os.Remove(promptFile.Name())
		return nil, fmt.Errorf("failed to write prompt: %w", err)
	}
	promptFile.Close()

	// Initial prompt
	initialPrompt := fmt.Sprintf(`Begin working on ticket %s. Follow the workflow steps in your system prompt.`, req.TicketID)

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
			fmt.Sprintf("NRWF_CONTEXT_THRESHOLD=%d", 100-s.config.getContextThreshold()),
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
	}

	// Register agent start (create agent_sessions row)
	s.registerAgentStart(req.ProjectID, req.TicketID, req.WorkflowName, wfiID, agentID, req.AgentType, cmd.Process.Pid, sessionID, modelID, phase, spawnCommand, prompt, "")

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

	// Single wait goroutine - closes doneCh when process exits
	go func() {
		proc.waitErr = cmd.Wait()
		close(proc.doneCh)
		os.Remove(promptFile.Name())
	}()

	return proc, nil
}

// monitorOutput reads stdout and tracks messages/stats for a process
func (s *Spawner) monitorOutput(proc *processInfo, stdout io.ReadCloser) {
	scanner := bufio.NewScanner(stdout)
	// Increase buffer size to 10MB for large JSON outputs (file reads, diffs, etc.)
	const maxScannerBuffer = 10 * 1024 * 1024
	scanner.Buffer(make([]byte, 64*1024), maxScannerBuffer)

	for scanner.Scan() {
		line := scanner.Text()
		s.processOutput(proc, line)
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("  [ERROR] Scanner error: %v\n", err)
	}
}

// monitorAll monitors all spawned processes until completion.
// If ctx is cancelled, kills all running processes and returns an error.
func (s *Spawner) monitorAll(ctx context.Context, processes []*processInfo, req SpawnRequest, phase string) error {
	const statusInterval = 30 * time.Second
	lastStatusTime := time.Time{}

	running := make([]*processInfo, len(processes))
	copy(running, processes)
	var completed []*processInfo

	for len(running) > 0 {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			// Kill all running processes
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
			s.completePhase(wfiID, req.ProjectID, req.TicketID, req.WorkflowName, phase, "fail")
			return ctx.Err()
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

			select {
			case <-proc.doneCh:
				// Process exited - doneCh was closed by the wait goroutine
				proc.elapsed = elapsed
				s.saveContextLeft(proc)
				s.handleCompletion(proc, req)

				// Check for continuation
				if proc.finalStatus == "CONTINUE" {
					maxCont := s.config.getMaxContinuations()
					if proc.continuationCount < maxCont {
						fmt.Printf("  %s: Continuation %d/%d — relaunching with fresh context...\n",
							proc.modelID, proc.continuationCount+1, maxCont)
						newProc, err := s.relaunchForContinuation(proc, req, phase)
						if err != nil {
							fmt.Fprintf(os.Stderr, "  Warning: Failed to relaunch %s: %v\n", proc.modelID, err)
							completed = append(completed, proc)
						} else {
							// Replace with new process in the running list
							stillRunning = append(stillRunning, newProc)
						}
					} else {
						fmt.Fprintf(os.Stderr, "  %s: Max continuations (%d) reached, marking as fail\n",
							proc.modelID, maxCont)
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
					s.handleGracefulTimeout(proc, req)
					completed = append(completed, proc)
				} else {
					stillRunning = append(stillRunning, proc)
					// Maybe flush messages and context
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
	return s.finalizePhase(completed, req, phase)
}

// handleGracefulTimeout sends SIGTERM, waits for grace period, then SIGKILL
func (s *Spawner) handleGracefulTimeout(proc *processInfo, req SpawnRequest) {
	proc.elapsed = time.Since(proc.startTime)

	// Send SIGTERM first
	if proc.cmd.Process != nil {
		proc.cmd.Process.Signal(syscall.SIGTERM)
	}

	// Grace period for clean shutdown
	gracePeriod := time.Duration(s.config.TimeoutGraceSec) * time.Second
	if gracePeriod == 0 {
		gracePeriod = 5 * time.Second
	}

	select {
	case <-proc.doneCh:
		// Exited gracefully after SIGTERM
	case <-time.After(gracePeriod):
		// Force kill
		if proc.cmd.Process != nil {
			proc.cmd.Process.Kill()
		}
		<-proc.doneCh // Wait for the wait goroutine to finish
	}

	proc.finalStatus = "TIMEOUT"
	fmt.Fprintf(os.Stderr, "  %s timed out after %v\n", proc.modelID, proc.timeout)

	// Final messages flush
	s.saveMessages(proc)

	// Register agent stop with timeout reason (also updates status to failed + sets ended_at)
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName, proc.sessionID, proc.agentID, "fail", "timeout", proc.modelID)
}

// maybeFlushMessages flushes messages to DB if interval elapsed
func (s *Spawner) maybeFlushMessages(proc *processInfo) {
	interval := time.Duration(s.config.MessageFlushIntervalMs) * time.Millisecond
	if interval == 0 {
		interval = 2 * time.Second
	}

	shouldFlush := time.Since(proc.lastMessagesFlush) >= interval

	if shouldFlush {
		if proc.messagesDirty {
			s.saveMessages(proc)
			proc.messagesDirty = false
		}
		s.saveContextLeft(proc)
		proc.lastMessagesFlush = time.Now()
	}
}

// printStatus prints status for all running/completed agents
func (s *Spawner) printStatus(running, completed []*processInfo, phase string) {
	fmt.Printf("[%s] %d agent(s) running:\n", phase, len(running))

	for _, proc := range running {
		elapsed := time.Since(proc.startTime).Round(time.Second)

		// Get last message
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

// handleCompletion handles a completed agent process with hybrid completion semantics
func (s *Spawner) handleCompletion(proc *processInfo, req SpawnRequest) {
	exitCode := 0
	if proc.cmd.ProcessState != nil {
		exitCode = proc.cmd.ProcessState.ExitCode()
	}

	var result, resultReason string

	if exitCode != 0 {
		// Non-zero exit = immediate fail
		result = "fail"
		resultReason = "exit_code"
		proc.finalStatus = "FAIL"
	} else {
		// Exit 0: check for explicit completion within grace period
		gracePeriod := time.Duration(s.config.CompletionGraceSec) * time.Second
		if gracePeriod == 0 {
			gracePeriod = 60 * time.Second
		}

		deadline := time.Now().Add(gracePeriod)
		for time.Now().Before(deadline) {
			explicit := s.getAgentResult(proc)
			if explicit == "pass" {
				result = "pass"
				resultReason = "explicit"
				break
			} else if explicit == "fail" {
				result = "fail"
				resultReason = "explicit"
				break
			} else if explicit == "continue" {
				result = "continue"
				resultReason = "explicit"
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		if result == "" {
			// No explicit completion within grace period
			result = "fail"
			resultReason = "no_complete"
		}

		switch result {
		case "pass":
			proc.finalStatus = "PASS"
		case "continue":
			proc.finalStatus = "CONTINUE"
		default:
			proc.finalStatus = "FAIL"
		}
	}

	// Save messages to database
	s.saveMessages(proc)

	// Register agent stop with reason
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName, proc.sessionID, proc.agentID, result, resultReason, proc.modelID)

	fmt.Printf("  %s: %s (exit code: %d, reason: %s, duration: %v)\n",
		proc.modelID, proc.finalStatus, exitCode, resultReason, proc.elapsed.Round(time.Second))
}

// relaunchForContinuation spawns a new agent process to continue where the previous one left off.
// It preserves the ancestor session chain and increments the continuation count.
func (s *Spawner) relaunchForContinuation(oldProc *processInfo, req SpawnRequest, phase string) (*processInfo, error) {
	// Determine ancestor session ID (root of the continuation chain)
	ancestorID := oldProc.ancestorSessionID
	if ancestorID == "" {
		// First continuation — the old session is the ancestor
		ancestorID = oldProc.sessionID
	}

	// Spawn a new process with the same model
	newProc, err := s.spawnSingle(req, oldProc.modelID, phase, oldProc.workflowInstanceID)
	if err != nil {
		return nil, err
	}

	// Carry over continuation tracking
	newProc.ancestorSessionID = ancestorID
	newProc.continuationCount = oldProc.continuationCount + 1

	// Update the ancestor_session_id on the new DB session record
	database, dbErr := db.Open(s.config.DataPath)
	if dbErr == nil {
		sessionRepo := repo.NewAgentSessionRepo(database)
		sessionRepo.UpdateAncestorSession(newProc.sessionID, ancestorID)
		database.Close()
	}

	// Broadcast continuation event
	s.broadcast(ws.EventAgentContinued, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"old_session_id":    oldProc.sessionID,
		"new_session_id":    newProc.sessionID,
		"ancestor_session":  ancestorID,
		"continuation_count": newProc.continuationCount,
		"agent_type":        req.AgentType,
		"model_id":          oldProc.modelID,
	})

	fmt.Printf("  Started continuation %s (PID: %d, Session: %s, Ancestor: %s)\n",
		oldProc.modelID, newProc.cmd.Process.Pid, newProc.sessionID, ancestorID)

	return newProc, nil
}

// getAgentResult reads the explicit result from agent_sessions table
func (s *Spawner) getAgentResult(proc *processInfo) string {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return ""
	}
	defer database.Close()

	sessionRepo := repo.NewAgentSessionRepo(database)
	session, err := sessionRepo.Get(proc.sessionID)
	if err != nil {
		return ""
	}

	if session.Result.Valid {
		return session.Result.String
	}
	return ""
}

// finalizePhase completes the phase after all agents finish
func (s *Spawner) finalizePhase(completed []*processInfo, req SpawnRequest, phase string) error {
	// Print summary
	fmt.Printf("\n[%s] All agents completed:\n", phase)
	for _, proc := range completed {
		fmt.Printf("  %s: %s (%v)\n", proc.modelID, proc.finalStatus, proc.elapsed.Round(time.Second))
	}
	fmt.Println()

	// Determine phase result - pass only if ALL agents pass
	allPassed := true
	for _, proc := range completed {
		if proc.finalStatus != "PASS" {
			allPassed = false
			break
		}
	}

	result := "pass"
	if !allPassed {
		result = "fail"
	}

	// Complete phase (get wfiID from first completed process)
	wfiID := ""
	if len(completed) > 0 {
		wfiID = completed[0].workflowInstanceID
	}
	s.completePhase(wfiID, req.ProjectID, req.TicketID, req.WorkflowName, phase, result)

	if allPassed {
		fmt.Printf("Phase complete: %s (PASS)\n", phase)
		return nil
	}

	var failedModels []string
	for _, proc := range completed {
		if proc.finalStatus != "PASS" {
			failedModels = append(failedModels, proc.modelID)
		}
	}
	fmt.Printf("Phase complete: %s (FAIL)\n", phase)
	fmt.Printf("  Failed: %s\n", strings.Join(failedModels, ", "))
	return fmt.Errorf("phase %s failed", phase)
}

// Preview generates the prompt without spawning
func (s *Spawner) Preview(agentType, ticketID, projectID, workflowName string) (string, error) {
	// Get model from config for preview
	model := "opus"
	cliName := s.config.DefaultCLI
	if cliName == "" {
		cliName = "claude"
	}
	if agentCfg, ok := s.config.Agents[agentType]; ok {
		if agentCfg.Model != "" {
			model = agentCfg.Model
		}
	}
	modelID := fmt.Sprintf("%s:%s", cliName, model)
	return s.loadTemplate(agentType, ticketID, projectID, "preview-parent", "preview-child", workflowName, modelID)
}

// loadPromptContent loads the prompt content for an agent from the DB.
func (s *Spawner) loadPromptContent(agentType, projectID, workflowName string) (string, error) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return "", fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database)
	def, err := adRepo.Get(projectID, workflowName, agentType)
	if err != nil {
		return "", fmt.Errorf("agent definition not found: %s (workflow=%s). Create via 'nrworkflow agent def create %s -w %s --prompt-file=<path>'", agentType, workflowName, agentType, workflowName)
	}
	if def.Prompt == "" {
		return "", fmt.Errorf("agent definition '%s' has empty prompt", agentType)
	}
	return def.Prompt, nil
}

// fetchTicketInfo returns the ticket title and description for template expansion.
// Returns placeholder text on error rather than failing the spawn.
func (s *Spawner) fetchTicketInfo(projectID, ticketID string) (title, description string) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open DB for ticket info: %v\n", err)
		return ticketID, "_No description available_"
	}
	defer database.Close()

	ticketRepo := repo.NewTicketRepo(database)
	ticket, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch ticket %s: %v\n", ticketID, err)
		return ticketID, "_No description available_"
	}
	title = ticket.Title
	if ticket.Description.Valid && ticket.Description.String != "" {
		description = ticket.Description.String
	} else {
		description = "_No description available_"
	}
	return title, description
}

// fetchUserInstructions returns user_instructions from the workflow instance findings.
// Returns placeholder text on error rather than failing the spawn.
func (s *Spawner) fetchUserInstructions(projectID, ticketID, workflowName string) string {
	pool, err := db.NewPool(s.config.DataPath, db.DefaultPoolConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open DB for user instructions: %v\n", err)
		return "_No user instructions provided_"
	}
	defer pool.Close()

	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	wi, err := wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch workflow instance for %s/%s: %v\n", ticketID, workflowName, err)
		return "_No user instructions provided_"
	}
	findings := wi.GetFindings()
	if instructions, ok := findings["user_instructions"]; ok {
		if str, ok := instructions.(string); ok && str != "" {
			return str
		}
		// Fallback: handle old nested map format {"instructions": "..."}
		if m, ok := instructions.(map[string]interface{}); ok {
			if str, ok := m["instructions"].(string); ok && str != "" {
				return str
			}
		}
	}
	return "_No user instructions provided_"
}

// loadTemplate loads and expands an agent template from DB.
func (s *Spawner) loadTemplate(agentType, ticketID, projectID, parentSession, childSession, workflowName, modelID string) (string, error) {
	promptContent, err := s.loadPromptContent(agentType, projectID, workflowName)
	if err != nil {
		return "", err
	}

	template := promptContent

	// Parse model from modelID
	_, model := parseModelID(modelID)
	if model == "" {
		model = "sonnet"
	}

	// Expand variables
	template = strings.ReplaceAll(template, "${AGENT}", agentType)
	template = strings.ReplaceAll(template, "${TICKET_ID}", ticketID)
	template = strings.ReplaceAll(template, "${WORKFLOW}", workflowName)
	template = strings.ReplaceAll(template, "${PARENT_SESSION}", parentSession)
	template = strings.ReplaceAll(template, "${CHILD_SESSION}", childSession)
	template = strings.ReplaceAll(template, "${MODEL_ID}", modelID)
	template = strings.ReplaceAll(template, "${MODEL}", model)

	// Expand ticket context variables (title, description, user instructions)
	if strings.Contains(template, "${TICKET_TITLE}") || strings.Contains(template, "${TICKET_DESCRIPTION}") {
		title, desc := s.fetchTicketInfo(projectID, ticketID)
		template = strings.ReplaceAll(template, "${TICKET_TITLE}", title)
		template = strings.ReplaceAll(template, "${TICKET_DESCRIPTION}", desc)
	}
	if strings.Contains(template, "${USER_INSTRUCTIONS}") {
		instructions := s.fetchUserInstructions(projectID, ticketID, workflowName)
		template = strings.ReplaceAll(template, "${USER_INSTRUCTIONS}", instructions)
	}

	// Expand findings patterns (after variable substitution)
	template, err = s.expandFindings(template, projectID, ticketID, workflowName)
	if err != nil {
		// Log warning but don't fail - findings might not exist yet
		fmt.Fprintf(os.Stderr, "Warning: findings expansion: %v\n", err)
	}

	return template, nil
}

// expandFindings replaces #{FINDINGS:AGENT:KEY} patterns with actual findings data.
// Patterns:
//   - #{FINDINGS:agent}           - All findings for agent
//   - #{FINDINGS:agent:key}       - Single specific key
//   - #{FINDINGS:agent:key1,key2} - Multiple specific keys
func (s *Spawner) expandFindings(template, projectID, ticketID, workflowName string) (string, error) {
	// Pattern: #{FINDINGS:agent_type} or #{FINDINGS:agent_type:key(s)}
	re := regexp.MustCompile(`#\{FINDINGS:([^:}]+)(?::([^}]*))?\}`)

	var lastErr error
	result := re.ReplaceAllStringFunc(template, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		agentType := parts[1]
		var keys []string
		if len(parts) >= 3 && parts[2] != "" {
			// Split comma-separated keys
			keys = strings.Split(parts[2], ",")
			for i := range keys {
				keys[i] = strings.TrimSpace(keys[i])
			}
		}

		// Fetch findings
		findings, err := s.fetchFindings(projectID, ticketID, workflowName, agentType, keys)
		if err != nil {
			lastErr = err
			return s.formatFindingsError(agentType)
		}

		return s.formatFindings(agentType, findings, keys)
	})

	return result, lastErr
}

// fetchFindings retrieves findings from the database using the FindingsService
func (s *Spawner) fetchFindings(projectID, ticketID, workflowName, agentType string, keys []string) (interface{}, error) {
	pool, err := db.NewPool(s.config.DataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer pool.Close()

	findingsService := service.NewFindingsService(pool)

	req := &types.FindingsGetRequest{
		Workflow:  workflowName,
		AgentType: agentType,
		Keys:      keys,
	}

	return findingsService.Get(projectID, ticketID, req)
}

// formatFindings converts findings to human-readable text (YAML-like format)
func (s *Spawner) formatFindings(agentType string, findings interface{}, keys []string) string {
	if findings == nil {
		return s.formatFindingsError(agentType)
	}

	findingsMap, ok := findings.(map[string]interface{})
	if !ok {
		// Single value (when fetching a single key)
		return s.formatValue(findings, "")
	}

	if len(findingsMap) == 0 {
		return s.formatFindingsError(agentType)
	}

	// Check if this is a parallel agents result (keys are model IDs like "claude:opus")
	isParallel := false
	for k := range findingsMap {
		if strings.Contains(k, ":") {
			isParallel = true
			break
		}
	}

	if isParallel {
		return s.formatParallelFindings(agentType, findingsMap, keys)
	}

	return s.formatSingleAgentFindings(findingsMap)
}

// formatParallelFindings formats findings from multiple parallel agents
func (s *Spawner) formatParallelFindings(agentType string, findings map[string]interface{}, keys []string) string {
	var lines []string

	// Sort model keys for consistent output
	var modelKeys []string
	for k := range findings {
		modelKeys = append(modelKeys, k)
	}
	sort.Strings(modelKeys)

	for _, modelKey := range modelKeys {
		agentKey := agentType + ":" + modelKey
		v := findings[modelKey]

		if len(keys) == 1 {
			// Single key requested - compact format: "- agent:model: value"
			lines = append(lines, fmt.Sprintf("- %s: %s", agentKey, s.formatValue(v, "")))
		} else {
			// Multiple keys or all findings - expanded format
			lines = append(lines, fmt.Sprintf("- %s:", agentKey))
			if agentFindings, ok := v.(map[string]interface{}); ok {
				// Sort keys for consistent output
				var sortedKeys []string
				for k := range agentFindings {
					sortedKeys = append(sortedKeys, k)
				}
				sort.Strings(sortedKeys)
				for _, k := range sortedKeys {
					val := agentFindings[k]
					lines = append(lines, "  "+k+":"+s.formatValue(val, "  "))
				}
			} else {
				lines = append(lines, "  "+s.formatValue(v, "  "))
			}
		}
	}

	return strings.Join(lines, "\n")
}

// formatSingleAgentFindings formats findings from a single agent as "key: value" lines
func (s *Spawner) formatSingleAgentFindings(findings map[string]interface{}) string {
	var lines []string

	// Sort keys for consistent output
	var keys []string
	for k := range findings {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		val := findings[key]
		lines = append(lines, key+":"+s.formatValue(val, ""))
	}
	return strings.Join(lines, "\n")
}

// formatValue converts any value to YAML-like text (never JSON)
func (s *Spawner) formatValue(v interface{}, indent string) string {
	switch val := v.(type) {
	case string:
		// Simple string value - add space after colon if inline
		if indent == "" {
			return " " + val
		}
		return " " + val
	case []interface{}:
		// Array - bullet list
		var lines []string
		for _, item := range val {
			itemStr := s.formatValue(item, indent+"  ")
			// Remove leading space for array items
			itemStr = strings.TrimPrefix(itemStr, " ")
			lines = append(lines, indent+"  - "+itemStr)
		}
		return "\n" + strings.Join(lines, "\n")
	case map[string]interface{}:
		// Object - nested key: value
		var lines []string
		// Sort keys for consistent output
		var keys []string
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, indent+"  "+k+":"+s.formatValue(val[k], indent+"  "))
		}
		return "\n" + strings.Join(lines, "\n")
	case float64:
		// JSON numbers come as float64
		if val == float64(int(val)) {
			return fmt.Sprintf(" %d", int(val))
		}
		return fmt.Sprintf(" %v", val)
	case bool:
		return fmt.Sprintf(" %v", val)
	case nil:
		return " null"
	default:
		return fmt.Sprintf(" %v", val)
	}
}

// formatFindingsError returns a placeholder for missing findings
func (s *Spawner) formatFindingsError(agentType string) string {
	return fmt.Sprintf("_No findings yet available from %s_", agentType)
}

// processOutput processes a line of output from the agent and tracks stats
// Handles both Claude CLI and opencode JSON formats
func (s *Spawner) processOutput(proc *processInfo, line string) {
	// Try to parse as JSON (stream-json format)
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		// Not JSON, skip
		return
	}

	// Extract message based on type
	eventType, _ := data["type"].(string)
	switch eventType {

	// === Claude CLI format ===
	case "assistant":
		message, _ := data["message"].(map[string]interface{})
		content, _ := message["content"].([]interface{})
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				itemType, _ := itemMap["type"].(string)
				if itemType == "text" {
					text, _ := itemMap["text"].(string)
					if text != "" {
						s.handleTextMessage(proc, text)
					}
				} else if itemType == "tool_use" {
					toolName, _ := itemMap["name"].(string)
					if toolName != "" {
						input, _ := itemMap["input"].(map[string]interface{})
						s.handleToolUse(proc, toolName, input)
					}
				}
			}
		}

	case "result":
		// Result subtype tracked via messages only

	// === Opencode format ===
	case "text":
		// Text content from opencode
		part, _ := data["part"].(map[string]interface{})
		if part != nil {
			text, _ := part["text"].(string)
			if text != "" {
				s.handleTextMessage(proc, text)
			}
		}

	case "tool_use":
		// Tool execution from opencode
		part, _ := data["part"].(map[string]interface{})
		if part != nil {
			toolName, _ := part["tool"].(string)
			if toolName != "" {
				// Opencode puts input under state.input, not part.input
				var input map[string]interface{}
				state, _ := part["state"].(map[string]interface{})
				if state != nil {
					input, _ = state["input"].(map[string]interface{})
				}
				// Fallback to part.input if state.input not found
				if input == nil {
					input, _ = part["input"].(map[string]interface{})
				}
				s.handleToolUse(proc, toolName, input)
			}
		}

	case "tool_result":
		// Tool result from opencode

	case "step_finish":
		// Step completion from opencode

	case "finish":
		// Session finish from opencode

	// === Codex CLI format ===
	case "thread.started":
		// Session start from codex

	case "turn.started":
		// Turn start from codex

	case "item.completed":
		// Item completion from codex - contains messages and tool calls
		item, _ := data["item"].(map[string]interface{})
		if item != nil {
			itemType, _ := item["type"].(string)
			switch itemType {
			case "agent_message":
				text, _ := item["text"].(string)
				if text != "" {
					s.handleTextMessage(proc, text)
				}
			case "tool_call":
				toolName, _ := item["name"].(string)
				if toolName != "" {
					args, _ := item["arguments"].(map[string]interface{})
					s.handleToolUse(proc, toolName, args)
				}
			case "tool_result":
				// Tool result from codex
			}
		}

	case "turn.completed":
		// Turn completion from codex
	}
}

// handleTextMessage processes text output from either Claude or opencode
func (s *Spawner) handleTextMessage(proc *processInfo, text string) {
	// Track message content (truncate for storage)
	msgPreview := text
	if len(msgPreview) > 150 {
		msgPreview = msgPreview[:150] + "..."
	}
	s.trackMessage(proc, msgPreview)

	// Print to console with truncation for long messages
	prefix := s.formatPrefix(proc)
	maxLen := 500
	if len(text) <= maxLen {
		fmt.Printf("  %s %s\n", prefix, text)
	} else {
		// Show start + ... + end for context
		startLen := 300
		endLen := 150
		fmt.Printf("  %s %s\n  ... [%d chars truncated] ...\n  %s\n", prefix, text[:startLen], len(text)-startLen-endLen, text[len(text)-endLen:])
	}
}

// handleToolUse processes tool usage from either Claude or opencode
func (s *Spawner) handleToolUse(proc *processInfo, toolName string, input map[string]interface{}) {
	toolDetail := s.formatToolDetail(toolName, input)

	// Track message
	s.trackMessage(proc, toolDetail)

	// Print to console with prefix
	prefix := s.formatPrefix(proc)
	fmt.Printf("  %s %s\n", prefix, toolDetail)
}

// trackMessage adds a message to the pending queue for DB insertion
func (s *Spawner) trackMessage(proc *processInfo, msg string) {
	proc.messagesMutex.Lock()
	defer proc.messagesMutex.Unlock()
	proc.pendingMessages = append(proc.pendingMessages, msg)
	proc.lastMessage = msg
	proc.messagesDirty = true
}

// formatPrefix returns a prefix string with agent type and model for console output
func (s *Spawner) formatPrefix(proc *processInfo) string {
	// Parse model from modelID (cli:model format)
	_, model := parseModelID(proc.modelID)
	if model == "" {
		model = "default"
	}
	return fmt.Sprintf("[%s:%s]", proc.agentType, model)
}

// formatToolDetail extracts relevant details from tool input based on tool type
func (s *Spawner) formatToolDetail(toolName string, input map[string]interface{}) string {
	// Normalize tool name to title case (opencode sends lowercase, Claude sends capitalized)
	if len(toolName) > 0 {
		toolName = strings.ToUpper(toolName[:1]) + toolName[1:]
	}

	if input == nil {
		return "[" + toolName + "]"
	}

	var detail string

	switch toolName {
	case "Skill":
		// Claude uses "skill", opencode uses "name"
		skillName, _ := input["skill"].(string)
		if skillName == "" {
			skillName, _ = input["name"].(string)
		}
		skillArgs, _ := input["args"].(string)
		if skillName != "" {
			detail = "skill:" + skillName
			if skillArgs != "" {
				detail += " " + skillArgs
			}
		}

	case "Bash":
		cmd, _ := input["command"].(string)
		if cmd != "" {
			detail = cmd
		}

	case "Read":
		// Try both snake_case (Claude) and camelCase (opencode)
		path, _ := input["file_path"].(string)
		if path == "" {
			path, _ = input["filePath"].(string)
		}
		if path != "" {
			detail = path
		}

	case "Write":
		path, _ := input["file_path"].(string)
		if path == "" {
			path, _ = input["filePath"].(string)
		}
		if path != "" {
			detail = path
		}

	case "Edit":
		path, _ := input["file_path"].(string)
		if path == "" {
			path, _ = input["filePath"].(string)
		}
		if path != "" {
			detail = path
		}

	case "Glob":
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		if pattern != "" {
			detail = pattern
			if path != "" {
				detail = path + "/" + pattern
			}
		}

	case "Grep":
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		if pattern != "" {
			detail = pattern
			if path != "" {
				detail += " in " + path
			}
		}

	case "Task":
		desc, _ := input["description"].(string)
		agentType, _ := input["subagent_type"].(string)
		if desc != "" {
			detail = desc
			if agentType != "" {
				detail = agentType + ": " + desc
			}
		}

	case "WebFetch":
		url, _ := input["url"].(string)
		if url != "" {
			detail = url
		}

	case "WebSearch":
		query, _ := input["query"].(string)
		if query != "" {
			detail = query
		}

	case "TodoWrite", "TaskCreate", "TaskUpdate", "TaskList":
		// Just show tool name for task management tools
		return "[" + toolName + "]"
	}

	// Format output: [ToolName] detail (truncated if needed)
	if detail == "" {
		return "[" + toolName + "]"
	}

	// Truncate long details
	maxLen := 200
	if len(detail) > maxLen {
		detail = detail[:maxLen] + "..."
	}

	return "[" + toolName + "] " + detail
}

// saveMessages flushes pending messages to the agent_messages table
func (s *Spawner) saveMessages(proc *processInfo) {
	// Drain pending messages
	proc.messagesMutex.Lock()
	pending := proc.pendingMessages
	proc.pendingMessages = make([]string, 0)
	seqStart := proc.nextSeq
	proc.nextSeq += len(pending)
	proc.messagesMutex.Unlock()

	if len(pending) == 0 {
		return
	}

	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	msgRepo := repo.NewAgentMessageRepo(database)
	msgRepo.InsertBatch(proc.sessionID, seqStart, pending)

	// Broadcast messages update for real-time UI
	if proc.projectID != "" {
		s.broadcast(ws.EventMessagesUpdated, proc.projectID, proc.ticketID, proc.workflowName, map[string]interface{}{
			"session_id": proc.sessionID,
			"agent_type": proc.agentType,
			"model_id":   proc.modelID,
		})
	}
}

// contextFileEntry represents one entry in /tmp/usable_context.json
type contextFileEntry struct {
	PctUsed *float64 `json:"pct_used"`
}

// readContextFile reads /tmp/usable_context.json and returns parsed data.
// Returns nil on any error (file not found, parse error, etc).
func readContextFile() map[string]contextFileEntry {
	data, err := os.ReadFile("/tmp/usable_context.json")
	if err != nil {
		return nil
	}
	var result map[string]contextFileEntry
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// updateContextLeft updates the context_left field on a process from context file data
func updateContextLeft(proc *processInfo, contextData map[string]contextFileEntry) {
	if contextData == nil {
		return
	}
	entry, ok := contextData[proc.sessionID]
	if !ok || entry.PctUsed == nil {
		return
	}
	remaining := 100 - int(*entry.PctUsed)
	if remaining != proc.contextLeft {
		proc.contextLeft = remaining
		proc.contextLeftDirty = true
	}
}

// saveContextLeft saves context_left to the database if dirty
func (s *Spawner) saveContextLeft(proc *processInfo) {
	if !proc.contextLeftDirty {
		return
	}
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	sessionRepo := repo.NewAgentSessionRepo(database)
	sessionRepo.UpdateContextLeft(proc.sessionID, proc.contextLeft)
	proc.contextLeftDirty = false
}

// registerAgentStart creates an agent_sessions row for a newly spawned agent
func (s *Spawner) registerAgentStart(projectID, ticketID, workflowName, wfiID, agentID, agentType string, pid int, sessionID, modelID, phase, spawnCommand, promptContext, ancestorSessionID string) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	sessionRepo := repo.NewAgentSessionRepo(database)
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          projectID,
		TicketID:           ticketID,
		WorkflowInstanceID: wfiID,
		Phase:              phase,
		AgentType:          agentType,
		ModelID:            sql.NullString{String: modelID, Valid: modelID != ""},
		Status:             model.AgentSessionRunning,
		PID:                sql.NullInt64{Int64: int64(pid), Valid: pid > 0},
		SpawnCommand:       sql.NullString{String: spawnCommand, Valid: spawnCommand != ""},
		PromptContext:      sql.NullString{String: promptContext, Valid: promptContext != ""},
		AncestorSessionID:  sql.NullString{String: ancestorSessionID, Valid: ancestorSessionID != ""},
		StartedAt:          sql.NullString{String: now, Valid: true},
	}
	sessionRepo.Create(session)

	s.broadcast(ws.EventAgentStarted, projectID, ticketID, workflowName, map[string]interface{}{
		"agent_id":   agentID,
		"agent_type": agentType,
		"model_id":   modelID,
		"session_id": sessionID,
		"phase":      phase,
	})
}

// registerAgentStopWithReason updates the agent_sessions row when an agent stops
func (s *Spawner) registerAgentStopWithReason(projectID, ticketID, workflowName, sessionID, agentID, result, resultReason, modelID string) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	sessionRepo := repo.NewAgentSessionRepo(database)

	// Update result and reason
	sessionRepo.UpdateResult(sessionID, result, resultReason)

	// Set ended_at timestamp
	sessionRepo.SetEndedAt(sessionID)

	// Update session status based on result
	status := model.AgentSessionCompleted
	switch result {
	case "fail", "timeout":
		status = model.AgentSessionFailed
	case "continue":
		status = model.AgentSessionContinued
	}
	sessionRepo.UpdateStatus(sessionID, status)

	s.broadcast(ws.EventAgentCompleted, projectID, ticketID, workflowName, map[string]interface{}{
		"agent_id":      agentID,
		"result":        result,
		"result_reason": resultReason,
		"model_id":      modelID,
	})
}

// getWorkflowInstance retrieves the workflow instance for a ticket, returning an error if not initialized
func (s *Spawner) getWorkflowInstance(projectID, ticketID, workflowName string) (*model.WorkflowInstance, error) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	wi, err := wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	if err != nil {
		return nil, fmt.Errorf("workflow '%s' not initialized on ticket '%s'. Use the web UI or API to initialize it",
			workflowName, ticketID)
	}
	return wi, nil
}

// validateAndAdvancePhase validates phase order and auto-skips phases with matching skip_for rules.
// Returns (phaseID, shouldSkip, error). Uses workflow_instances table for state.
func (s *Spawner) validateAndAdvancePhase(wi *model.WorkflowInstance, workflowName, requestedAgent string) (string, bool, error) {
	workflow, ok := s.config.Workflows[workflowName]
	if !ok {
		return "", false, fmt.Errorf("unknown workflow: %s", workflowName)
	}

	// Find requested agent's phase
	var requestedPhase *PhaseDef
	var requestedIndex int = -1
	for i := range workflow.Phases {
		if workflow.Phases[i].Agent == requestedAgent {
			requestedPhase = &workflow.Phases[i]
			requestedIndex = i
			break
		}
	}
	if requestedPhase == nil {
		return "", false, fmt.Errorf("agent '%s' not found in workflow '%s'", requestedAgent, workflowName)
	}

	phases := wi.GetPhases()
	category := ""
	if wi.Category.Valid {
		category = wi.Category.String
	}

	// Check if requested phase should be skipped
	if s.categoryMatchesSkipFor(category, requestedPhase.SkipFor) {
		s.completePhase(wi.ID, wi.ProjectID, wi.TicketID, workflowName, requestedPhase.ID, "skipped")
		return requestedPhase.ID, true, nil
	}

	// Validate that prior phases are completed or skipped
	for i := 0; i < requestedIndex; i++ {
		priorPhase := workflow.Phases[i]
		phaseStatus, exists := phases[priorPhase.ID]

		if exists && phaseStatus.Status == "completed" {
			continue
		}

		// Check if phase can be auto-skipped due to category
		if s.categoryMatchesSkipFor(category, priorPhase.SkipFor) {
			s.completePhase(wi.ID, wi.ProjectID, wi.TicketID, workflowName, priorPhase.ID, "skipped")
			continue
		}

		return "", false, fmt.Errorf("phase '%s' must complete before '%s'", priorPhase.ID, requestedPhase.ID)
	}

	return requestedPhase.ID, false, nil
}

// categoryMatchesSkipFor checks if the category matches any of the skip_for rules
func (s *Spawner) categoryMatchesSkipFor(category string, skipFor []string) bool {
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

// startPhase marks a phase as in_progress using workflow_instances table
func (s *Spawner) startPhase(wfiID, projectID, ticketID, workflowName, phase string) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	if err := wfiRepo.StartPhase(wfiID, phase); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to start phase %s: %v\n", phase, err)
		return
	}

	s.broadcast(ws.EventPhaseStarted, projectID, ticketID, workflowName, map[string]interface{}{
		"phase": phase,
	})
}

// completePhase marks a phase as completed using workflow_instances table
func (s *Spawner) completePhase(wfiID, projectID, ticketID, workflowName, phase, result string) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	if err := wfiRepo.CompletePhase(wfiID, phase, result); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to complete phase %s: %v\n", phase, err)
		return
	}

	s.broadcast(ws.EventPhaseCompleted, projectID, ticketID, workflowName, map[string]interface{}{
		"phase":  phase,
		"result": result,
	})
}

func parseModelID(modelID string) (cli, model string) {
	if modelID == "" || !strings.Contains(modelID, ":") {
		return "claude", modelID
	}
	parts := strings.SplitN(modelID, ":", 2)
	return parts[0], parts[1]
}
