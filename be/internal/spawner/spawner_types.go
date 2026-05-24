package spawner

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
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
	APIMaxTokens     *int   `json:"api_max_tokens"`
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

	defaultAPIMaxIterations = 50
	defaultAPIMaxTokens     = 16384
	defaultAPISystemPrompt  = "You are an agent in a workflow. Follow the instructions below."
)

// ModelConfig holds DB-sourced model configuration for the spawner.
// Zero values mean "not configured" — adapters fall back to their hardcoded methods.
type ModelConfig struct {
	CLIType              string // "claude", "codex"
	MappedModel          string // actual CLI arg: "opus[1m]", "gpt-5.3-codex"
	ReasoningEffort      string // "", "high", "medium"
	ContextLength        int    // 200000, 1000000
	OverrideSystemPrompt bool   // when true and CLIType=="claude", emit --system-prompt-file from system-prompt injectable
}

// APIModelConfig holds DB-sourced configuration for an API-mode model row.
// Sourced from the api_models table, keyed by row id.
type APIModelConfig struct {
	Provider        string // "anthropic", "openai"
	MappedModel     string // actual provider model ID, e.g. "claude-opus-4-7"
	ContextLength   int    // max input context window in tokens
	ReasoningEffort string // "", "low", "medium", "high", "xhigh"
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
	// BuildAPIProvider constructs a provider.Provider for API-mode agents.
	// Called once per spawn with the provider name (e.g. "anthropic", "openai")
	// and project ID. Required when any agent definition selects api mode.
	BuildAPIProvider func(ctx context.Context, providerName, projectID string) (provider.Provider, error)
	// APIModelConfigs maps api_models row id to DB-sourced configuration.
	// Used in the api branch of prepareSpawn to look up provider, mapped model,
	// context length, and reasoning effort. nil map is safe (lookup returns zero value).
	APIModelConfigs map[string]APIModelConfig
	// AgentSvc persists context_left for API-mode agents (mirrors what the
	// CLI hook does for CLI agents).
	AgentSvc apirun.AgentSvc
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
	// APIMode enables execution_mode='api' agents. When false, prepareSpawn rejects any
	// agent with execution_mode='api' before making any provider call. Injected by the
	// orchestrator from the api_mode_enabled global setting at spawn time.
	APIMode bool
	// PTYManager manages PTY sessions for cli_interactive agents.
	PTYManager *ptyPkg.Manager
	// IdleAfterMessageTimeoutSec: idle window after last message before nudge (default 240s, 0 = use default).
	// Only applies to cliInteractiveBackend agents.
	IdleAfterMessageTimeoutSec int
	// IdleStartTimeoutSec: idle window before first message before nudge (default 120s, 0 = use default).
	// Only applies to cliInteractiveBackend agents.
	IdleStartTimeoutSec int
	// NudgeMax: max nudge attempts before auto-fail (default 5, 0 = use default).
	// Only applies to cliInteractiveBackend agents.
	NudgeMax int
	// DispatchRepo records tool dispatch events for python and HTTP tools.
	// Optional (nil-safe): when nil, dispatch rows are not inserted.
	DispatchRepo *repo.DispatchRepo
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
	// ArtifactSvc provides artifact listing and storage access for NRF_ARTIFACTS_DIR injection
	// and #{ARTIFACT[S]} template expansion. Optional (nil-safe): when nil, NRF_ARTIFACTS_DIR
	// still resolves to the stage dir but artifacts are not pre-materialized.
	ArtifactSvc *service.ArtifactService
	// WorkflowControl allows API-mode workflow_continue/workflow_fail builtins to act on the workflow.
	// Optional (nil-safe).
	WorkflowControl apirun.WorkflowController
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
	cmd     *exec.Cmd
	backend ExecutionBackend
	pid     int // OS pid; set by backends when proc.cmd is nil (e.g. PTY-owned process)
	// env is the full process env assembled in prepareSpawn (nrflo-controlled vars +
	// per-project vars). Stored separately from cmd.Env so contextSaveViaResume can
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
	modelID         string
	sessionID       string
	startTime       time.Time
	timeout         time.Duration
	pendingMessages []repo.MessageEntry // messages not yet flushed to DB
	lastMessage     string              // most recent message (for status display)
	nextSeq         int                 // next sequence number for agent_messages table
	messagesMutex   sync.Mutex
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
}

// terminalSignal is routed via the per-session terminalSignals registry to kill
// an agent immediately so handleCompletion reads the DB-written result
// (fail/continue/callback).
type terminalSignal struct {
	SessionID string
	Result    string
}
