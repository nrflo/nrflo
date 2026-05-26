package socket

import (
	"context"
	"encoding/json"
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
	CallTool(instanceID, sessionID, name string, input json.RawMessage) (output string, isError bool, err error)
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
