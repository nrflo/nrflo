package spawner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	manifestConfig "be/internal/manifest/config"
	"be/internal/manifest/python"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/anthropic"
	"be/internal/spawner/apirun/tools_builtin"
	"be/internal/spawner/apirun/tools_http"
	"be/internal/spawner/apirun/tools_manifest"
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
	Model            string `json:"model"`
	Timeout          int    `json:"timeout"`
	ExecutionMode    string `json:"execution_mode"`
	Tools            string `json:"tools"`
	APIMaxIterations *int   `json:"api_max_iterations"`
}

// ErrorRecorder records error events. Implemented by service.ErrorService.
type ErrorRecorder interface {
	RecordError(projectID, errorType, instanceID, message string) error
}

const (
	defaultMaxContinuations       = 10
	defaultContextThreshold       = 25
	defaultFailRetryDelay         = 15 * time.Second
	defaultStallStartTimeout      = 2 * time.Minute
	defaultStallRunningTimeout    = 8 * time.Minute
	maxStallRestarts              = 15
	defaultIdleAfterMessageTimeout = 3 * time.Minute
	defaultIdleStartTimeout        = 2 * time.Minute
	defaultNudgeMax                = 5

	defaultAPIMaxIterations = 50
	defaultAPIMaxTokens     = 4096
	defaultAPISystemPrompt  = "You are an agent in a workflow. Follow the instructions below."
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
	// Provider is the provider abstraction used by API-mode agents
	// (execution_mode='api'). Required when any agent definition selects api mode.
	Provider provider.Provider
	// AgentSvc persists context_left for API-mode agents (mirrors what the
	// CLI hook does for CLI agents).
	AgentSvc apirun.AgentSvc
	// APICredentialRepo resolves provider API keys for API-mode agents.
	APICredentialRepo anthropic.APICredentialRepo
	// FindingsSvc, ProjectFindingsSvc, AgentSvcReal, WorkflowSvc are used by
	// API-mode tool builtins (findings_*, project_findings_*, agent_*,
	// workflow_skip). They mirror the services the socket handler uses for
	// CLI agents so WS event parity is automatic.
	FindingsSvc        *service.FindingsService
	ProjectFindingsSvc *service.ProjectFindingsService
	AgentSvcReal       *service.AgentService
	WorkflowSvc        *service.WorkflowService
	// ToolDefRepo lists HTTP tool definitions for API-mode registry resolution.
	ToolDefRepo *repo.ToolDefinitionRepo
	// APIMode enables execution_mode='api' agents. When false (default, --mode=cli),
	// prepareSpawn rejects any agent with execution_mode='api' before making any provider call.
	APIMode bool
	// PTYManager manages PTY sessions for cli_interactive agents.
	PTYManager *ptyPkg.Manager
	// IdleAfterMessageTimeoutSec: idle window after last message before nudge (default 180s, 0 = use default).
	// Only applies to cliInteractiveBackend agents.
	IdleAfterMessageTimeoutSec int
	// IdleStartTimeoutSec: idle window before first message before nudge (default 120s, 0 = use default).
	// Only applies to cliInteractiveBackend agents.
	IdleStartTimeoutSec int
	// NudgeMax: max nudge attempts before auto-fail (default 5, 0 = use default).
	// Only applies to cliInteractiveBackend agents.
	NudgeMax int
	// DispatchRepo records tool dispatch events for manifest tools.
	// Optional (nil-safe): when nil, dispatch rows are not inserted.
	DispatchRepo *repo.DispatchRepo
	// ReviewRepo stores review items created by manifest tools with review:true.
	// Optional (nil-safe): when nil, review rows are not inserted.
	ReviewRepo *repo.ReviewRepo
	// PythonRunner executes Python scripts for manifest tools.
	// Optional (nil-safe): when nil, manifest tools are unavailable even if APIMode is set.
	PythonRunner python.Runner
	// CustomerConfigDir is the absolute path to the customer config directory for this project.
	// Used to load tool_manifest.yaml when APIMode is true and the dir is non-empty.
	CustomerConfigDir string
	// ProjectEnv holds per-project env vars as "KEY=value" strings, loaded once at workflow
	// start from project_env_vars. Appended after nrflo-controlled vars in every spawn path
	// so duplicates resolve last-wins (nrflo reserved names are also guarded at the service layer).
	ProjectEnv []string
	// OnSessionRegister is called after registerTerminalSignal adds sessionID to the registry.
	// The callback fires outside terminalSignalsMu to avoid lock-order inversion.
	// The orchestrator uses this to maintain its sessionID→*Spawner index.
	OnSessionRegister func(sessionID string, sp *Spawner)
	// OnSessionUnregister is called after unregisterTerminalSignal removes sessionID.
	// Fires outside terminalSignalsMu. Used symmetrically with OnSessionRegister.
	OnSessionUnregister func(sessionID string)
	// SDKDir is the absolute path to the nrflo SDK directory (NRFLO_HOME/sdk).
	// When non-empty, NRFLO_SDK_DIR is injected into script-mode agent environments.
	// T3 writes the embedded SDK file; T2 only plumbs the directory.
	SDKDir string
	// PythonPath is the absolute path to the python binary in the project venv.
	// When non-empty, script-mode agents use this instead of "python3" from PATH.
	// Empty string means fall back to "python3" on PATH.
	PythonPath string
	// PythonScriptRepo loads python_scripts rows for script-mode agents.
	// Required when any agent definition uses execution_mode='script'.
	PythonScriptRepo *repo.PythonScriptRepo
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
	backend       ExecutionBackend
	pid           int // OS pid; set by backends when proc.cmd is nil (e.g. PTY-owned process)
	// env is the full process env assembled in prepareSpawn (nrflo-controlled vars +
	// per-project vars). Stored separately from cmd.Env so contextSaveViaResume can
	// reach it for PTY-owned processes where cmd is nil by design.
	env []string
	// sessionStartCh is closed (idempotently) when Claude's SessionStart hook
	// fires — the canonical readiness signal. firstByteCh is closed on the
	// first non-empty PTY read — used only as a fallback when SessionStart
	// does not arrive (older Claude builds, codex/opencode without hooks).
	// deliverPrompt prefers sessionStartCh; firstByteCh + a bootstrap floor
	// only kick in if SessionStart never appears within ~3s.
	sessionStartCh   chan struct{}
	sessionStartOnce sync.Once
	firstByteCh      chan struct{}
	firstByteOnce    sync.Once
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
	spawnCommand string
	prompt       string // rendered user prompt body
	systemPrompt string // rendered system-prompt-suffix delivered to the agent
	spawnToken   string // bearer token injected into env for HTTP API auth (valid while session is running)
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
	// Idle/nudge detection (cli_interactive backend only; nudgeMax=0 means disabled)
	nudgeCount             int
	nudgeMax               int
	idleAfterMessageTimeout time.Duration
	idleStartTimeout       time.Duration
	lastNudgeAt            time.Time
	// External session ID (e.g., codex thread_id) — for logging only
	externalSessionID string
	// Callback level set by API-mode agent_callback handler. Mirrors the
	// callback_level finding written by AgentService.Callback for CLI agents.
	callbackLevel int
	// Transaction ID for structured logging (from orchestrator context)
	trx string
}

// terminalSignal is routed via the per-session terminalSignals registry to kill
// an agent immediately so handleCompletion reads the DB-written result
// (fail/continue/callback).
type terminalSignal struct {
	SessionID string
	Result    string
}

// manifestCacheEntry caches a loaded manifest keyed by its directory path.
type manifestCacheEntry struct {
	mtime    time.Time
	manifest *manifestConfig.Manifest
}

// Spawner manages agent lifecycle
type Spawner struct {
	config             Config
	restartCh          chan string             // carries sessionID of agent to restart
	takeControlCh      chan string             // carries sessionID of agent to take control of
	takeControlReadies map[string]chan struct{} // sessionID → closed when status is user_interactive (or take-control rejected/attached)
	takeControlReadiesMu sync.Mutex             // protects takeControlReadies
	terminalSignals    map[string]chan terminalSignal // sessionID → its monitorAll's receive channel
	terminalSignalsMu  sync.Mutex              // protects terminalSignals
	bumpMessageCh      chan string             // carries sessionID to bump lastMessageTime (hook events)
	interactiveWaits   map[string]chan struct{} // sessionID → closed when interactive session completes
	mu                 sync.Mutex              // protects interactiveWaits
	sessionProcsMu     sync.Mutex              // protects sessionProcs
	sessionProcs       map[string]*processInfo // sessionID → live proc for RecordUserInput lookups
	manifestCache      map[string]*manifestCacheEntry // configDir → cached manifest (mtime-keyed)
	manifestCacheMu    sync.Mutex                     // protects manifestCache
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
		config:             config,
		restartCh:          make(chan string, 1),
		takeControlCh:      make(chan string, 1),
		takeControlReadies: make(map[string]chan struct{}),
		terminalSignals:    make(map[string]chan terminalSignal),
		bumpMessageCh:      make(chan string, 1),
		interactiveWaits:   make(map[string]chan struct{}),
		sessionProcs:       make(map[string]*processInfo),
		manifestCache:      make(map[string]*manifestCacheEntry),
	}
}

// loadManifestCached loads and caches tool_manifest.yaml from configDir. It
// reloads only when the file's mtime has changed since the last load.
// Returns (nil, nil) when the manifest file does not exist.
func (s *Spawner) loadManifestCached(configDir string) (*manifestConfig.Manifest, error) {
	manifestPath := filepath.Join(configDir, "tool_manifest.yaml")
	info, statErr := os.Stat(manifestPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, nil
		}
		return nil, statErr
	}
	mtime := info.ModTime()

	s.manifestCacheMu.Lock()
	entry, ok := s.manifestCache[configDir]
	if ok && !entry.mtime.Before(mtime) {
		m := entry.manifest
		s.manifestCacheMu.Unlock()
		return m, nil
	}
	s.manifestCacheMu.Unlock()

	m, loadErr := manifestConfig.Load(configDir)
	if loadErr != nil {
		return nil, loadErr
	}

	s.manifestCacheMu.Lock()
	s.manifestCache[configDir] = &manifestCacheEntry{mtime: mtime, manifest: m}
	s.manifestCacheMu.Unlock()
	return m, nil
}

// RequestRestart sends a restart signal for the given session ID.
// Non-blocking: if a restart is already pending, this is a no-op.
func (s *Spawner) RequestRestart(sessionID string) {
	select {
	case s.restartCh <- sessionID:
	default:
	}
}

// RequestTakeControl sends a take-control signal for the given session ID and
// registers a readiness channel that closes once monitorAll has finished
// killing the agent and flipped the session to user_interactive (or rejected
// the take-control request, or attached as a viewer for cli_interactive).
// Callers can wait for readiness via WaitForTakeControlReady. Non-blocking on
// the channel send: if a take-control is already pending, the existing
// readiness entry is reused.
func (s *Spawner) RequestTakeControl(sessionID string) {
	s.takeControlReadiesMu.Lock()
	if _, exists := s.takeControlReadies[sessionID]; !exists {
		s.takeControlReadies[sessionID] = make(chan struct{})
	}
	s.takeControlReadiesMu.Unlock()

	select {
	case s.takeControlCh <- sessionID:
	default:
	}
}

// WaitForTakeControlReady blocks until the take-control flow for the given
// session ID has completed its synchronous setup (kill + status flip to
// user_interactive, or reject, or viewer-attach), or until the timeout
// elapses. Returns true if the ready signal fired, false on timeout or when
// no readiness was registered for this session.
func (s *Spawner) WaitForTakeControlReady(sessionID string, timeout time.Duration) bool {
	s.takeControlReadiesMu.Lock()
	ch, ok := s.takeControlReadies[sessionID]
	s.takeControlReadiesMu.Unlock()
	if !ok {
		return false
	}
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

// signalTakeControlReady closes the readiness channel for the given session
// (idempotent) and removes it from the map. Called by monitorAll once the
// take-control flow has reached a state where a PTY connection can succeed.
func (s *Spawner) signalTakeControlReady(sessionID string) {
	s.takeControlReadiesMu.Lock()
	ch, ok := s.takeControlReadies[sessionID]
	if ok {
		delete(s.takeControlReadies, sessionID)
	}
	s.takeControlReadiesMu.Unlock()
	if ok {
		close(ch)
	}
}

// RequestTerminalSignal kills the matching agent so monitorAll exits the
// natural-exit wait and handleCompletion reads the DB result already written
// by the socket handler. Routes the signal to the specific monitorAll
// goroutine that owns this sessionID, so concurrent monitorAlls cannot
// steal each other's signals. No-op if the session is not registered
// (already finished or never started). Non-blocking on the channel send.
func (s *Spawner) RequestTerminalSignal(sessionID, result string) {
	s.terminalSignalsMu.Lock()
	ch, ok := s.terminalSignals[sessionID]
	s.terminalSignalsMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- terminalSignal{SessionID: sessionID, Result: result}:
	default:
	}
}

// registerTerminalSignal binds sessionID to ch in the registry so
// RequestTerminalSignal(sessionID, ...) routes to ch. Used by monitorAll
// at start and on continuation-relaunch to track new session IDs.
func (s *Spawner) registerTerminalSignal(sessionID string, ch chan terminalSignal) {
	s.terminalSignalsMu.Lock()
	s.terminalSignals[sessionID] = ch
	s.terminalSignalsMu.Unlock()
	// Fire callback outside the mutex to avoid lock-order inversion
	// (callback acquires orchestrator's mu; terminalSignalsMu must not be held).
	if s.config.OnSessionRegister != nil {
		s.config.OnSessionRegister(sessionID, s)
	}
}

// unregisterTerminalSignal removes sessionID from the registry. Subsequent
// RequestTerminalSignal calls for this sessionID become no-ops.
func (s *Spawner) unregisterTerminalSignal(sessionID string) {
	s.terminalSignalsMu.Lock()
	delete(s.terminalSignals, sessionID)
	s.terminalSignalsMu.Unlock()
	if s.config.OnSessionUnregister != nil {
		s.config.OnSessionUnregister(sessionID)
	}
}

// BumpLastMessage sends a non-blocking signal to monitorAll to update
// lastMessageTime and hasReceivedMessage for the matching proc. Used by the
// socket handler to reset stall detection when hook events arrive for
// interactive CLI agents. Silently dropped when channel is full.
func (s *Spawner) BumpLastMessage(sessionID string) {
	select {
	case s.bumpMessageCh <- sessionID:
	default:
	}
}

// SetLastMessage updates proc.lastMessage for the matching session so the
// status log line ("agent status ... last_msg=...") shows the most recent
// agent output. Interactive CLI mode otherwise leaves lastMessage empty
// because the PTY ferry drops bytes — hook events / SSE events feed content
// here directly. Also bumps lastMessageTime + hasReceivedMessage (same as
// BumpLastMessage) so stall/idle detection treats this as activity.
// No-op when the session is unknown or content is empty.
func (s *Spawner) SetLastMessage(sessionID, content string) {
	if content == "" {
		return
	}
	proc := s.lookupSessionProc(sessionID)
	if proc == nil {
		return
	}
	proc.messagesMutex.Lock()
	proc.lastMessage = content
	proc.lastMessageTime = s.config.Clock.Now()
	proc.hasReceivedMessage = true
	proc.messagesMutex.Unlock()
}

// MarkSessionReady closes the matching proc's sessionStartCh — the canonical
// TUI-ready signal from Claude's SessionStart hook. Idempotent. Called by the
// socket handler when SessionStart arrives.
func (s *Spawner) MarkSessionReady(sessionID string) {
	proc := s.lookupSessionProc(sessionID)
	if proc == nil || proc.sessionStartCh == nil {
		return
	}
	proc.sessionStartOnce.Do(func() {
		s.logAgent(proc, "ready signal: SessionStart hook")
		close(proc.sessionStartCh)
	})
}

// CompleteInteractive signals that the interactive session has ended,
// unblocking the spawner's monitorAll wait.
func (s *Spawner) CompleteInteractive(sessionID string) {
	s.mu.Lock()
	ch, ok := s.interactiveWaits[sessionID]
	if ok {
		delete(s.interactiveWaits, sessionID)
	}
	s.mu.Unlock()
	if ok {
		select {
		case <-ch:
			// already closed
		default:
			close(ch)
		}
		// Note: OnSessionUnregister is NOT fired here. The orchestrator holds
		// o.mu when it calls this method (iterating rs.spawners), so firing the
		// callback would deadlock. Pre-step spawners remain in rs.spawners as
		// harmless orphans until the runState is GC'd; take-control spawners are
		// cleaned up by unregisterTerminalSignal when monitorAll unblocks.
	}
}

// RegisterInteractiveWait creates and returns a channel that blocks until
// CompleteInteractive is called for the given session ID. Used by the
// orchestrator to wait on interactive/plan PTY sessions before entering
// the layer execution loop. Fires OnSessionRegister so the orchestrator's
// sessionID→*Spawner index includes this spawner for the duration of the wait.
func (s *Spawner) RegisterInteractiveWait(sessionID string) <-chan struct{} {
	ch := make(chan struct{})
	s.mu.Lock()
	s.interactiveWaits[sessionID] = ch
	s.mu.Unlock()
	// Fire outside the mutex (same discipline as registerTerminalSignal) to
	// avoid lock-order inversion with the orchestrator's mu.
	if s.config.OnSessionRegister != nil {
		s.config.OnSessionRegister(sessionID, s)
	}
	return ch
}

// Close is a no-op retained for API compatibility (e.g. orchestrator defer).
func (s *Spawner) Close() {}

// registerSessionProc tracks a live proc by sessionID so RecordUserInput can
// route user keystrokes through the normal TrackMessage pipeline.
func (s *Spawner) registerSessionProc(sessionID string, proc *processInfo) {
	s.sessionProcsMu.Lock()
	s.sessionProcs[sessionID] = proc
	s.sessionProcsMu.Unlock()
}

// lookupSessionProc returns the live proc for sessionID, or nil if unknown.
func (s *Spawner) lookupSessionProc(sessionID string) *processInfo {
	s.sessionProcsMu.Lock()
	defer s.sessionProcsMu.Unlock()
	return s.sessionProcs[sessionID]
}

// unregisterSessionProcs removes completed procs from the session proc map.
func (s *Spawner) unregisterSessionProcs(procs []*processInfo) {
	if len(procs) == 0 {
		return
	}
	s.sessionProcsMu.Lock()
	for _, proc := range procs {
		delete(s.sessionProcs, proc.sessionID)
	}
	s.sessionProcsMu.Unlock()
}

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
	model := "opus_4_7"
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
	proc, err := s.spawnSingle(ctx, req, modelID, phase.ID, wi.ID)
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
	return s.monitorAll(ctx, processes, req, phase.ID)
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

// prepareScriptSpawn handles execution_mode="script" agent prep:
// loads the python_scripts row, builds minimal SpawnOptions with NRF_* env, and
// returns early without template loading or CLI adapter resolution.
func (s *Spawner) prepareScriptSpawn(ctx context.Context, req SpawnRequest, phase, wfiID, agentID, sessionID, spawnToken string, agentDef *model.AgentDefinition) (*processInfo, *prepResult, error) {
	if s.config.PythonScriptRepo == nil {
		return nil, nil, fmt.Errorf("python_script_id_required: PythonScriptRepo not configured")
	}
	if agentDef == nil || agentDef.PythonScriptID == nil {
		return nil, nil, fmt.Errorf("python_script_id_required")
	}

	script, err := s.config.PythonScriptRepo.Get(req.ProjectID, *agentDef.PythonScriptID)
	if err != nil {
		return nil, nil, fmt.Errorf("python_script_not_found: %w", err)
	}

	scriptCode := script.Code
	if script.FilePath != "" {
		if !filepath.IsAbs(script.FilePath) {
			return nil, nil, fmt.Errorf("python_script_file_path_invalid: file_path must be absolute")
		}
		info, err := os.Stat(script.FilePath)
		if err != nil {
			return nil, nil, fmt.Errorf("python_script_file_path_invalid: %w", err)
		}
		if !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("python_script_file_path_invalid: file_path must be a regular file")
		}
		if !strings.HasSuffix(script.FilePath, ".py") {
			return nil, nil, fmt.Errorf("python_script_file_path_invalid: file_path must end in .py")
		}
		data, err := os.ReadFile(script.FilePath)
		if err != nil {
			return nil, nil, fmt.Errorf("python_script_file_path_read: %w", err)
		}
		scriptCode = string(data)
	}

	// Resolve timeout from agent config or agent definition.
	timeout := 40
	if agentCfg, ok := s.config.Agents[req.AgentType]; ok && agentCfg.Timeout > 0 {
		timeout = agentCfg.Timeout
	}
	if agentDef.Timeout > 0 {
		timeout = agentDef.Timeout
	}

	// Stall settings: stall_start disabled by default for scripts.
	stallStartTimeout := time.Duration(0)
	stallRunningTimeout := defaultStallRunningTimeout
	if agentDef.StallStartTimeoutSec != nil {
		if *agentDef.StallStartTimeoutSec == 0 {
			stallStartTimeout = 0
		} else {
			stallStartTimeout = time.Duration(*agentDef.StallStartTimeoutSec) * time.Second
		}
	}
	if agentDef.StallRunningTimeoutSec != nil {
		if *agentDef.StallRunningTimeoutSec == 0 {
			stallRunningTimeout = 0
		} else {
			stallRunningTimeout = time.Duration(*agentDef.StallRunningTimeoutSec) * time.Second
		}
	}

	workDir := s.config.ProjectRoot
	if workDir == "" || workDir == "." {
		workDir = ""
	}

	modelID := "script:" + script.ID

	env := append(filterEnv(os.Environ(), "CLAUDECODE"),
		fmt.Sprintf("NRFLO_PROJECT=%s", req.ProjectID),
		fmt.Sprintf("NRF_WORKFLOW_INSTANCE_ID=%s", wfiID),
		fmt.Sprintf("NRF_SESSION_ID=%s", sessionID),
		fmt.Sprintf("NRFLO_AGENT_TOKEN=%s", spawnToken),
		fmt.Sprintf("NRF_TRX=%s", logger.TrxFromContext(ctx)),
		"NRF_SPAWNED=1",
	)
	env = append(env, s.config.ProjectEnv...)

	proc := &processInfo{
		agentID:             agentID,
		agentType:           req.AgentType,
		modelID:             modelID,
		sessionID:           sessionID,
		spawnToken:          spawnToken,
		startTime:           s.config.Clock.Now(),
		timeout:             time.Duration(timeout) * time.Minute,
		pendingMessages:     make([]repo.MessageEntry, 0),
		pendingTasks:        make(map[string]taskInfo),
		doneCh:              make(chan struct{}),
		sessionStartCh:      make(chan struct{}),
		firstByteCh:         make(chan struct{}),
		lastMessagesFlush:   s.config.Clock.Now(),
		projectID:           req.ProjectID,
		ticketID:            req.TicketID,
		workflowName:        req.WorkflowName,
		workflowInstanceID:  wfiID,
		lastMessageTime:     s.config.Clock.Now(),
		stallStartTimeout:   stallStartTimeout,
		stallRunningTimeout: stallRunningTimeout,
		maxContext:          0,
		restartThreshold:    defaultContextThreshold,
		env:                 env,
	}

	prep := &prepResult{
		executionMode: "script",
		scriptCode:    scriptCode,
		scriptID:      script.ID,
		pythonPath:    s.config.PythonPath,
		phase:         phase,
		opts: SpawnOptions{
			WorkDir: workDir,
			Env:     env,
		},
	}

	return proc, prep, nil
}

// prepareSpawn does all CLI-agnostic prep work: session/agent IDs, agent-def
// lookup (timeouts, restart threshold, stall settings), template loading,
// prompt file creation, and SpawnOptions assembly. The returned processInfo
// has cmd left nil — startBackend wires up the chosen ExecutionBackend.
// The ctx's trx is threaded into NRF_TRX so socket-driven log lines from
// spawned agents share the workflow's trx.
func (s *Spawner) prepareSpawn(ctx context.Context, req SpawnRequest, modelID, phase, wfiID string) (*processInfo, *prepResult, error) {
	agentID := "spawn-" + uuid.New().String()[:8]
	sessionID := uuid.New().String()
	spawnToken := MintSpawnToken()

	// Parse modelID (cli:model format)
	cliName, model := parseModelID(modelID)
	if cliName == "" {
		cliName = s.cliForModel(model)
		modelID = fmt.Sprintf("%s:%s", cliName, model)
	}

	// Load agent definition early — execution_mode determines whether we
	// resolve a CLI adapter or skip CLI prep for api/script mode.
	agentDef := s.loadAgentDefinition(req.AgentType, req.ProjectID, req.WorkflowName)
	executionMode := "cli"
	if agentDef != nil && (agentDef.ExecutionMode == "api" || agentDef.ExecutionMode == "script" || agentDef.ExecutionMode == "cli_interactive") {
		executionMode = agentDef.ExecutionMode
	} else if agentDef == nil {
		if agentCfg, ok := s.config.Agents[req.AgentType]; ok && (agentCfg.ExecutionMode == "api" || agentCfg.ExecutionMode == "script" || agentCfg.ExecutionMode == "cli_interactive") {
			executionMode = agentCfg.ExecutionMode
		}
	}

	// Script mode: delegate to dedicated prep path (not gated by APIMode).
	if executionMode == "script" {
		return s.prepareScriptSpawn(ctx, req, phase, wfiID, agentID, sessionID, spawnToken, agentDef)
	}

	// Reject api-mode agents when the server was not started with --mode=api.
	if executionMode == "api" && !s.config.APIMode {
		return nil, nil, fmt.Errorf("api_mode_disabled")
	}

	// Get CLI adapter (api/script modes skip this — there is no CLI process)
	var adapter CLIAdapter
	if executionMode == "cli" || executionMode == "cli_interactive" {
		var err error
		adapter, err = GetCLIAdapter(cliName)
		if err != nil {
			return nil, nil, err
		}
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
	agentLayer := 0
	if agentDef != nil {
		agentLayer = agentDef.Layer
	}
	prompt, suffix, err := s.loadTemplate(req.AgentType, req.TicketID, req.ProjectID, req.ParentSession, sessionID, req.WorkflowName, modelID, phase, req.WorkflowInstanceID, req.ExtraVars, agentLayer)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load template: %w", err)
	}

	workDir := s.config.ProjectRoot
	if workDir == "" || workDir == "." {
		workDir = ""
	}

	_, modelName := parseModelID(modelID)
	proc := &processInfo{
		agentID:             agentID,
		agentType:           req.AgentType,
		modelID:             modelID,
		sessionID:           sessionID,
		spawnToken:          spawnToken,
		startTime:           s.config.Clock.Now(),
		timeout:             time.Duration(timeout) * time.Minute,
		pendingMessages:     make([]repo.MessageEntry, 0),
		pendingTasks:        make(map[string]taskInfo),
		doneCh:              make(chan struct{}),
		sessionStartCh:      make(chan struct{}),
		firstByteCh:         make(chan struct{}),
		lastMessagesFlush:   s.config.Clock.Now(),
		prompt:              prompt,
		systemPrompt:        suffix,
		projectID:           req.ProjectID,
		ticketID:            req.TicketID,
		workflowName:        req.WorkflowName,
		workflowInstanceID:  wfiID,
		restartThreshold:    effectiveThreshold,
		maxFailRestarts:     maxFailRestarts,
		lastMessageTime:     s.config.Clock.Now(),
		stallStartTimeout:   stallStartTimeout,
		stallRunningTimeout: stallRunningTimeout,
		maxContext:          s.maxContextForModel(modelName),
	}

	// Populate idle/nudge fields only for cliInteractiveBackend agents.
	if executionMode == "cli_interactive" {
		nudgeMax := defaultNudgeMax
		if s.config.NudgeMax > 0 {
			nudgeMax = s.config.NudgeMax
		}
		proc.nudgeMax = nudgeMax

		idleAfterMsg := defaultIdleAfterMessageTimeout
		if s.config.IdleAfterMessageTimeoutSec > 0 {
			idleAfterMsg = time.Duration(s.config.IdleAfterMessageTimeoutSec) * time.Second
		}
		proc.idleAfterMessageTimeout = idleAfterMsg

		idleStart := defaultIdleStartTimeout
		if s.config.IdleStartTimeoutSec > 0 {
			idleStart = time.Duration(s.config.IdleStartTimeoutSec) * time.Second
		}
		proc.idleStartTimeout = idleStart
	}
	// nudgeMax = 0 (zero value) → disabled for non-interactive backends

	prep := &prepResult{
		cliName:       cliName,
		prompt:        prompt,
		phase:         phase,
		executionMode: executionMode,
	}

	if executionMode == "api" {
		// Resolve the API key up-front so spawn fails fast on misconfiguration
		// (matches the CLI failure mode of a missing binary).
		if _, keyErr := anthropic.ResolveAPIKey(context.Background(), s.config.APICredentialRepo, req.ProjectID); keyErr != nil {
			return nil, nil, fmt.Errorf("api mode: %w", keyErr)
		}

		// Resolve mapped model name for the provider call.
		apiModelID := model
		if cfg, ok := s.config.ModelConfigs[model]; ok && cfg.MappedModel != "" {
			apiModelID = cfg.MappedModel
		}

		maxIter := defaultAPIMaxIterations
		if agentDef != nil && agentDef.APIMaxIterations != nil && *agentDef.APIMaxIterations > 0 {
			maxIter = *agentDef.APIMaxIterations
		} else if agentDef == nil {
			if agentCfg, ok := s.config.Agents[req.AgentType]; ok && agentCfg.APIMaxIterations != nil && *agentCfg.APIMaxIterations > 0 {
				maxIter = *agentCfg.APIMaxIterations
			}
		}
		maxCtx := s.maxContextForModel(modelName)
		if s.config.Provider != nil {
			if pmc := s.config.Provider.MaxContext(apiModelID); pmc > 0 {
				maxCtx = pmc
			}
		}
		proc.maxContext = maxCtx

		// Resolve per-agent tool registry from the CSV. Empty CSV ⇒ text-only.
		toolsCSV := ""
		if agentDef != nil {
			toolsCSV = agentDef.Tools
		} else if agentCfg, ok := s.config.Agents[req.AgentType]; ok {
			toolsCSV = agentCfg.Tools
		}
		httpDefs, defsErr := s.loadAPIHTTPToolDefs(req.ProjectID, req.WorkflowName)
		if defsErr != nil {
			return nil, nil, fmt.Errorf("api mode: load tool defs: %w", defsErr)
		}

		// Load manifest tools when a customer config dir is configured.
		var manifestProv apirun.ManifestProvider
		if s.config.CustomerConfigDir != "" && s.config.PythonRunner != nil {
			manifest, mErr := s.loadManifestCached(s.config.CustomerConfigDir)
			if mErr != nil {
				logger.Warn(ctx, "manifest load failed, skipping manifest tools", "dir", s.config.CustomerConfigDir, "err", mErr)
			} else if manifest != nil {
				manifestProv = tools_manifest.New(
					manifest,
					s.config.PythonRunner,
					req.ProjectID,
					proc.sessionID,
					s.config.DispatchRepo,
					s.config.ReviewRepo,
					s.config.WSHub,
					s.config.Clock,
					s.config.ProjectEnv,
				)
			}
		}

		specs, handlers, regErr := apirun.ResolveRegistry(toolsCSV, tools_builtin.Builtins(), httpDefs, tools_http.New(nil), manifestProv)
		if regErr != nil {
			return nil, nil, fmt.Errorf("api mode: %w", regErr)
		}

		if suffix != "" {
			prep.apiSystem = strings.TrimSpace(defaultAPISystemPrompt + "\n\n" + suffix)
		} else {
			prep.apiSystem = defaultAPISystemPrompt
		}
		prep.apiInitialPrompt = prompt
		prep.apiTools = specs
		prep.apiHandlers = handlers
		prep.apiToolEnv = apirun.ToolEnv{
			Pool:               s.config.Pool,
			WSHub:              s.config.WSHub,
			Clock:              s.config.Clock,
			SessionID:          proc.sessionID,
			AgentID:            proc.agentID,
			AgentType:          req.AgentType,
			ProjectID:          req.ProjectID,
			TicketID:           req.TicketID,
			WorkflowName:       req.WorkflowName,
			WorkflowInstanceID: wfiID,
			Findings:           s.config.FindingsSvc,
			ProjectFindings:    s.config.ProjectFindingsSvc,
			Agent:              s.config.AgentSvcReal,
			Workflow:           s.config.WorkflowSvc,
		}
		prep.apiMaxIterations = maxIter
		prep.apiMaxTokens = defaultAPIMaxTokens
		prep.apiDeadline = proc.startTime.Add(proc.timeout)
		prep.apiModelID = apiModelID
		prep.apiMaxContext = maxCtx
		return proc, prep, nil
	}

	// CLI mode: write prompt to temp file and assemble SpawnOptions.

	// For adapters without system-prompt-file support (Codex, Opencode), prepend
	// the suffix directly into the prompt body so it is delivered via the prompt file.
	promptBody := prompt
	if suffix != "" && !adapter.SupportsSystemPromptFile() {
		promptBody = suffix + "\n\n" + prompt
	}

	// Backends that consume `prep.prompt` directly (cliInteractiveBackend
	// passing the body to PTY stdin or — for codex — to argv) must see the
	// suffix-prepended version too. prep.prompt was set earlier to the bare
	// body for parity with the API backend; overwrite now that promptBody
	// is final.
	prep.prompt = promptBody

	filePrefix := req.TicketID
	if req.IsProjectScope() {
		filePrefix = "project-" + req.ProjectID
	}
	safePrefix := strings.ReplaceAll(filePrefix, "/", "_")
	safePrefix = strings.ReplaceAll(safePrefix, "\\", "_")
	promptFile, err := os.CreateTemp("/tmp/nrflo", fmt.Sprintf("%s-%s-*.md", safePrefix, req.AgentType))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	if _, err := promptFile.WriteString(promptBody); err != nil {
		os.Remove(promptFile.Name())
		return nil, nil, fmt.Errorf("failed to write prompt: %w", err)
	}
	promptFile.Close()

	// For adapters that support --append-system-prompt-file (Claude), write
	// the suffix to a separate temp file so Claude appends it to its system prompt.
	var suffixFilePath string
	if suffix != "" && adapter.SupportsSystemPromptFile() {
		sf, sfErr := os.CreateTemp("/tmp/nrflo", "system-suffix-*.md")
		if sfErr != nil {
			logger.Warn(context.Background(), "failed to create suffix temp file", "error", sfErr)
		} else {
			if _, sfErr = sf.WriteString(suffix); sfErr != nil {
				sf.Close()
				os.Remove(sf.Name())
				logger.Warn(context.Background(), "failed to write suffix temp file", "error", sfErr)
			} else {
				sf.Close()
				suffixFilePath = sf.Name()
			}
		}
	}

	// Initial prompt (skipped for stdin-based adapters — the template IS the full prompt)
	var initialPrompt string
	if !adapter.UsesStdinPrompt() {
		if req.IsProjectScope() {
			initialPrompt = fmt.Sprintf(`Begin working on project %s. Follow the workflow steps in your system prompt.`, req.ProjectID)
		} else {
			initialPrompt = fmt.Sprintf(`Begin working on ticket %s. Follow the workflow steps in your system prompt.`, req.TicketID)
		}
	}

	// DB-sourced mapped model + reasoning effort
	var mappedModel, reasoningEffort string
	if cfg, ok := s.config.ModelConfigs[model]; ok {
		mappedModel = cfg.MappedModel
		reasoningEffort = cfg.ReasoningEffort
	}

	opts := SpawnOptions{
		Model:            model,
		SessionID:        sessionID,
		PromptFile:       promptFile.Name(),
		Prompt:           promptBody,
		InitialPrompt:    initialPrompt,
		WorkDir:          workDir,
		MappedModel:      mappedModel,
		ReasoningEffort:  reasoningEffort,
		SettingsJSON:     s.config.ClaudeSettingsJSON,
		SystemPromptFile: suffixFilePath,
		Env: append(append(filterEnv(os.Environ(), "CLAUDECODE"),
			fmt.Sprintf("NRFLO_PROJECT=%s", req.ProjectID),
			fmt.Sprintf("NRF_WORKFLOW_INSTANCE_ID=%s", wfiID),
			fmt.Sprintf("NRF_SESSION_ID=%s", sessionID),
			fmt.Sprintf("NRFLO_AGENT_TOKEN=%s", spawnToken),
			fmt.Sprintf("NRF_TRX=%s", logger.TrxFromContext(ctx)),
			"NRF_SPAWNED=1",
			fmt.Sprintf("NRF_CONTEXT_THRESHOLD=%d", 100-effectiveThreshold),
			fmt.Sprintf("NRF_MAX_CONTEXT=%d", s.maxContextForModel(model)),
		), s.config.ProjectEnv...),
	}

	prep.adapter = adapter
	prep.opts = opts
	prep.promptFile = promptFile.Name()
	prep.suffixFile = suffixFilePath
	proc.env = opts.Env
	return proc, prep, nil
}

// startBackend selects an ExecutionBackend based purely on prep.executionMode:
//   - "api"            → apiBackend (in-process Anthropic runner)
//   - "script"         → scriptBackend (Python exec.Cmd)
//   - "cli_interactive" → cliInteractiveBackend (PTY)
//   - "cli" / default  → cliBackend (exec.Cmd with structured JSON output)
//
// System agents (conflict-resolver, context-saver) flow through the same
// selector — their execution_mode is sourced from system_agent_definitions.execution_mode.
func (s *Spawner) startBackend(proc *processInfo, prep *prepResult) error {
	var backend ExecutionBackend
	switch prep.executionMode {
	case "api":
		backend = newAPIBackend(s)
	case "script":
		backend = newScriptBackend(s)
	case "cli_interactive":
		backend = newCLIInteractiveBackend(prep.adapter, s, wrapPtyManager(s.config.PTYManager))
	default:
		backend = newCLIBackend(prep.adapter, s)
	}
	proc.backend = backend

	var effectiveMode string
	switch prep.executionMode {
	case "api":
		effectiveMode = "api"
	case "script":
		effectiveMode = "script"
	case "cli_interactive":
		effectiveMode = "cli_interactive"
	default:
		effectiveMode = "cli"
	}

	// Register sessionProc BEFORE backend.Start so a fast SessionStart hook
	// (or any other socket lookup keyed by sessionID) can find the proc the
	// moment Claude posts back, not after we've returned from Start.
	s.registerSessionProc(proc.sessionID, proc)

	if err := backend.Start(context.Background(), proc, prep); err != nil {
		s.unregisterSessionProcs([]*processInfo{proc})
		return err
	}

	pid := proc.pid
	if proc.cmd != nil && proc.cmd.Process != nil {
		pid = proc.cmd.Process.Pid
	}
	s.registerAgentStart(proc.projectID, proc.ticketID, proc.workflowName, proc.workflowInstanceID,
		proc.agentID, proc.agentType, pid, proc.sessionID, proc.modelID, prep.phase,
		proc.spawnCommand, proc.prompt, proc.systemPrompt, "", proc.spawnToken, effectiveMode, 0, proc.restartThreshold)
	return nil
}

// monitorAll monitors all spawned processes until completion.
func (s *Spawner) monitorAll(ctx context.Context, processes []*processInfo, req SpawnRequest, phase string) error {
	const statusInterval = 30 * time.Second
	lastStatusTime := time.Time{}

	running := make([]*processInfo, len(processes))
	copy(running, processes)
	var completed []*processInfo

	// Per-monitorAll terminal-signal channel. Each session registered against
	// this channel routes its RequestTerminalSignal sends here, so concurrent
	// monitorAll goroutines cannot steal each other's signals.
	// Buffer large enough for all initial procs plus a margin for relaunches.
	ownTerminalCh := make(chan terminalSignal, len(processes)+4)
	registeredSessions := make(map[string]struct{}, len(processes))
	for _, proc := range processes {
		s.registerTerminalSignal(proc.sessionID, ownTerminalCh)
		registeredSessions[proc.sessionID] = struct{}{}
	}
	defer func() {
		for sid := range registeredSessions {
			s.unregisterTerminalSignal(sid)
		}
	}()
	// relaunchAndRegister wraps relaunchForContinuation to keep the
	// terminal-signal registry in sync across continuation relaunches:
	// drop the old session ID and bind the new one to ownTerminalCh.
	relaunchAndRegister := func(oldProc *processInfo) (*processInfo, error) {
		newProc, err := s.relaunchForContinuation(ctx, oldProc, req, phase)
		if err != nil {
			return nil, err
		}
		s.unregisterTerminalSignal(oldProc.sessionID)
		delete(registeredSessions, oldProc.sessionID)
		s.registerTerminalSignal(newProc.sessionID, ownTerminalCh)
		registeredSessions[newProc.sessionID] = struct{}{}
		return newProc, nil
	}

	for len(running) > 0 {
		// Check for context cancellation or manual restart signal
		select {
		case <-ctx.Done():
			// Kill all running processes
			logger.Warn(ctx, "agents cancelled", "count", len(running))
			for _, proc := range running {
				proc.backend.Kill(ctx, proc, syscall.SIGTERM)
			}
			// Wait for each process to exit gracefully (up to 2s each) before SIGKILL.
			// Per-process select avoids a fixed sleep when processes exit quickly.
			for _, proc := range running {
				select {
				case <-proc.doneCh:
				case <-time.After(2 * time.Second):
					proc.backend.Kill(ctx, proc, syscall.SIGKILL)
					<-proc.doneCh
				}
				proc.finalStatus = "CANCELLED"
				s.saveMessages(proc)
				s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
					proc.sessionID, proc.agentID, "fail", "cancelled", proc.modelID)
				completed = append(completed, proc)
			}
			s.unregisterSessionProcs(completed)
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
			tcMatched := false
			for i, proc := range running {
				if proc.sessionID != takeControlSessionID {
					continue
				}
				tcMatched = true
				// Validate backend supports take-control
				if proc.backend == nil || !proc.backend.SupportsTakeControl() {
					cliName, _ := parseModelID(proc.modelID)
					logger.Error(ctx, "take-control: backend does not support take-control", "cli", cliName, "session_id", takeControlSessionID)
					s.broadcast(ws.EventAgentTakeControlRejected, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
						"session_id": proc.sessionID,
						"agent_type": proc.agentType,
						"model_id":   proc.modelID,
						"reason":     "api_mode_unsupported",
					})
					s.signalTakeControlReady(takeControlSessionID)
					break
				}

				// Interactive backend: viewer-attach — broadcast but do NOT kill or block.
				// The agent keeps running; the viewer connects via /api/v1/pty/{session_id}.
				// No exit-interactive call is made on disconnect (completePtyInteractive is skipped).
				if proc.backend.Name() == "cli_interactive" {
					logger.Info(ctx, "take-control: viewer attach (interactive backend)", "session_id", takeControlSessionID)
					s.broadcast(ws.EventAgentViewerAttached, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
						"session_id": proc.sessionID,
						"agent_type": proc.agentType,
						"model_id":   proc.modelID,
					})
					s.signalTakeControlReady(takeControlSessionID)
					break
				}

				logger.Info(ctx, "take-control: killing agent", "session_id", takeControlSessionID)

				// Kill process: SIGTERM → grace → SIGKILL
				proc.backend.Kill(ctx, proc, syscall.SIGTERM)
				gracePeriod := time.Duration(s.config.TimeoutGraceSec) * time.Second
				if gracePeriod == 0 {
					gracePeriod = 5 * time.Second
				}
				select {
				case <-proc.doneCh:
				case <-time.After(gracePeriod):
					proc.backend.Kill(ctx, proc, syscall.SIGKILL)
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

				// Status is now user_interactive and the agent is killed — the
				// PTY handler can safely accept a connection. Unblock any HTTP
				// caller waiting in WaitForTakeControlReady before we settle
				// into the interactive wait.
				s.signalTakeControlReady(takeControlSessionID)

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
			if !tcMatched {
				// Session is not in our running list (already finished, or
				// owned by a different spawner). Unblock any caller waiting
				// on the readiness channel so it doesn't hang to its timeout.
				s.signalTakeControlReady(takeControlSessionID)
			}
		case sig := <-ownTerminalCh:
			// Terminal signal: DB result already written by socket handler.
			// Kill the matching agent so handleCompletion reads it on next iteration.
			// The registry ensures this signal is routed to *our* monitorAll only.
			for _, proc := range running {
				if proc.sessionID != sig.SessionID {
					continue
				}
				logger.Info(ctx, "terminal signal: killing agent", "session_id", sig.SessionID, "result", sig.Result)
				proc.backend.Kill(ctx, proc, syscall.SIGTERM)
				gracePeriod := time.Duration(s.config.TimeoutGraceSec) * time.Second
				if gracePeriod == 0 {
					gracePeriod = 5 * time.Second
				}
				select {
				case <-proc.doneCh:
				case <-time.After(gracePeriod):
					proc.backend.Kill(ctx, proc, syscall.SIGKILL)
					<-proc.doneCh
				}
				// doneCh closed; next loop iteration picks it up via handleCompletion
				break
			}
		case bumpSessionID := <-s.bumpMessageCh:
			// Hook event signal: update lastMessageTime so stall detection is not triggered.
			for _, proc := range running {
				if proc.sessionID == bumpSessionID {
					proc.messagesMutex.Lock()
					proc.lastMessageTime = s.config.Clock.Now()
					proc.hasReceivedMessage = true
					proc.messagesMutex.Unlock()
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

		// Read context_left from DB once per iteration
		readContextLeftFromDB(s.pool(), running)

		// Check each process using doneCh (no double-wait bug)
		var stillRunning []*processInfo
		for _, proc := range running {
			elapsed := time.Since(proc.startTime)

			// Detect low context and initiate save (only for backends that track context)
			if !proc.lowContextSaving && proc.contextLeft > 0 && proc.contextLeft <= proc.restartThreshold &&
				(proc.backend == nil || proc.backend.TracksContext()) {
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
						newProc, err := relaunchAndRegister(proc)
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
						newProc, err := relaunchAndRegister(proc)
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
				// Idle/nudge loop — send reminder or auto-fail unresponsive agent
				s.checkIdleNudge(ctx, proc, req)
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
							newProc, err := relaunchAndRegister(proc)
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
	// Clean up per-session tracking for completed sessions.
	cleanupBroadcastCoalescing(completed)
	s.unregisterSessionProcs(completed)

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

// MintSpawnToken returns a 32-byte random hex token used as the spawned
// agent's HTTP API bearer credential. Persisted in agent_sessions.spawn_token
// and exposed to the agent process via NRFLO_AGENT_TOKEN.
func MintSpawnToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failure is essentially impossible on supported platforms;
		// fall back to a UUID so we never panic. Token uniqueness is the only
		// invariant — uniqueness via UUIDv4 is sufficient.
		return strings.ReplaceAll(uuid.New().String(), "-", "") +
			strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	return hex.EncodeToString(buf)
}

func (s *Spawner) maxContextForModel(model string) int {
	if cfg, ok := s.config.ModelConfigs[model]; ok && cfg.ContextLength > 0 {
		return cfg.ContextLength
	}
	if model == "opus_4_6_1m" || model == "opus_4_7_1m" {
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

// loadAPIHTTPToolDefs returns HTTP tool definitions in scope for an api-mode
// agent. Scope rules: project_id IS NULL or matches projectID, AND workflow_id
// IS NULL or matches workflowName. The repo's ListByProject already applies
// the project filter; this helper additionally filters by workflow scope.
func (s *Spawner) loadAPIHTTPToolDefs(projectID, workflowName string) ([]*model.ToolDefinition, error) {
	if s.config.ToolDefRepo == nil {
		return nil, nil
	}
	all, err := s.config.ToolDefRepo.ListByProject(projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ToolDefinition, 0, len(all))
	for _, def := range all {
		if def.WorkflowID == nil || *def.WorkflowID == "" || strings.EqualFold(*def.WorkflowID, workflowName) {
			out = append(out, def)
		}
	}
	return out, nil
}

func parseModelID(modelID string) (cli, model string) {
	if modelID == "" || !strings.Contains(modelID, ":") {
		return "claude", modelID
	}
	parts := strings.SplitN(modelID, ":", 2)
	return parts[0], parts[1]
}
