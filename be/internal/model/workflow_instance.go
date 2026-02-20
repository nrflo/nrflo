package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// PhaseStatus represents the status of a phase within a workflow instance.
// Used by service.derivePhaseStatuses — not stored in workflow_instances table.
type PhaseStatus struct {
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
}

// WorkflowInstanceStatus represents the status of a workflow instance
type WorkflowInstanceStatus string

const (
	WorkflowInstanceActive           WorkflowInstanceStatus = "active"
	WorkflowInstanceCompleted        WorkflowInstanceStatus = "completed"
	WorkflowInstanceFailed           WorkflowInstanceStatus = "failed"
	WorkflowInstanceProjectCompleted WorkflowInstanceStatus = "project_completed"
)

// WorkflowInstance represents a running workflow on a ticket or project
type WorkflowInstance struct {
	ID            string                 `json:"id"`
	ProjectID     string                 `json:"project_id"`
	TicketID      string                 `json:"ticket_id"`
	WorkflowID    string                 `json:"workflow_id"`
	ScopeType     string                 `json:"scope_type"` // "ticket" or "project"
	Status        WorkflowInstanceStatus `json:"status"`
	Findings      string                 `json:"-"` // JSON object string
	RetryCount    int                    `json:"retry_count"`
	ParentSession sql.NullString         `json:"-"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// IsProjectScope returns true if this is a project-scoped workflow instance
func (wi *WorkflowInstance) IsProjectScope() bool {
	return wi.ScopeType == "project"
}

// GetFindings returns the workflow-level findings as a map
func (wi *WorkflowInstance) GetFindings() map[string]interface{} {
	findings := make(map[string]interface{})
	json.Unmarshal([]byte(wi.Findings), &findings)
	return findings
}

// SetFindings updates the findings JSON string from a map
func (wi *WorkflowInstance) SetFindings(findings map[string]interface{}) {
	data, _ := json.Marshal(findings)
	wi.Findings = string(data)
}

// MarshalJSON implements custom JSON marshaling for WorkflowInstance
func (wi WorkflowInstance) MarshalJSON() ([]byte, error) {
	var parentSession *string
	if wi.ParentSession.Valid {
		parentSession = &wi.ParentSession.String
	}

	var findings map[string]interface{}
	json.Unmarshal([]byte(wi.Findings), &findings)
	if findings == nil {
		findings = make(map[string]interface{})
	}

	scopeType := wi.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}

	return json.Marshal(&struct {
		ID            string                 `json:"id"`
		ProjectID     string                 `json:"project_id"`
		TicketID      string                 `json:"ticket_id,omitempty"`
		WorkflowID    string                 `json:"workflow_id"`
		ScopeType     string                 `json:"scope_type"`
		Status        WorkflowInstanceStatus `json:"status"`
		Findings      map[string]interface{} `json:"findings"`
		RetryCount    int                    `json:"retry_count"`
		ParentSession *string                `json:"parent_session,omitempty"`
		CreatedAt     time.Time              `json:"created_at"`
		UpdatedAt     time.Time              `json:"updated_at"`
	}{
		ID:            wi.ID,
		ProjectID:     wi.ProjectID,
		TicketID:      wi.TicketID,
		WorkflowID:    wi.WorkflowID,
		ScopeType:     scopeType,
		Status:        wi.Status,
		Findings:      findings,
		RetryCount:    wi.RetryCount,
		ParentSession: parentSession,
		CreatedAt:     wi.CreatedAt,
		UpdatedAt:     wi.UpdatedAt,
	})
}
