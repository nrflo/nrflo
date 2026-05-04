package model

import "time"

// Review item status constants
const (
	ReviewStatusPending  = "pending"
	ReviewStatusApproved = "approved"
	ReviewStatusRejected = "rejected"
)

// ReviewItem represents a tool output awaiting human review
type ReviewItem struct {
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
