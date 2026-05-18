package spawner

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// RetryClass categorizes the reason an agent exited abnormally for retry decisions.
type RetryClass int

const (
	RetryClassNone      RetryClass = 0 // normal exit or unrecognized pattern
	RetryClassRateLimit RetryClass = 1 // provider rate limit / quota exhausted
	RetryClassError     RetryClass = 2 // provider error pattern (non-retriable via backoff)
)

// matchAnyCaseInsensitive reports whether text contains any of the patterns
// (case-insensitive). Returns the first matched pattern and true on success.
func matchAnyCaseInsensitive(text string, patterns []string) (string, bool) {
	lower := strings.ToLower(text)
	for _, p := range patterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return p, true
		}
	}
	return "", false
}

// CLIAdapter defines the interface for different CLI backends
type CLIAdapter interface {
	// Name returns the CLI identifier (e.g., "claude", "opencode")
	Name() string

	// MapModel converts a short model name to the CLI's expected format
	MapModel(model string) string

	// SupportsSessionID returns true if the CLI supports custom session IDs
	SupportsSessionID() bool

	// SupportsSystemPromptFile returns true if the CLI supports --append-system-prompt-file
	SupportsSystemPromptFile() bool

	// SupportsResume returns true if the CLI supports resuming a session
	SupportsResume() bool

	// BuildInteractiveCommand creates the exec.Cmd for interactive PTY execution.
	// When opts.ResumeSessionID is non-empty, the CLI resumes that session with
	// opts.Prompt delivered as the first turn's input.
	BuildInteractiveCommand(opts InteractiveSpawnOptions) *exec.Cmd

	// PrepareInteractive performs adapter-owned spawn-time setup for interactive
	// runs (e.g., codex's per-session CODEX_HOME profile dir + hook event list).
	// Returns extras that the backend forwards into InteractiveSpawnOptions, a
	// cleanup func (always non-nil; safe to call on the error path and from the
	// wait goroutine), and any error. Adapters with no setup return zero
	// InteractiveExtras and a noop cleanup.
	PrepareInteractive(opts InteractivePrepOptions) (InteractiveExtras, func(), error)

	// DeliversPromptInline returns true when the adapter passes the rendered
	// prompt to the CLI itself (e.g., codex via argv positional). When false,
	// the backend writes the prompt body to PTY stdin after the readiness delay.
	DeliversPromptInline() bool

	// NeedsTerminalQueryReplies returns true when the CLI's TUI sends DSR/DA/
	// kitty/OSC capability queries during init that must be auto-answered for
	// the TUI to proceed (codex). Adapters that don't probe (claude) return
	// false so the responder is skipped on their PTY ferry.
	NeedsTerminalQueryReplies() bool

	// BumpsOnPTYBytes returns true when receiving PTY bytes should bump
	// lastMessageTime / hasReceivedMessage for stall detection purposes.
	// All current adapters return false: heartbeat comes from structured
	// activity channels — PreToolUse/PostToolUse/Stop hooks (Claude), SSE
	// message.part.updated/session.idle events (Opencode), or the rollout
	// JSONL tailer (Codex). The method is kept on the interface so future
	// adapters that lack a structured channel can opt back in.
	BumpsOnPTYBytes() bool

	// NaturalExitGrace is how long the terminal-signal handler should
	// wait for the CLI process to exit on its own before sending SIGTERM
	// when the agent reported `nrflo agent finished`. Adapters whose CLI
	// writes critical telemetry at end-of-turn (opencode flushes its
	// `step-finish` part with token usage *after* the agent's last tool
	// call returns) return a non-zero value here so the wrap-up has time
	// to land. Default 0 = no wait (kill immediately).
	NaturalExitGrace() time.Duration

	// ClassifyExit inspects recent stdout/stderr text and the exit code to
	// determine whether the agent exited due to a rate limit, a provider
	// error, or some other reason. recentText is the concatenated last ~10
	// output blocks; stderrTail is the last ~10 stderr blocks.
	// extraLimitPatterns and extraErrorPatterns are user-configured additions
	// resolved at call site from proc.rateLimitConfig; implementations merge
	// them with their code defaults before matching. Limit patterns are
	// checked first so a rate-limit message wins over a generic error pattern.
	ClassifyExit(recentText, stderrTail string, exitCode int, extraLimitPatterns, extraErrorPatterns []string) (RetryClass, string)
}

// InteractiveExtras carries adapter-owned spawn-time outputs that the backend
// forwards into InteractiveSpawnOptions. Fields are zero for adapters with no
// extras.
type InteractiveExtras struct {
	CodexHome  string      // per-session CODEX_HOME dir (codex only)
	GeminiHome string      // per-session GEMINI_HOME dir (gemini only)
	Hooks      []HookEvent // event-keyed hook commands (codex only)
	Port       int         // embedded HTTP server port (opencode only; 0 = not used)
}

// InteractivePrepOptions carries the per-spawn context the adapter needs for
// PrepareInteractive. Kept as a struct of explicit fields so the interface
// doesn't leak the unexported processInfo type to external implementers.
type InteractivePrepOptions struct {
	SessionID          string
	WorkflowInstanceID string
	ProjectID          string
	WorkDir            string
}

// InteractiveSpawnOptions contains parameters for building an interactive PTY command.
// For most adapters the prompt is delivered post-spawn via PTY stdin Write
// (see deliverPrompt). Codex is the exception: its TUI input box has a
// wrapping bug that panics on multi-KB pasted bodies (`tui/src/wrapping.rs:52`,
// integer underflow), so we pass the prompt as an argv positional instead.
type InteractiveSpawnOptions struct {
	SessionID        string
	Model            string
	ReasoningEffort  string // passed as --effort (Claude) or --variant (Opencode)
	WorkDir          string
	Env              []string
	SystemPromptFile string      // path to suffix file; Claude: --append-system-prompt-file; others: ignored
	SettingsJSON     string      // Claude: --settings JSON; others: ignored
	CodexHome        string      // CODEX_HOME dir path; Codex only — ignored by other adapters
	GeminiHome       string      // GEMINI_HOME dir path; Gemini only — ignored by other adapters
	Prompt           string      // initial user prompt; Codex passes this as argv positional, others ignore
	Hooks            []HookEvent // event-keyed hook commands; Codex injects via repeated `-c hooks.<event>=…` (TUI ignores config.toml hooks); other adapters ignore
	Port             int         // embedded HTTP server port (opencode only; 0 = not used by other adapters)
	ResumeSessionID  string      // when set, CLI resumes this session; Claude: --resume <id>; Codex: `resume <id>` subcommand; Opencode: ignored
}

// Sink is a spawner-internal interface the SSE event consumer uses to report
// events back to the spawner without importing the concrete *Spawner type.
// All methods are best-effort: implementations must not panic on errors.
type Sink interface {
	// RecordHookMessage inserts one agent_messages row + returns IDs for broadcast.
	RecordHookMessage(sessionID, content, category, payload string) (projectID, ticketID, workflowName string, err error)
	// UpdateContextLeft updates context_left percentage for a session.
	UpdateContextLeft(sessionID string, pct int) (projectID, ticketID, workflowName string, err error)
	// BumpLastMessage resets stall/idle detection timestamp for the session.
	BumpLastMessage(sessionID string)
	// SetLastMessage updates proc.lastMessage so the periodic "agent status"
	// log line surfaces SSE/hook content. Empty content or unknown session
	// is a no-op.
	SetLastMessage(sessionID, content string)
	// OnTurnComplete signals end of an assistant turn (e.g. session.idle event).
	OnTurnComplete(sessionID string)
	// BroadcastMessagesUpdated broadcasts a messages.updated WS event.
	BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID string)
	// RecordError records an actionable error to the errors table.
	RecordError(projectID, errType, sessionID, msg string)
}

// PostStartOptions holds parameters for PostStart.
type PostStartOptions struct {
	SessionID  string
	WorkDir    string
	Port       int       // opencode embedded HTTP event server port (0 for other adapters)
	CodexHome  string    // codex per-session CODEX_HOME dir ("" for other adapters)
	GeminiHome string    // gemini per-session GEMINI_HOME dir ("" for other adapters)
	StartedAt  time.Time // wall-clock right before launch; opencode uses it to disambiguate our session from prior history
	MaxContext int       // max context window tokens; 0 falls back to ComputeContextLeftPct default (200k)
	Sink       Sink
}

// PostStarter is an optional sub-interface for CLIAdapter implementations that
// need to run additional setup after the PTY session starts. Asserted at the call
// site via interface assertion in cliInteractiveBackend.Start — NOT added to
// CLIAdapter itself — so adapters that don't need it (claude, opencode) are unaffected.
type PostStarter interface {
	PostStart(ctx context.Context, opts PostStartOptions) (cleanup func(), err error)
}

// HookEvent describes one hook event registration the spawner wants codex to
// fire. Translated to a `-c hooks.<Event>=[{matcher="*",hooks=[{...}]}]`
// inline-TOML CLI override at command-build time.
type HookEvent struct {
	Event      string // e.g. "SessionStart", "PostToolUse", "Stop"
	Command    string // shell command codex execs when the event fires
	TimeoutSec int    // hook timeout in seconds
}

// SpawnOptions contains parameters for building a spawn command
type SpawnOptions struct {
	Model            string
	SessionID        string
	PromptFile       string // Path to system prompt file
	Prompt           string // Full prompt content (for CLIs without file support)
	WorkDir          string
	Env              []string
	MappedModel      string // DB-sourced mapped model name; if set, adapters skip their own MapModel()
	ReasoningEffort  string // DB-sourced reasoning effort; if set, adapters skip their own GetReasoningEffort()
	SettingsJSON     string // Claude --settings JSON (ignored by non-Claude adapters)
	SystemPromptFile string // Path to system prompt suffix file (--append-system-prompt-file; Claude only)
}

// DefaultCLIForModel returns the appropriate CLI name for a model.
// opencode_* → opencode, codex_gpt* → codex, everything else → claude.
func DefaultCLIForModel(model string) string {
	if strings.HasPrefix(model, "opencode_") {
		return "opencode"
	}
	if strings.HasPrefix(model, "codex_gpt") {
		return "codex"
	}
	if strings.HasPrefix(model, "gemini_") {
		return "gemini"
	}
	return "claude"
}

// GetCLIAdapter returns the appropriate adapter for a CLI name
func GetCLIAdapter(name string) (CLIAdapter, error) {
	switch name {
	case "claude":
		return &ClaudeAdapter{}, nil
	case "opencode":
		return &OpencodeAdapter{}, nil
	case "codex":
		return &CodexAdapter{}, nil
	case "gemini":
		return &GeminiAdapter{}, nil
	default:
		return nil, fmt.Errorf("unknown CLI: %s", name)
	}
}
