package spawner

// NativeToolPolicy values for EngineSpec.NativeToolPolicy — mirrors
// console.NativeToolPolicyNone/Full by string value only (spawner must not
// import console; console imports spawner).
const (
	NativeToolPolicyNone = "none"
	NativeToolPolicyFull = "full"
)

// EngineSpec carries the per-session parameters an engine needs to start.
type EngineSpec struct {
	SessionID string
	// CLISessionID is the fresh underlying CLI --session-id (claude) identity
	// for a rotated engine; empty defaults to SessionID. SessionID stays the
	// stable console identity used for hub registration, MCPEnv
	// NRFLO_CONSOLE_SESSION_ID, and WS — only the CLI's own session/transcript
	// identity changes on a proactive-restart rotation. Codex needs no
	// equivalent split: thread/start mints a fresh thread on every Start.
	CLISessionID    string
	ProjectID       string
	WorkDir         string
	Model           string
	ReasoningEffort string
	FallbackModels  string // claude-only: comma-separated --fallback-model chain
	MaxContext      int
	Env             []string
	ApprovalPolicy  string // e.g. "on-request"; engine-specific default when empty
	Sandbox         string // e.g. "workspace-write"; engine-specific default when empty
	// Yolo auto-approves console tool calls: claude/api short-circuit their
	// RequestApproval/requestToolApproval to allow, codex starts its thread
	// with approvalPolicy="never". Resolved once at chat create (and on
	// rotate) from the default-ON console_yolo global setting.
	Yolo          bool
	MCPServerPath string
	MCPEnv        map[string]string
	// APIProvider is "anthropic" or "openai", resolved from the unified model row.
	// (chat_model_resolver.go). Empty for claude/codex specs.
	APIProvider string
	// SystemPrompt is the rendered def/profile system_template_id text, resolved
	// by buildChatEngineSpec (console package). Empty = engine default (api
	// falls back to its own injectable/constant; codex/claude add nothing).
	SystemPrompt string
	// NativeToolsCSV is the claude engine's --tools value, mirroring the
	// autonomous spawn path's SpawnOptions.NativeToolsCSV (cli_adapter_claude.go).
	// Empty leaves the CLI's native tools unrestricted.
	NativeToolsCSV string
	// NativeToolPolicy is a console.Profile's native-tool policy
	// ("none"/"full"/""): "none" gates the api engine's withFSTools off
	// regardless of the api_native_tools_enabled global (console_engine_api.go)
	// and (via NativeToolsCSV/Sandbox, set by the console package alongside
	// this field) restricts claude/codex; "" keeps each engine's own default.
	NativeToolPolicy string
	// ContextBudgetTokens is a console.Profile's proactive-rotation budget
	// (spawner.ProactiveRestartConsoleThreshold) and, for the api engine, its
	// context-watcher budget fallback (console_engine_api.go). 0 = engine/
	// global default.
	ContextBudgetTokens int
	// SeededContext is the rotation-carried refinery digest, seeded into the
	// FIRST provider request by engines that do NOT fire the
	// UserPromptSubmit hook (api, codex). The claude engine must NOT read
	// it — its WorkingSetInjector hook already re-injects the digest from the
	// DB by session id, so consuming this too would double-inject. Empty on
	// a fresh Create (no digest folded yet).
	SeededContext string
}

// effectiveCLISessionID returns CLISessionID when set, else SessionID — the
// CLI session identity a claude engine launches with.
func (s EngineSpec) effectiveCLISessionID() string {
	if s.CLISessionID != "" {
		return s.CLISessionID
	}
	return s.SessionID
}
