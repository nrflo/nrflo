package model

import "time"

// Dispatch status constants
const (
	DispatchStatusSuccess = "success"
	DispatchStatusError   = "error"
)

// Dispatch source constants: which invoke site recorded the row. mcp is a
// CLI-driven agent calling through the `agent mcp`/`mcp-external` bridge
// (Spawner.DispatchTool), http is a pure in-process api-mode agent
// (apirun Runner.invokeTool), console is a console/console_chat tool call
// (console.Dispatch), engine is a native CLI tool tap with no tool_use_id to
// pair invoke/result (chat_events.go), and python is a tools_python handler
// recording its own row directly (PythonToolHandler.recordDispatch).
const (
	DispatchSourceMCP     = "mcp"
	DispatchSourceHTTP    = "http"
	DispatchSourceConsole = "console"
	DispatchSourceEngine  = "engine"
	DispatchSourcePython  = "python"
)

// ToolDispatch records a tool execution event
type ToolDispatch struct {
	ID         string  `json:"id"`
	ProjectID  string  `json:"project_id"`
	SessionID  *string `json:"session_id"`
	ToolName   string  `json:"tool_name"`
	Input      string  `json:"input"`
	Output     *string `json:"output"`
	Status     string  `json:"status"`
	ErrorMsg   *string `json:"error_msg"`
	DurationMs int64   `json:"duration_ms"`
	// Source discriminates which invoke site wrote this row (see
	// DispatchSource* constants above); "" for rows written before migration
	// 000229.
	Source string `json:"source,omitempty"`
	// SessionKind mirrors agent_sessions.kind at write time (workflow_agent,
	// console, console_chat) — denormalized so distribution queries never
	// need a join back to a since-deleted session row.
	SessionKind string `json:"session_kind,omitempty"`
	// WorkflowInstanceID is the calling session's bound instance, when any
	// (empty for console/console_chat calls with no bound run).
	WorkflowInstanceID string    `json:"workflow_instance_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}
