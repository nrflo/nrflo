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

// DispatchSummary aggregates dispatch statistics
type DispatchSummary struct {
	Total   int   `json:"total"`
	Success int   `json:"success"`
	Error   int   `json:"error"`
	P50Ms   int64 `json:"p50_ms"`
	P95Ms   int64 `json:"p95_ms"`
}

// EditRateRow holds per-tool review outcome ratios
type EditRateRow struct {
	ToolName         string `json:"tool_name"`
	ApproveNoEdits   int    `json:"approve_no_edits"`
	ApproveWithEdits int    `json:"approve_with_edits"`
	Rejected         int    `json:"rejected"`
}

// ThroughputPoint is a time-bucketed dispatch count
type ThroughputPoint struct {
	BucketStart time.Time `json:"bucket_start"`
	Count       int       `json:"count"`
}
