package model

import "time"

// AuditEntry represents a row in the audit_log table.
type AuditEntry struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id,omitempty"` // empty if user deleted
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	IP           string    `json:"ip"`
	UserAgent    string    `json:"user_agent"`
	Metadata     string    `json:"metadata"` // JSON object
	CreatedAt    time.Time `json:"created_at"`
}

// AuditFilter restricts audit_log queries.
type AuditFilter struct {
	UserID string
	Action string
	Since  *time.Time
	Until  *time.Time
}
