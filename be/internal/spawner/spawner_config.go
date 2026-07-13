package spawner

import (
	"context"

	"be/internal/clock"
	"be/internal/db"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/ws"
)

// ModelConfig holds DB-sourced model configuration for the spawner.
// Zero values mean "not configured" — adapters fall back to their hardcoded methods.
type ModelConfig struct {
	CLIType         string // "claude", "codex"
	MappedModel     string // actual CLI arg: "opus[1m]", "gpt-5.3-codex"
	ReasoningEffort string // "", "low", "medium", "high", "xhigh", "max", "ultra" (ultra: codex gpt-5.6 only)
	FallbackModels  string // claude only: comma-separated --fallback-model chain (≤3)
	ContextLength   int    // 200000, 1000000
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
	// FindingsSvc, ProjectFindingsSvc, AgentSvcReal, WorkflowSvc, TicketSvc are
	// used by API-mode tool builtins (findings_*, project_findings_*, agent_*,
	// workflow_skip, ticket_*). They mirror the services the socket handler uses
	// for CLI agents so WS event parity is automatic.
	FindingsSvc        *service.FindingsService
	ProjectFindingsSvc *service.ProjectFindingsService
	AgentSvcReal       *service.AgentService
	WorkflowSvc        *service.WorkflowService
	TicketSvc          *service.TicketService
	// APIMode enables execution_mode='api' agents. When false, prepareSpawn rejects any
	// agent with execution_mode='api' before making any provider call. Injected by the
	// orchestrator from the api_mode_enabled global setting at spawn time.
	APIMode bool
	// APIViaCLI routes Claude API calls through the CLI instead of direct HTTP. Injected from api_via_cli_enabled global setting.
	APIViaCLI bool
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
	// Subworkflows starts/polls callable sub-workflows for the run_subworkflow /
	// get_subworkflow builtins. Optional (nil-safe); set by the orchestrator.
	Subworkflows apirun.SubworkflowRunner
}
