package socket

import (
	"context"
	"encoding/json"

	"be/internal/types"
)

// WorkflowOrchestrator enables observer agents to trigger and retry workflows via the socket.
// Nil-safe; pass nil in tests.
type WorkflowOrchestrator interface {
	StartWorkflow(ctx context.Context, projectID, ticketID, workflowName, instructions, scopeType string) (instanceID string, err error)
	RetryFailed(ctx context.Context, projectID, ticketID, workflowName, sessionID string) error
	RetryFailedProject(ctx context.Context, projectID, workflowName, sessionID, instanceID string) error
	ContinueWorkflow(ctx context.Context, projectID, instanceID, instructions string) error
	FailWorkflow(ctx context.Context, projectID, instanceID, reason string) error
	Consult(ctx context.Context, callerSessionID, consultantID, question string) (string, error)
}

// ToolDispatcher proxies MCP tool list/call requests from the socket to the
// in-process API-via-CLI registry held by the active spawner. Traffics only
// json.RawMessage and primitives so the socket package stays free of apirun imports.
type ToolDispatcher interface {
	ListTools(instanceID, sessionID string) (json.RawMessage, error)
	// CallTool's media is a marshaled array of {kind, media_type, data_b64, name}
	// objects (nil when the tool returned no media).
	CallTool(instanceID, sessionID, name string, input json.RawMessage) (output string, media json.RawMessage, isError bool, err error)
}

// ConsoleHooks routes hook-driven events for `kind='console'` sessions to
// their live console engine (via spawner.ConsoleHub). Optional, nil-safe —
// pass nil in tests. Mirrors TerminalSignaler/ToolDispatcher: only
// primitives/json cross the boundary so the socket package stays free of
// spawner engine internals. handled=false means "no live console engine for
// this session" — the caller keeps today's autonomous behavior.
type ConsoleHooks interface {
	// ApproveConsoleTool blocks until a human answers a PreToolUse approval
	// request (or it times out), returning the decision ("allow"/"deny") and
	// an optional reason to embed in the hookSpecificOutput response.
	ApproveConsoleTool(ctx context.Context, sessionID, toolName string, toolInput map[string]any, toolUseID string) (decision, reason string, handled bool)
	// ConsoleTurnEnd notifies the engine that a Stop hook fired.
	ConsoleTurnEnd(sessionID string) (handled bool)
	// ConsoleSessionReady notifies the engine that a SessionStart hook fired.
	ConsoleSessionReady(sessionID string) (handled bool)
	// ConsoleContextLeft forwards an agent.context_update to the engine.
	ConsoleContextLeft(sessionID string, pct int) (handled bool)
	// ConsoleUserPrompt routes a UserPromptSubmit hook echo to the live
	// console engine. handled=true means the engine already persisted this
	// user turn itself (SendUserTurn's echo) — skip recording; false means
	// no live engine OR a human-typed prompt from an attached terminal,
	// which the caller must record as usual.
	ConsoleUserPrompt(sessionID, prompt string) (handled bool)
}

// ContextInjector resolves additional context to attach to a console
// session's UserPromptSubmit hook response. Optional, nil-safe — pass nil in
// tests. Mirrors ConsoleHooks: only primitives cross the boundary so the
// socket package stays free of spawner engine internals. An empty return
// means "nothing to inject" — the caller adds no additional_context field.
type ContextInjector interface {
	// InjectUserPromptContext returns the context to surface for sessionID's
	// current turn (prompt is the submitted text), or "" when there is none
	// (non-console session, unknown session, empty template).
	InjectUserPromptContext(ctx context.Context, sessionID, prompt string) string
}

// ConsoleChatCreator mints a server-owned chat for the trusted local TUI.
type ConsoleChatCreator interface {
	CreateAuthenticated(engine, modelID, effort, projectID, systemTemplateID string, refineryEnabled bool) (sessionID, token string, err error)
	AttachAuthenticated(sessionID, projectID string) (token string, err error)
	Catalog(projectID string) (types.ConsoleCatalog, error)
}

// TerminalSignaler dispatches a best-effort kill signal to an active spawner
// after the socket handler has already written the agent result to the DB.
// The Handler nil-guards before calling — pass nil in tests.
type TerminalSignaler interface {
	RequestTerminalSignal(projectID, ticketID, workflow, sessionID, result string) error
	// BumpLastMessage resets stall-detection state for the matching proc so
	// hook-driven activity (PreToolUse/PostToolUse) does not trigger a stall restart.
	BumpLastMessage(projectID, ticketID, workflow, sessionID string) error
	// SetLastMessage updates the running proc's in-memory lastMessage so the
	// periodic "agent status" log line surfaces hook/SSE-delivered content for
	// interactive CLI agents (whose PTY output is otherwise dropped). Empty
	// content or unknown session is a no-op.
	SetLastMessage(projectID, ticketID, workflow, sessionID, content string) error
	// SignalSessionReady marks the matching proc as TUI-ready, unblocking the
	// PTY prompt-delivery wait. Best-effort and idempotent — repeated calls,
	// or calls for unknown sessions, are no-ops.
	SignalSessionReady(sessionID string) error
}
