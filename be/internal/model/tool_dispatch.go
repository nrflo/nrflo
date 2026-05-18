package model

import "time"

// Dispatch status constants
const (
	DispatchStatusSuccess = "success"
	DispatchStatusError   = "error"
)

// ToolDispatch records a tool execution event
type ToolDispatch struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	SessionID  *string   `json:"session_id"`
	ToolName   string    `json:"tool_name"`
	Input      string    `json:"input"`
	Output     *string   `json:"output"`
	Status     string    `json:"status"`
	ErrorMsg   *string   `json:"error_msg"`
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}
