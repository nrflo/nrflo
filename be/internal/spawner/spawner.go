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

	"be/internal/ws"
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

const (
	defaultMaxContinuations = 3
	defaultContextThreshold = 30
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
	// Raw output buffering (stdout/stderr lines before any parsing)
	pendingRawOutput []string
	rawOutputDirty   bool
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
	// Low-context save state
	lowContextSaving bool // True while initiateContextSave is running
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
		models = phase.Parallel.Models
	} else {
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
	prompt, err := s.loadTemplate(req.AgentType, req.TicketID, req.ProjectID, req.ParentSession, sessionID, req.WorkflowName, modelID, phase)
	if err != nil {
		return nil, fmt.Errorf("failed to load template: %w", err)
	}

	// Write prompt to temp file
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
			fmt.Sprintf("NRWF_CONTEXT_THRESHOLD=%d", 100-defaultContextThreshold),
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
		pendingRawOutput:  make([]string, 0),
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
	s.registerAgentStart(req.ProjectID, req.TicketID, req.WorkflowName, wfiID, agentID, req.AgentType, cmd.Process.Pid, sessionID, modelID, phase, spawnCommand, prompt, "", 0)

	// Start output monitoring goroutines
	go s.monitorOutput(proc, stdout)
	go func() {
		scanner := bufio.NewScanner(stderr)
		prefix := s.formatPrefix(proc)
		for scanner.Scan() {
			line := scanner.Text()
			// Display and track stderr for debugging
			fmt.Printf("  %s [stderr] %s\n", prefix, line)
			s.trackRawOutput(proc, "[stderr] "+line)
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

			// Detect low context and initiate save (only for CLIs that support resume)
			if !proc.lowContextSaving && proc.contextLeft > 0 && proc.contextLeft <= defaultContextThreshold {
				cliName, _ := parseModelID(proc.modelID)
				adapter, _ := GetCLIAdapter(cliName)
				if adapter != nil && adapter.SupportsResume() {
					proc.lowContextSaving = true
					// Replace doneCh — initiateContextSave will close the new one when the full flow completes
					oldDoneCh := proc.doneCh
					newDoneCh := make(chan struct{})
					proc.doneCh = newDoneCh
					go s.initiateContextSave(proc, req, oldDoneCh, newDoneCh)
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
					s.handleCompletion(proc, req)
				}

				// Check for continuation
				if proc.finalStatus == "CONTINUE" {
					if proc.restartCount < defaultMaxContinuations {
						fmt.Printf("  %s: Continuation %d/%d — relaunching with fresh context...\n",
							proc.modelID, proc.restartCount+1, defaultMaxContinuations)
						newProc, err := s.relaunchForContinuation(proc, req, phase)
						if err != nil {
							fmt.Fprintf(os.Stderr, "  Warning: Failed to relaunch %s: %v\n", proc.modelID, err)
							completed = append(completed, proc)
						} else {
							stillRunning = append(stillRunning, newProc)
						}
					} else {
						fmt.Fprintf(os.Stderr, "  %s: Max continuations (%d) reached, marking as fail\n",
							proc.modelID, defaultMaxContinuations)
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

// finalizePhase completes the phase after all agents finish
func (s *Spawner) finalizePhase(completed []*processInfo, req SpawnRequest, phase string) error {
	fmt.Printf("\n[%s] All agents completed:\n", phase)
	for _, proc := range completed {
		fmt.Printf("  %s: %s (%v)\n", proc.modelID, proc.finalStatus, proc.elapsed.Round(time.Second))
	}
	fmt.Println()

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

func parseModelID(modelID string) (cli, model string) {
	if modelID == "" || !strings.Contains(modelID, ":") {
		return "claude", modelID
	}
	parts := strings.SplitN(modelID, ":", 2)
	return parts[0], parts[1]
}
