package spawner

import (
	"os/exec"
	"sync"
	"time"

	"be/internal/repo"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// WorkflowDef represents a workflow definition (copied from cli for decoupling)
type WorkflowDef struct {
	Description string     `json:"description"`
	ScopeType   string     `json:"scope_type"` // "ticket" or "project"
	Phases      []PhaseDef `json:"phases"`
}

// PhaseDef represents a phase definition. NodeID is the execution identity
// (which slot in the run); Agent is the agent_definitions template key.
// Instructions carries a materialized plan node's per-node prompt data
// (empty for static phases).
type PhaseDef struct {
	NodeID       string `json:"id"`
	Agent        string `json:"agent"`
	Layer        int    `json:"layer"`
	Instructions string `json:"instructions,omitempty"`
}

// AgentConfig holds agent-specific configuration
type AgentConfig struct {
	Model            string  `json:"model"`
	Timeout          int     `json:"timeout"`
	ExecutionMode    string  `json:"execution_mode"`
	Tools            string  `json:"tools"`
	APIMaxIterations *int    `json:"api_max_iterations"`
	APIMaxTokens     *int    `json:"api_max_tokens"`
	ReasoningEffort  *string `json:"reasoning_effort,omitempty"`
}

// ErrorRecorder records error events. Implemented by service.ErrorService.
type ErrorRecorder interface {
	RecordError(projectID, errorType, instanceID, message string) error
}

const (
	defaultMaxContinuations        = 10
	defaultContextThreshold        = 25
	defaultFailRetryDelay          = 15 * time.Second
	defaultStallStartTimeout       = 2 * time.Minute
	defaultStallRunningTimeout     = 8 * time.Minute
	maxStallRestarts               = 15
	defaultIdleAfterMessageTimeout = 4 * time.Minute
	defaultIdleStartTimeout        = 2 * time.Minute
	defaultNudgeMax                = 5
	// codexBetweenTurnsNudgeDelay caps how long the codex app-server backend waits
	// after a turn completes without a finish call before nudging. A completed codex
	// turn with no `agent_finished` is an unambiguous stop (codex never self-continues),
	// so there is no reason to sit out the full idle window the way a PTY agent does.
	codexBetweenTurnsNudgeDelay = 10 * time.Second

	defaultAPIMaxIterations = 50
	defaultAPIMaxTokens     = 16384
	defaultAPISystemPrompt  = "You are an agent in a workflow. Follow the instructions below."
)

// taskInfo tracks an in-flight Task/Agent tool invocation for tool_result correlation
type taskInfo struct {
	toolName     string // original tool name ("Task" or "Agent")
	description  string
	subagentType string
	background   bool
}

// processInfo tracks a single spawned agent process
type processInfo struct {
	cmd     *exec.Cmd
	backend ExecutionBackend
	pid     int // OS pid; set by backends when proc.cmd is nil (e.g. PTY-owned process)
	// env is the full process env assembled in prepareSpawn (nrflo-controlled vars +
	// per-project vars). Stored separately from cmd.Env so context save can
	// reach it for PTY-owned processes where cmd is nil by design.
	env []string
	// sessionStartCh is closed (idempotently) when Claude's SessionStart hook
	// fires — the canonical readiness signal. firstByteCh is closed on the
	// first non-empty PTY read — used only as a fallback when SessionStart
	// does not arrive (older Claude builds, or codex which has no hooks).
	// deliverPrompt prefers sessionStartCh; firstByteCh + a quiescence gate
	// only kick in if SessionStart never appears within ~3s.
	sessionStartCh   chan struct{}
	sessionStartOnce sync.Once
	firstByteCh      chan struct{}
	firstByteOnce    sync.Once
	// lastPTYByteAt is the wall-clock of the most recent PTY chunk read by
	// ferryPTYOutput. Read by deliverPrompt's quiescence gate to wait for
	// the TUI to finish its splash render before submitting the prompt.
	// Protected by messagesMutex. Zero-valued for non-PTY backends.
	lastPTYByteAt   time.Time
	agentID         string
	agentType       string
	nodeID          string // execution identity (which slot in the run); distinct from agentType (template)
	modelID         string
	sessionID       string
	startTime       time.Time
	timeout         time.Duration
	pendingMessages []repo.MessageEntry // messages not yet flushed to DB
	lastMessage     string              // most recent message (for status display)
	messagesMutex   sync.Mutex
	saveMu          sync.Mutex          // serializes saveMessages drain+insert per proc
	pendingTasks    map[string]taskInfo // tool_use_id -> taskInfo for in-flight Task invocations
	finalStatus     string
	elapsed         time.Duration
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
	// Proactive restart-with-digest (watcher-triggered, task-boundary + idle)
	proactiveRestartThreshold int  // Resolved ledger-token ceiling; 0 = disabled
	proactiveRestartCount     int  // How many times this session has proactively rotated
	proactiveRotationPending  bool // Set on the OLD proc before its relaunch; tells relaunchForContinuation to reset restartCount instead of incrementing it
	proactiveTokensBefore     int  // Ledger tokens at the moment the rotation fired (for tokens-before/after logging)
	// Stall detection
	lastMessageTime     time.Time     // set on spawn, updated on every trackMessage()
	hasReceivedMessage  bool          // distinguishes "no messages yet" from "had messages, now stalled"
	stallStartTimeout   time.Duration // from agent_definition or default 120s
	stallRunningTimeout time.Duration // from agent_definition or default 480s
	stallRestartCount   int           // incremented on each stall restart
	validationCommands  []string      // parsed from agent_def.ValidationCommands at spawn time
	workDir             string        // resolved working directory for validation commands
	// Idle/nudge detection (cli_interactive backend only; nudgeMax=0 means disabled)
	nudgeCount              int
	nudgeMax                int
	idleAfterMessageTimeout time.Duration
	idleStartTimeout        time.Duration
	lastNudgeAt             time.Time
	// External session ID (e.g., codex thread_id) — for logging only
	externalSessionID string
	// Callback level set by API-mode agent_callback handler. Mirrors the
	// callback_level finding written by AgentService.Callback for CLI agents.
	callbackLevel int
	// Transaction ID for structured logging (from orchestrator context)
	trx string
	// Rate-limit detection ring buffers (protected by recentMu).
	recentBlocks []string   // last ≤10 recent text blocks (structured msgs + PTY chunks)
	stderrBlocks []string   // last ≤10 stderr blocks
	recentMu     sync.Mutex // protects recentBlocks and stderrBlocks
	// Rate-limit restart tracking (separate counter from failRestartCount).
	rateLimitRetryCount int
	rateLimitTotalWait  time.Duration
	rateLimitConfig     rateLimitConfig
	adapter             CLIAdapter // nil for api/script backends
	// API-via-CLI tool registry: populated by apiBackend when APIViaCLI is enabled.
	// Read by spawner_tools.go to serve MCP tool list/dispatch over the socket bridge.
	apiTools    []provider.ToolSpec
	apiHandlers apirun.Registry
	apiToolEnv  apirun.ToolEnv
}

// terminalSignal is routed via the per-session terminalSignals registry to kill
// an agent immediately so handleCompletion reads the DB-written result
// (fail/continue/callback).
type terminalSignal struct {
	SessionID string
	Result    string
}
