package spawner

import (
	"context"
	"encoding/json"
	"fmt"

	ptyPkg "be/internal/pty"
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
	// Stop tears down the engine: cancels the run context, closes the
	// underlying client/process, and closes the Events channel.
	Stop()
}

// EngineSpec carries the per-session parameters an engine needs to start.
type EngineSpec struct {
	SessionID       string
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
}

// EventType identifies the kind of a normalized console event.
type EventType string

const (
	EventTextDelta       EventType = "text_delta"
	EventText            EventType = "text"
	EventThinking        EventType = "thinking"
	EventToolInvoke      EventType = "tool_invoke"
	EventToolResult      EventType = "tool_result"
	EventApprovalRequest EventType = "approval_request"
	EventTurnStarted     EventType = "turn_started"
	EventTurnCompleted   EventType = "turn_completed"
	EventTokenUsage      EventType = "token_usage"
	EventError           EventType = "error"
)

// EngineEvent is one normalized event surfaced to a console session,
// independent of which CLI provider produced it.
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

// EngineDeps bundles what GetConsoleEngine needs to construct any engine.
// PTY is the exported concrete *pty.Manager so callers outside this package
// can pass one; claudeEngine wraps it internally via wrapPtyManager. codex
// ignores PTY/Hub/NrfloPath — its transport is app-server JSON-RPC, not a PTY.
type EngineDeps struct {
	Sink      Sink
	PTY       *ptyPkg.Manager
	Hub       *ConsoleHub
	NrfloPath string
}

// GetConsoleEngine returns the ConsoleEngine for a --cli name. Mirrors
// GetCLIAdapter (cli_adapter.go:233) and console.GetDriver
// (console/driver.go:62) — the ONE place an engine name is compared;
// per-engine divergence (approval vocabulary, delta method names, profile
// writing, transport) lives inside the returned engine.
func GetConsoleEngine(name string, deps EngineDeps) (ConsoleEngine, error) {
	switch name {
	case "codex":
		return newCodexEngine(deps.Sink), nil
	case "claude":
		return newClaudeEngine(deps), nil
	default:
		return nil, fmt.Errorf("unknown console engine: %s", name)
	}
}
