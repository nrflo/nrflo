package spawner

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"nrworkflow/internal/db"
	"nrworkflow/internal/model"
	"nrworkflow/internal/repo"
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
	Model    string `json:"model"`
	MaxTurns int    `json:"max_turns"`
	Timeout  int    `json:"timeout"`
}

// FullConfig represents the complete config.json structure
type FullConfig struct {
	CLI struct {
		Default string `json:"default"`
	} `json:"cli"`
	Agents    map[string]AgentConfig `json:"agents"`
	Workflows map[string]WorkflowDef `json:"workflows"`
}

// Config holds the spawner configuration
type Config struct {
	Workflows   map[string]WorkflowDef
	Agents      map[string]AgentConfig
	DefaultCLI  string
	DataPath    string
	ProjectRoot string
	// Spawner behavior settings
	TimeoutGraceSec      int // Grace period for SIGTERM before SIGKILL (default: 5)
	CompletionGraceSec   int // Wait for explicit completion after exit 0 (default: 60)
	StatsFlushIntervalMs int // Interval between stats flushes (default: 2000)
	StatsFlushMaxEvents  int // Max events before forced flush (default: 25)
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
	messages      []string
	messagesMutex sync.Mutex
	stats         map[string]int
	statsMutex    sync.Mutex
	finalStatus   string
	elapsed       time.Duration
	// Process lifecycle tracking
	doneCh  chan struct{} // closed when process exits
	waitErr error         // stores Wait() error
	// Stats buffering
	statsDirty     bool
	lastStatsFlush time.Time
	// Spawn context (for debugging/replay)
	spawnCommand  string
	promptContext string
}

// Spawner manages agent lifecycle
type Spawner struct {
	config      Config
	maxMessages int
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
		config:      config,
		maxMessages: 50, // Keep last 50 messages
	}
}

// Spawn spawns agents for a phase according to workflow config.
// If parallel is enabled, spawns all configured models concurrently.
func (s *Spawner) Spawn(req SpawnRequest) error {
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
	if err := s.validateWorkflowInitialized(req.ProjectID, req.TicketID, req.WorkflowName); err != nil {
		return err
	}

	// Validate phase order and auto-skip phases with matching skip_for category
	validatedPhaseID, shouldSkip, err := s.validateAndAdvancePhase(req.ProjectID, req.TicketID, req.WorkflowName, req.AgentType)
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
	s.startPhase(req.ProjectID, req.TicketID, req.WorkflowName, phase.ID)

	// Spawn all models
	var processes []*processInfo
	for _, modelID := range models {
		proc, err := s.spawnSingle(req, modelID, phase.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: Failed to spawn %s: %v\n", modelID, err)
			continue
		}
		processes = append(processes, proc)
		fmt.Printf("  Started %s (PID: %d, Session: %s)\n", modelID, proc.cmd.Process.Pid, proc.sessionID)
	}

	if len(processes) == 0 {
		s.completePhase(req.ProjectID, req.TicketID, req.WorkflowName, phase.ID, "fail")
		return fmt.Errorf("no agents were spawned")
	}

	fmt.Println()

	// Monitor all processes
	return s.monitorAll(processes, req, phase.ID)
}

// spawnSingle spawns a single agent process using the appropriate CLI adapter
func (s *Spawner) spawnSingle(req SpawnRequest, modelID, phase string) (*processInfo, error) {
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
	maxTurns := 50
	timeout := 25 // minutes
	if agentCfg, ok := s.config.Agents[req.AgentType]; ok {
		if agentCfg.MaxTurns > 0 {
			maxTurns = agentCfg.MaxTurns
		}
		if agentCfg.Timeout > 0 {
			timeout = agentCfg.Timeout
		}
	}

	// Load agent template
	prompt, err := s.loadTemplate(req.AgentType, req.TicketID, req.ParentSession, sessionID, req.WorkflowName, modelID)
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
	initialPrompt := fmt.Sprintf(`Begin working on ticket %s. Follow the workflow steps in your system prompt.

CRITICAL REQUIREMENT: After completing all work, you MUST run this exact command:
  nrworkflow agent complete %s %s -w %s

If you fail to run this command, the workflow will be blocked. This is not optional.`,
		req.TicketID, req.TicketID, req.AgentType, req.WorkflowName)

	// Prepare spawn options
	workDir := s.config.ProjectRoot
	if workDir == "" || workDir == "." {
		workDir = ""
	}

	opts := SpawnOptions{
		Model:         model,
		MaxTurns:      maxTurns,
		SessionID:     sessionID,
		PromptFile:    promptFile.Name(),
		Prompt:        prompt,
		InitialPrompt: initialPrompt,
		WorkDir:       workDir,
		Env:           append(os.Environ(), fmt.Sprintf("NRWORKFLOW_PROJECT=%s", req.ProjectID)),
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
		messages:       make([]string, 0),
		stats:          make(map[string]int),
		doneCh:         make(chan struct{}),
		lastStatsFlush: time.Now(),
		spawnCommand:   spawnCommand,
		promptContext:  prompt,
	}

	// Register agent start (with spawn context)
	s.registerAgentStart(req.ProjectID, req.TicketID, req.WorkflowName, agentID, req.AgentType, cmd.Process.Pid, sessionID, modelID, phase, spawnCommand, prompt)

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

// monitorAll monitors all spawned processes until completion
func (s *Spawner) monitorAll(processes []*processInfo, req SpawnRequest, phase string) error {
	const statusInterval = 30 * time.Second
	lastStatusTime := time.Time{}

	running := make([]*processInfo, len(processes))
	copy(running, processes)
	var completed []*processInfo

	for len(running) > 0 {
		now := time.Now()

		// Print status every interval
		if now.Sub(lastStatusTime) >= statusInterval {
			s.printStatus(running, completed, phase)
			lastStatusTime = now
		}

		// Check each process using doneCh (no double-wait bug)
		var stillRunning []*processInfo
		for _, proc := range running {
			elapsed := time.Since(proc.startTime)

			select {
			case <-proc.doneCh:
				// Process exited - doneCh was closed by the wait goroutine
				proc.elapsed = elapsed
				s.handleCompletion(proc, req)
				completed = append(completed, proc)
			default:
				// Still running - check timeout
				if elapsed > proc.timeout {
					s.handleGracefulTimeout(proc, req)
					completed = append(completed, proc)
				} else {
					stillRunning = append(stillRunning, proc)
					// Maybe flush stats
					s.maybeFlushStats(proc)
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

	// Final stats flush
	s.saveStats(proc)

	// Register agent stop with timeout reason
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName, proc.agentID, "fail", "timeout", proc.modelID)

	// Update session status
	database, err := db.Open(s.config.DataPath)
	if err == nil {
		sessionRepo := repo.NewAgentSessionRepo(database)
		sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionTimeout)
		database.Close()
	}
}

// maybeFlushStats flushes stats to DB if interval elapsed
func (s *Spawner) maybeFlushStats(proc *processInfo) {
	interval := time.Duration(s.config.StatsFlushIntervalMs) * time.Millisecond
	if interval == 0 {
		interval = 2 * time.Second
	}

	shouldFlush := time.Since(proc.lastStatsFlush) >= interval

	if shouldFlush && proc.statsDirty {
		s.saveStats(proc)
		proc.statsDirty = false
		proc.lastStatsFlush = time.Now()
	}
}

// printStatus prints status for all running/completed agents
func (s *Spawner) printStatus(running, completed []*processInfo, phase string) {
	fmt.Printf("[%s] %d agent(s) running:\n", phase, len(running))

	for _, proc := range running {
		elapsed := time.Since(proc.startTime).Round(time.Second)

		// Get last message
		proc.messagesMutex.Lock()
		lastMsg := ""
		if len(proc.messages) > 0 {
			lastMsg = proc.messages[len(proc.messages)-1]
			if len(lastMsg) > 80 {
				lastMsg = lastMsg[:77] + "..."
			}
			lastMsg = " | " + lastMsg
		}
		proc.messagesMutex.Unlock()

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
			explicit := s.getAgentResult(req, proc)
			if explicit == "pass" {
				result = "pass"
				resultReason = "explicit"
				break
			} else if explicit == "fail" {
				result = "fail"
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

		if result == "pass" {
			proc.finalStatus = "PASS"
		} else {
			proc.finalStatus = "FAIL"
		}
	}

	// Save stats to database
	s.saveStats(proc)

	// Register agent stop with reason
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName, proc.agentID, result, resultReason, proc.modelID)

	fmt.Printf("  %s: %s (exit code: %d, reason: %s, duration: %v)\n",
		proc.modelID, proc.finalStatus, exitCode, resultReason, proc.elapsed.Round(time.Second))
}

// getAgentResult reads the explicit result from active_agents in ticket state
func (s *Spawner) getAgentResult(req SpawnRequest, proc *processInfo) string {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return ""
	}
	defer database.Close()

	ticketRepo := repo.NewTicketRepo(database)
	ticket, err := ticketRepo.Get(req.ProjectID, req.TicketID)
	if err != nil {
		return ""
	}

	if !ticket.AgentsState.Valid {
		return ""
	}

	var allState map[string]interface{}
	if err := json.Unmarshal([]byte(ticket.AgentsState.String), &allState); err != nil {
		return ""
	}

	stateRaw, ok := allState[req.WorkflowName]
	if !ok {
		return ""
	}
	state, _ := stateRaw.(map[string]interface{})

	activeAgents, _ := state["active_agents"].(map[string]interface{})
	if activeAgents == nil {
		return ""
	}

	// Look for this agent by key or agent_id
	key := proc.agentType + ":" + proc.modelID
	if agentRaw, ok := activeAgents[key]; ok {
		agent, _ := agentRaw.(map[string]interface{})
		if result, ok := agent["result"].(string); ok {
			return result
		}
	}

	// Also check by agent_id
	for _, agentRaw := range activeAgents {
		agent, _ := agentRaw.(map[string]interface{})
		if agent["agent_id"] == proc.agentID {
			if result, ok := agent["result"].(string); ok {
				return result
			}
		}
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

	// Complete phase
	s.completePhase(req.ProjectID, req.TicketID, req.WorkflowName, phase, result)

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
func (s *Spawner) Preview(agentType, ticketID, workflowName string) (string, error) {
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
	return s.loadTemplate(agentType, ticketID, "preview-parent", "preview-child", workflowName, modelID)
}

// loadTemplate loads and expands an agent template from project-local path
func (s *Spawner) loadTemplate(agentType, ticketID, parentSession, childSession, workflowName, modelID string) (string, error) {
	if s.config.ProjectRoot == "" {
		return "", fmt.Errorf("project root required for template loading")
	}

	// Project template required (no fallback)
	projectPath := filepath.Join(s.config.ProjectRoot, ".claude", "nrworkflow", "agents", agentType+".md")
	data, err := os.ReadFile(projectPath)
	if err != nil {
		return "", fmt.Errorf("template not found: %s. Create it at %s", agentType, projectPath)
	}
	template := string(data)

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

	return template, nil
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
		subtype, _ := data["subtype"].(string)
		if subtype != "" {
			s.trackStat(proc, "result:"+subtype)
		}

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
		// Tool result from opencode - just track it
		s.trackStat(proc, "tool_result")

	case "step_finish":
		// Step completion from opencode
		part, _ := data["part"].(map[string]interface{})
		if part != nil {
			reason, _ := part["reason"].(string)
			if reason != "" && reason != "tool-calls" {
				s.trackStat(proc, "step:"+reason)
			}
		}

	case "finish":
		// Session finish from opencode
		s.trackStat(proc, "finish")

	// === Codex CLI format ===
	case "thread.started":
		// Session start from codex
		s.trackStat(proc, "thread_started")

	case "turn.started":
		// Turn start from codex
		s.trackStat(proc, "turn_started")

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
				s.trackStat(proc, "tool_result")
			}
		}

	case "turn.completed":
		// Turn completion from codex with usage stats
		s.trackStat(proc, "turn_completed")
	}

	// Mark stats as dirty for rate-limited flush
	proc.statsDirty = true
}

// handleTextMessage processes text output from either Claude or opencode
func (s *Spawner) handleTextMessage(proc *processInfo, text string) {
	s.trackStat(proc, "text")
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

	// Track stat and message
	s.trackStat(proc, "tool:"+toolName)
	s.trackMessage(proc, toolDetail)

	// Print to console with prefix
	prefix := s.formatPrefix(proc)
	fmt.Printf("  %s %s\n", prefix, toolDetail)
}

// trackStat increments the count for a stat key
func (s *Spawner) trackStat(proc *processInfo, key string) {
	proc.statsMutex.Lock()
	defer proc.statsMutex.Unlock()
	proc.stats[key]++
}

// trackMessage adds a message to the rolling buffer
func (s *Spawner) trackMessage(proc *processInfo, msg string) {
	proc.messagesMutex.Lock()
	defer proc.messagesMutex.Unlock()
	proc.messages = append(proc.messages, msg)
	// Keep only the last N messages
	if len(proc.messages) > s.maxMessages {
		proc.messages = proc.messages[len(proc.messages)-s.maxMessages:]
	}
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

// saveStats saves the accumulated stats and messages to the database
func (s *Spawner) saveStats(proc *processInfo) {
	// Copy stats
	proc.statsMutex.Lock()
	statsCopy := make(map[string]int)
	for k, v := range proc.stats {
		statsCopy[k] = v
	}
	proc.statsMutex.Unlock()

	// Copy messages
	proc.messagesMutex.Lock()
	messagesCopy := make([]string, len(proc.messages))
	copy(messagesCopy, proc.messages)
	proc.messagesMutex.Unlock()

	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	sessionRepo := repo.NewAgentSessionRepo(database)

	// Save stats if any
	if len(statsCopy) > 0 {
		statsJSON, err := json.Marshal(statsCopy)
		if err == nil {
			sessionRepo.UpdateStats(proc.sessionID, string(statsJSON))
		}
	}

	// Save messages if any
	if len(messagesCopy) > 0 {
		messagesJSON, err := json.Marshal(messagesCopy)
		if err == nil {
			sessionRepo.UpdateMessages(proc.sessionID, string(messagesJSON))
		}
	}
}

// registerAgentStart registers the start of an agent with spawn context
func (s *Spawner) registerAgentStart(projectID, ticketID, workflowName, agentID, agentType string, pid int, sessionID, modelID, phase, spawnCommand, promptContext string) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	ticketRepo := repo.NewTicketRepo(database)
	ticket, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		return
	}

	var allState map[string]interface{}
	if ticket.AgentsState.Valid {
		json.Unmarshal([]byte(ticket.AgentsState.String), &allState)
	}
	if allState == nil {
		return
	}

	stateRaw, ok := allState[workflowName]
	if !ok {
		return
	}
	state, _ := stateRaw.(map[string]interface{})

	activeAgents, _ := state["active_agents"].(map[string]interface{})
	if activeAgents == nil {
		activeAgents = make(map[string]interface{})
	}

	cli, modelName := parseModelID(modelID)
	key := agentType + ":" + modelID

	activeAgents[key] = map[string]interface{}{
		"agent_id":   agentID,
		"agent_type": agentType,
		"model_id":   modelID,
		"cli":        cli,
		"model":      modelName,
		"pid":        pid,
		"session_id": sessionID,
		"started_at": time.Now().UTC().Format(time.RFC3339),
		"result":     nil,
	}

	state["active_agents"] = activeAgents
	allState[workflowName] = state

	stateJSON, _ := json.Marshal(allState)
	stateStr := string(stateJSON)
	fields := &repo.UpdateFields{AgentsState: &stateStr}
	ticketRepo.Update(projectID, ticketID, fields)

	// Create agent session record for API access (with spawn context)
	sessionRepo := repo.NewAgentSessionRepo(database)
	session := &model.AgentSession{
		ID:            sessionID,
		ProjectID:     projectID,
		TicketID:      ticketID,
		Phase:         phase,
		Workflow:      workflowName,
		AgentType:     agentType,
		ModelID:       sql.NullString{String: modelID, Valid: modelID != ""},
		Status:        model.AgentSessionRunning,
		SpawnCommand:  sql.NullString{String: spawnCommand, Valid: spawnCommand != ""},
		PromptContext: sql.NullString{String: promptContext, Valid: promptContext != ""},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	sessionRepo.Create(session)
}

// registerAgentStop registers the stop of an agent (backward-compatible wrapper)
func (s *Spawner) registerAgentStop(projectID, ticketID, workflowName, agentID, result, modelID string) {
	s.registerAgentStopWithReason(projectID, ticketID, workflowName, agentID, result, "", modelID)
}

// registerAgentStopWithReason registers the stop of an agent with result reason
func (s *Spawner) registerAgentStopWithReason(projectID, ticketID, workflowName, agentID, result, resultReason, modelID string) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	ticketRepo := repo.NewTicketRepo(database)
	ticket, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		return
	}

	var allState map[string]interface{}
	if ticket.AgentsState.Valid {
		json.Unmarshal([]byte(ticket.AgentsState.String), &allState)
	}
	if allState == nil {
		return
	}

	stateRaw, ok := allState[workflowName]
	if !ok {
		return
	}
	state, _ := stateRaw.(map[string]interface{})

	activeAgents, _ := state["active_agents"].(map[string]interface{})
	history, _ := state["agent_history"].([]interface{})
	if history == nil {
		history = []interface{}{}
	}

	sessionRepo := repo.NewAgentSessionRepo(database)
	var sessionID string

	// Find and remove agent
	for key, agentRaw := range activeAgents {
		agent, _ := agentRaw.(map[string]interface{})
		if agent["agent_id"] == agentID || strings.Contains(key, modelID) {
			// Capture session ID for status update
			if sid, ok := agent["session_id"].(string); ok {
				sessionID = sid
			}
			historyEntry := map[string]interface{}{
				"agent_id":      agent["agent_id"],
				"agent_type":    agent["agent_type"],
				"model_id":      agent["model_id"],
				"phase":         state["current_phase"],
				"started_at":    agent["started_at"],
				"ended_at":      time.Now().UTC().Format(time.RFC3339),
				"result":        result,
				"result_reason": resultReason,
			}
			history = append(history, historyEntry)
			delete(activeAgents, key)
			break
		}
	}

	state["active_agents"] = activeAgents
	state["agent_history"] = history
	allState[workflowName] = state

	stateJSON, _ := json.Marshal(allState)
	stateStr := string(stateJSON)
	fields := &repo.UpdateFields{AgentsState: &stateStr}
	ticketRepo.Update(projectID, ticketID, fields)

	// Update agent session status
	if sessionID != "" {
		status := model.AgentSessionCompleted
		if result == "fail" || result == "timeout" {
			status = model.AgentSessionFailed
		}
		sessionRepo.UpdateStatus(sessionID, status)
	}
}

// validateWorkflowInitialized checks if the workflow is initialized on the ticket
func (s *Spawner) validateWorkflowInitialized(projectID, ticketID, workflowName string) error {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	ticketRepo := repo.NewTicketRepo(database)
	ticket, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		return fmt.Errorf("ticket '%s' not found in project '%s'", ticketID, projectID)
	}

	if !ticket.AgentsState.Valid || ticket.AgentsState.String == "" {
		return fmt.Errorf("workflow '%s' not initialized on ticket '%s'. Run: nrworkflow workflow init %s -w %s",
			workflowName, ticketID, ticketID, workflowName)
	}

	var allState map[string]interface{}
	if err := json.Unmarshal([]byte(ticket.AgentsState.String), &allState); err != nil {
		return fmt.Errorf("invalid agents_state on ticket '%s': %w", ticketID, err)
	}

	if _, ok := allState[workflowName]; !ok {
		return fmt.Errorf("workflow '%s' not initialized on ticket '%s'. Run: nrworkflow workflow init %s -w %s",
			workflowName, ticketID, ticketID, workflowName)
	}

	return nil
}

// validateAndAdvancePhase validates phase order and auto-skips phases with matching skip_for rules.
// Returns (phaseID, shouldSkip, error).
func (s *Spawner) validateAndAdvancePhase(projectID, ticketID, workflowName, requestedAgent string) (string, bool, error) {
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

	// Get current state
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return "", false, fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	ticketRepo := repo.NewTicketRepo(database)
	ticket, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		return "", false, err
	}

	if !ticket.AgentsState.Valid {
		return "", false, fmt.Errorf("workflow not initialized")
	}

	var allState map[string]interface{}
	if err := json.Unmarshal([]byte(ticket.AgentsState.String), &allState); err != nil {
		return "", false, err
	}

	stateRaw, ok := allState[workflowName]
	if !ok {
		return "", false, fmt.Errorf("workflow '%s' not found in state", workflowName)
	}
	state, _ := stateRaw.(map[string]interface{})

	phases, _ := state["phases"].(map[string]interface{})
	if phases == nil {
		phases = make(map[string]interface{})
	}

	category, _ := state["category"].(string)

	// Check if requested phase should be skipped
	if s.categoryMatchesSkipFor(category, requestedPhase.SkipFor) {
		// Mark this phase as skipped
		s.completePhase(projectID, ticketID, workflowName, requestedPhase.ID, "skipped")
		return requestedPhase.ID, true, nil
	}

	// Validate that prior phases are completed or skipped
	for i := 0; i < requestedIndex; i++ {
		priorPhase := workflow.Phases[i]
		phaseState, _ := phases[priorPhase.ID].(map[string]interface{})

		status := ""
		if phaseState != nil {
			status, _ = phaseState["status"].(string)
		}

		// Check if phase is completed or skipped
		if status == "completed" {
			continue
		}

		// Check if phase can be auto-skipped due to category
		if s.categoryMatchesSkipFor(category, priorPhase.SkipFor) {
			// Auto-skip this phase
			s.completePhase(projectID, ticketID, workflowName, priorPhase.ID, "skipped")
			continue
		}

		// Phase must complete before we can proceed
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

// startPhase marks a phase as in_progress
func (s *Spawner) startPhase(projectID, ticketID, workflowName, phase string) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	ticketRepo := repo.NewTicketRepo(database)
	ticket, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		return
	}

	var allState map[string]interface{}
	if ticket.AgentsState.Valid {
		json.Unmarshal([]byte(ticket.AgentsState.String), &allState)
	}
	if allState == nil {
		return
	}

	stateRaw, ok := allState[workflowName]
	if !ok {
		return
	}
	state, _ := stateRaw.(map[string]interface{})

	phases, _ := state["phases"].(map[string]interface{})
	if phases == nil {
		phases = make(map[string]interface{})
	}

	phaseState, _ := phases[phase].(map[string]interface{})
	if phaseState == nil {
		phaseState = make(map[string]interface{})
	}

	// Only start if pending
	if phaseState["status"] == nil || phaseState["status"] == "pending" {
		phaseState["status"] = "in_progress"
		phases[phase] = phaseState
		state["phases"] = phases
		state["current_phase"] = phase
		allState[workflowName] = state

		stateJSON, _ := json.Marshal(allState)
		stateStr := string(stateJSON)
		fields := &repo.UpdateFields{AgentsState: &stateStr}
		ticketRepo.Update(projectID, ticketID, fields)
	}
}

// completePhase marks a phase as completed
func (s *Spawner) completePhase(projectID, ticketID, workflowName, phase, result string) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	ticketRepo := repo.NewTicketRepo(database)
	ticket, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		return
	}

	var allState map[string]interface{}
	if ticket.AgentsState.Valid {
		json.Unmarshal([]byte(ticket.AgentsState.String), &allState)
	}
	if allState == nil {
		return
	}

	stateRaw, ok := allState[workflowName]
	if !ok {
		return
	}
	state, _ := stateRaw.(map[string]interface{})

	phases, _ := state["phases"].(map[string]interface{})
	if phases == nil {
		return
	}

	phaseState, _ := phases[phase].(map[string]interface{})
	if phaseState == nil {
		phaseState = make(map[string]interface{})
	}

	phaseState["status"] = "completed"
	phaseState["result"] = result
	phases[phase] = phaseState
	state["phases"] = phases
	allState[workflowName] = state

	stateJSON, _ := json.Marshal(allState)
	stateStr := string(stateJSON)
	fields := &repo.UpdateFields{AgentsState: &stateStr}
	ticketRepo.Update(projectID, ticketID, fields)
}

func parseModelID(modelID string) (cli, model string) {
	if modelID == "" || !strings.Contains(modelID, ":") {
		return "claude", modelID
	}
	parts := strings.SplitN(modelID, ":", 2)
	return parts[0], parts[1]
}
