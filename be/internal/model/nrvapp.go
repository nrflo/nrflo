package model

import "time"

// Review item status constants
const (
	ReviewStatusPending  = "pending"
	ReviewStatusApproved = "approved"
	ReviewStatusRejected = "rejected"
)

// Dispatch status constants
const (
	DispatchStatusSuccess = "success"
	DispatchStatusError   = "error"
)

// NrvappReviewItem represents a tool output awaiting human review
type NrvappReviewItem struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id"`
	ToolName     string     `json:"tool_name"`
	SessionID    *string    `json:"session_id"`
	Input        string     `json:"input"`
	Output       *string    `json:"output"`
	Draft        *string    `json:"draft"`
	Status       string     `json:"status"`
	RejectReason *string    `json:"reject_reason"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ApprovedAt   *time.Time `json:"approved_at"`
}

// NrvappToolDispatch records a tool execution event
type NrvappToolDispatch struct {
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

// NrvappConfigVersion stores a versioned snapshot of a config file
type NrvappConfigVersion struct {
	ID        int64     `json:"id"`
	ProjectID string    `json:"project_id"`
	File      string    `json:"file"`
	Version   int       `json:"version"`
	Content   []byte    `json:"content"`
	Actor     *string   `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
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
	ToolName        string `json:"tool_name"`
	ApproveNoEdits  int    `json:"approve_no_edits"`
	ApproveWithEdits int   `json:"approve_with_edits"`
	Rejected        int    `json:"rejected"`
}

// ThroughputPoint is a time-bucketed dispatch count
type ThroughputPoint struct {
	BucketStart time.Time `json:"bucket_start"`
	Count       int       `json:"count"`
}
