package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// WorkflowInstanceStatus represents the status of a workflow instance
type WorkflowInstanceStatus string

const (
	WorkflowInstanceActive    WorkflowInstanceStatus = "active"
	WorkflowInstanceCompleted WorkflowInstanceStatus = "completed"
	WorkflowInstanceFailed    WorkflowInstanceStatus = "failed"
)

// PhaseStatus represents the status of a phase within a workflow instance
type PhaseStatus struct {
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
}

// WorkflowInstance represents a running workflow on a ticket
type WorkflowInstance struct {
	ID            string                 `json:"id"`
	ProjectID     string                 `json:"project_id"`
	TicketID      string                 `json:"ticket_id"`
	WorkflowID    string                 `json:"workflow_id"`
	Status        WorkflowInstanceStatus `json:"status"`
	Category      sql.NullString         `json:"-"`
	CurrentPhase  sql.NullString         `json:"-"`
	PhaseOrder    string                 `json:"-"` // JSON array string
	Phases        string                 `json:"-"` // JSON object string
	Findings      string                 `json:"-"` // JSON object string
	RetryCount    int                    `json:"retry_count"`
	ParentSession sql.NullString         `json:"-"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// GetPhaseOrder returns the phase order as a string slice
func (wi *WorkflowInstance) GetPhaseOrder() []string {
	var order []string
	json.Unmarshal([]byte(wi.PhaseOrder), &order)
	return order
}

// GetPhases returns the phases as a map of phase ID -> PhaseStatus
func (wi *WorkflowInstance) GetPhases() map[string]PhaseStatus {
	phases := make(map[string]PhaseStatus)
	json.Unmarshal([]byte(wi.Phases), &phases)
	return phases
}

// SetPhases updates the phases JSON string from a map
func (wi *WorkflowInstance) SetPhases(phases map[string]PhaseStatus) {
	data, _ := json.Marshal(phases)
	wi.Phases = string(data)
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
	var category *string
	if wi.Category.Valid {
		category = &wi.Category.String
	}
	var currentPhase *string
	if wi.CurrentPhase.Valid {
		currentPhase = &wi.CurrentPhase.String
	}
	var parentSession *string
	if wi.ParentSession.Valid {
		parentSession = &wi.ParentSession.String
	}

	// Parse JSON fields for proper serialization
	var phaseOrder []string
	json.Unmarshal([]byte(wi.PhaseOrder), &phaseOrder)
	if phaseOrder == nil {
		phaseOrder = []string{}
	}

	var phases map[string]PhaseStatus
	json.Unmarshal([]byte(wi.Phases), &phases)
	if phases == nil {
		phases = make(map[string]PhaseStatus)
	}

	var findings map[string]interface{}
	json.Unmarshal([]byte(wi.Findings), &findings)
	if findings == nil {
		findings = make(map[string]interface{})
	}

	return json.Marshal(&struct {
		ID            string                 `json:"id"`
		ProjectID     string                 `json:"project_id"`
		TicketID      string                 `json:"ticket_id"`
		WorkflowID    string                 `json:"workflow_id"`
		Status        WorkflowInstanceStatus `json:"status"`
		Category      *string                `json:"category,omitempty"`
		CurrentPhase  *string                `json:"current_phase,omitempty"`
		PhaseOrder    []string               `json:"phase_order"`
		Phases        map[string]PhaseStatus `json:"phases"`
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
		Status:        wi.Status,
		Category:      category,
		CurrentPhase:  currentPhase,
		PhaseOrder:    phaseOrder,
		Phases:        phases,
		Findings:      findings,
		RetryCount:    wi.RetryCount,
		ParentSession: parentSession,
		CreatedAt:     wi.CreatedAt,
		UpdatedAt:     wi.UpdatedAt,
	})
}
