package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"be/internal/clock"
	"be/internal/db"
	ptyPkg "be/internal/pty"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// ConsoleEngine drives a human-attended console session for one CLI provider
// (codex, claude) over that CLI's own event channel. The two engines share
// this interface but not the transport: codexEngine speaks app-server
// JSON-RPC over stdio; claudeEngine drives a PTY + the existing Claude-hooks
// path (no headless -p/stream-json). Unlike the autonomous ExecutionBackend/
// processInfo path, an engine holds no processInfo: there is no stall
// heartbeat, nudge loop, or restart cap to opt out of — those policies live
// on processInfo/monitorAll and are simply unreachable from an object that
// never has one.
type ConsoleEngine interface {
	// Name returns the engine's CLI identifier (e.g. "codex").
	Name() string
	// Start spawns the underlying CLI process/session per spec.
	Start(ctx context.Context, spec EngineSpec) error
	// SendUserTurn submits one user turn. Returns ErrTurnActive when a turn
	// is already in flight.
	SendUserTurn(ctx context.Context, text string) error
	// Events returns the channel of normalized events for this session. It is
	// closed when the engine's run loop exits (see Stop).
	Events() <-chan EngineEvent
	// ReplyApproval answers a pending approval request by id.
	ReplyApproval(id string, decision ApprovalDecision) error
	// SessionApprovals lists the tool names auto-allowed for the rest of the
	// session by approve_for_session decisions. Engines whose session scope
	// lives outside the server (codex acceptForSession) report none.
	SessionApprovals() []string
	// RevokeSessionApproval removes one tool from the session allowlist, so
	// its next use asks the human again. Errors when the engine cannot
	// revoke (codex — the allowlist is the app-server's).
	RevokeSessionApproval(tool string) error
	// InterruptTurn cancels the active turn without closing the conversation.
	// Returns ErrNoActiveTurn when the engine is idle.
	InterruptTurn(ctx context.Context) error
	// Stop tears down the engine: cancels the run context, closes the
	// underlying client/process, and closes the Events channel.
	Stop()
}

// sortedKeys returns m's keys in sorted order — the stable shape both
// session-allowlist implementations return from listAllowed.
func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

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
	MCPServerPath   string
	MCPEnv          map[string]string
	// APIProvider is "anthropic" or "openai", resolved from the unified model row.
	// (chat_model_resolver.go). Empty for claude/codex specs.
	APIProvider string
	// SystemPrompt is the rendered def/profile system_template_id text, resolved
	// by buildChatEngineSpec (console package). Empty = engine default (api
	// falls back to its own injectable/constant; codex/claude add nothing).
	SystemPrompt string
}

// effectiveCLISessionID returns CLISessionID when set, else SessionID — the
// CLI session identity a claude engine launches with.
func (s EngineSpec) effectiveCLISessionID() string {
	if s.CLISessionID != "" {
		return s.CLISessionID
	}
	return s.SessionID
}

// EventType identifies the kind of a normalized console event.
type EventType string

const (
	EventTextDelta        EventType = "text_delta"
	EventText             EventType = "text"
	EventThinking         EventType = "thinking"
	EventToolInvoke       EventType = "tool_invoke"
	EventToolResult       EventType = "tool_result"
	EventApprovalRequest  EventType = "approval_request"
	EventApprovalResolved EventType = "approval_resolved"
	EventTurnStarted      EventType = "turn_started"
	EventTurnCompleted    EventType = "turn_completed"
	EventTokenUsage       EventType = "token_usage"
	EventError            EventType = "error"
)

// EngineEvent is one normalized event surfaced to a console session,
// independent of which CLI provider produced it. Every approval an engine
// registers via EventApprovalRequest is settled by exactly one
// EventApprovalResolved (ApprovalID + Decision, reason in Text) — whether the
// settling path is a human ReplyApproval, a timeout, engine stop/ctx
// cancellation, or the CLI resolving the request on its own. Consumers may
// rely on this to clear a pending-approval UI without a separate timeout path
// of their own.
type EngineEvent struct {
	Type           EventType
	SessionID      string
	ItemID         string
	Text           string
	ToolName       string
	ToolInput      map[string]any
	IsError        bool
	ContextLeftPct int
	Approval       *ApprovalRequest
	ApprovalID     string
	Decision       ApprovalDecision
}

// EventEmitter delivers one EngineEvent. A nil EventEmitter is valid and
// means "no console listener attached" — every call site must go through the
// nil-safe emit method below rather than invoking the func value directly.
type EventEmitter func(EngineEvent)

// emit is nil-safe: emit.emit(ev) is a no-op when the emitter is nil (the
// autonomous codex spawn path passes nil and must behave byte-for-byte as
// before this event ever existed).
func (e EventEmitter) emit(ev EngineEvent) {
	if e != nil {
		e(ev)
	}
}

// ApprovalRequest describes one pending server->client approval prompt.
type ApprovalRequest struct {
	ID      string
	Kind    string // the wire method name, e.g. "item/commandExecution/requestApproval"
	Command string
	Cwd     string
	Reason  string
	Raw     json.RawMessage
}

// ApprovalDecision is the caller-facing decision for a pending approval;
// engines map it to the wire vocabulary for the specific request method.
type ApprovalDecision string

const (
	ApprovalApprove           ApprovalDecision = "approve"
	ApprovalApproveForSession ApprovalDecision = "approve_for_session"
	ApprovalDeny              ApprovalDecision = "deny"
	ApprovalAbort             ApprovalDecision = "abort"
)

// ErrTurnActive is returned by SendUserTurn when a turn is already in flight.
var ErrTurnActive = fmt.Errorf("console engine: turn already active")

// ErrEngineStopped is returned by SendUserTurn when the engine is stopping or
// already stopped, so a message racing Close/StopAll is rejected instead of
// starting a turn against a torn-down event channel.
var ErrEngineStopped = fmt.Errorf("console engine: stopped")

// ErrNoActiveTurn is returned when interruption is requested while idle.
var ErrNoActiveTurn = fmt.Errorf("console engine: no active turn")

// EngineDeps bundles what GetConsoleEngine needs to construct any engine.
// PTY is the exported concrete *pty.Manager so callers outside this package
// can pass one; claudeEngine wraps it internally via wrapPtyManager. codex
// ignores PTY/Hub/NrfloPath — its transport is app-server JSON-RPC, not a PTY.
// claude/codex both ignore API — it is the api engine's tool profile only.
type EngineDeps struct {
	Sink      Sink
	PTY       *ptyPkg.Manager
	Hub       *ConsoleHub
	NrfloPath string
	API       APIEngineDeps
}

// APIEngineDeps carries the api console engine's tool profile, injected by
// console.ChatService (spawner must not import console — console imports
// spawner — so these are plain apirun types, not console.Deps/Registry
// built in-package).
type APIEngineDeps struct {
	Pool     *db.Pool
	Clock    clock.Clock
	Tools    []provider.ToolSpec
	Handlers apirun.Registry
	ToolEnv  apirun.ToolEnv
}

// GetConsoleEngine returns the ConsoleEngine for an engine name. It is the
// one place a console engine name is compared;
// per-engine divergence (approval vocabulary, delta method names, profile
// writing, transport) lives inside the returned engine.
func GetConsoleEngine(name string, deps EngineDeps) (ConsoleEngine, error) {
	switch name {
	case "codex":
		return newCodexEngine(deps.Sink), nil
	case "claude":
		return newClaudeEngine(deps), nil
	case "api":
		return newAPIConsoleEngine(deps), nil
	default:
		return nil, fmt.Errorf("unknown console engine: %s", name)
	}
}
