package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Workflow represents a workflow definition stored in the database
type Workflow struct {
	ID                     string         `json:"id"`
	ProjectID              string         `json:"project_id"`
	Description            string         `json:"description"`
	ScopeType              string         `json:"scope_type"` // "ticket" or "project"
	CloseTicketOnComplete  bool           `json:"close_ticket_on_complete"`
	Groups                 string         `json:"-"`           // JSON array of tag strings
	NextWorkflowOnSuccess       string         `json:"-"`
	FinalizeSuccessCommand      string         `json:"-"`
	FinalizeSuccessScriptID     string         `json:"-"`
	FinalizeFailureCommand      string         `json:"-"`
	FinalizeFailureScriptID     string         `json:"-"`
	ObserverContext             string         `json:"-"` // workflow-level override for observer system context
	ObserverProvider       sql.NullString `json:"-"` // workflow-level override for observer provider
	ObserverModel          sql.NullString `json:"-"` // workflow-level override for observer model
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
}

// GetGroups returns the parsed groups as a string slice
func (w *Workflow) GetGroups() []string {
	var groups []string
	if w.Groups != "" {
		json.Unmarshal([]byte(w.Groups), &groups)
	}
	if groups == nil {
		groups = []string{}
	}
	return groups
}

// SetGroups sets the groups from a string slice
func (w *Workflow) SetGroups(groups []string) {
	if groups == nil {
		groups = []string{}
	}
	data, _ := json.Marshal(groups)
	w.Groups = string(data)
}

// MarshalJSON implements custom JSON marshaling for Workflow
func (w Workflow) MarshalJSON() ([]byte, error) {
	groups := w.GetGroups()

	scopeType := w.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}

	return json.Marshal(&struct {
		ID                          string    `json:"id"`
		ProjectID                   string    `json:"project_id"`
		Description                 string    `json:"description"`
		ScopeType                   string    `json:"scope_type"`
		CloseTicketOnComplete       bool      `json:"close_ticket_on_complete"`
		Groups                      []string  `json:"groups"`
		NextWorkflowOnSuccess       string    `json:"next_workflow_on_success"`
		FinalizeSuccessCommand      string    `json:"finalize_success_command"`
		FinalizeSuccessScriptID     string    `json:"finalize_success_script_id"`
		FinalizeFailureCommand      string    `json:"finalize_failure_command"`
		FinalizeFailureScriptID     string    `json:"finalize_failure_script_id"`
		CreatedAt                   time.Time `json:"created_at"`
		UpdatedAt                   time.Time `json:"updated_at"`
	}{
		ID:                          w.ID,
		ProjectID:                   w.ProjectID,
		Description:                 w.Description,
		ScopeType:                   scopeType,
		CloseTicketOnComplete:       w.CloseTicketOnComplete,
		Groups:                      groups,
		NextWorkflowOnSuccess:       w.NextWorkflowOnSuccess,
		FinalizeSuccessCommand:      w.FinalizeSuccessCommand,
		FinalizeSuccessScriptID:     w.FinalizeSuccessScriptID,
		FinalizeFailureCommand:      w.FinalizeFailureCommand,
		FinalizeFailureScriptID:     w.FinalizeFailureScriptID,
		CreatedAt:                   w.CreatedAt,
		UpdatedAt:                   w.UpdatedAt,
	})
}
