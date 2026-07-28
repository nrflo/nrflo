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

// RefinerySidecar starts/stops a per-autonomous-session refinery fold
// sidecar, keyed by the slot the session runs in. Satisfied by
// *refinery.Manager; declared here (not imported) so spawner never depends
// on the refinery package's internals — mirrors console's locally-declared
// RefineryLifecycle (be/internal/console/chat_service_refinery.go).
type RefinerySidecar interface {
	StartSession(sessionID, projectID, workflowInstanceID, nodeID string)
	StopSession(sessionID string)
	// FoldNow forces one bounded fold for a live session, leaving the sidecar
	// running — the kill-time save path calls it so a session that outran the
	// fold debounce still has a digest to hand off instead of paying for a
	// context-saver agent.
	FoldNow(sessionID string)
}

// ModelConfig holds one enabled row from the unified model registry.
type ModelConfig struct {
	Provider       string
	CLIModel       string
	CLIContext     int
	CLIEfforts     []string
	APIModel       string
	APIContext     int
	APIEfforts     []string
	FallbackModels string
	DefaultEffort  string
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
	// GlobalStallStartTimeout overrides the default stall start timeout when agent def has no value.
	// nil = use hardcoded default, 0 = disabled, >0 = custom seconds.
	GlobalStallStartTimeout *int
	// GlobalStallRunningTimeout overrides the default stall running timeout when agent def has no value.
	// nil = use hardcoded default, 0 = disabled, >0 = custom seconds.
	GlobalStallRunningTimeout *int
	// ClaudeSettingsJSON is the --settings JSON for Claude CLI agents (safety hooks).
	// Empty string means no settings. Read once at workflow start from project config.
	ClaudeSettingsJSON string
	// ModelConfigs maps registry slug to its enabled provider/mode configuration.
	// Unknown slugs remain valid only as raw CLI passthrough values.
	ModelConfigs map[string]ModelConfig
	// ErrorSvc records agent errors (optional, nil-safe).
	ErrorSvc ErrorRecorder
	// BuildAPIProvider constructs a provider.Provider for API-mode agents.
	// Called once per spawn with the provider name (e.g. "anthropic", "openai")
	// and project ID. Required when any agent definition selects api mode.
	BuildAPIProvider func(ctx context.Context, providerName, projectID string) (provider.Provider, error)
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
	// DelegateDepth is this spawner's position in a delegate chain: 0 for a
	// top-level (non-delegate) spawner, N for a spawner running a delegate
	// worker N levels down. Threaded in-memory down the spawn tree (never a
	// shared DB counter), so it is per-chain and race-free: buildAPIRegistry
	// strips `delegate` once DelegateDepth+1 exceeds the cap, and Delegate
	// stamps each worker's child spawner with DelegateDepth+1.
	DelegateDepth int
	// RefinerySidecar drives the autonomous refinery fold sidecar's
	// StartSession/StopSession lifecycle around cli_interactive spawns.
	// Optional (nil-safe): only the main-workflow orchestrator config sets
	// it, so system one-off spawns (context-saver/consult/planner/observer),
	// which build their own Config, get no sidecar — by omission, not a
	// name-check.
	RefinerySidecar RefinerySidecar
}
