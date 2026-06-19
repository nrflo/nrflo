package model

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// Workflow represents a workflow definition stored in the database
type Workflow struct {
	ID                      string         `json:"id"`
	ProjectID               string         `json:"project_id"`
	Description             string         `json:"description"`
	ScopeType               string         `json:"scope_type"` // "ticket" or "project"
	CloseTicketOnComplete   bool           `json:"close_ticket_on_complete"`
	PurgeOnCompletion       bool           `json:"purge_on_completion"`
	Groups                  string         `json:"-"` // JSON array of tag strings
	NextWorkflowOnSuccess   string         `json:"-"`
	FinalizeSuccessCommand  string         `json:"-"`
	FinalizeSuccessScriptID string         `json:"-"`
	FinalizeFailureCommand  string         `json:"-"`
	FinalizeFailureScriptID string         `json:"-"`
	PauseEventCommand       string         `json:"-"`
	PauseEventScriptID      string         `json:"-"`
	ObserverContext         string         `json:"-"` // workflow-level override for observer system context
	ObserverProvider        sql.NullString `json:"-"` // workflow-level override for observer provider
	ObserverModel           sql.NullString `json:"-"` // workflow-level override for observer model
	FindingSchemas          string         `json:"-"` // JSON array of {key, schema, example} finding contracts
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
}

// GetFindingSchemas returns the finding_schemas column as a raw JSON array,
// defaulting to "[]" when empty.
func (w *Workflow) GetFindingSchemas() json.RawMessage {
	if strings.TrimSpace(w.FindingSchemas) == "" {
		return json.RawMessage("[]")
	}
	return json.RawMessage(w.FindingSchemas)
}

// GetGroups returns the parsed groups as a string slice
func (w *Workflow) GetGroups() []string {
	var groups []string
	if w.Groups != "" {
		_ = json.Unmarshal([]byte(w.Groups), &groups)
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
		ID                      string          `json:"id"`
		ProjectID               string          `json:"project_id"`
		Description             string          `json:"description"`
		ScopeType               string          `json:"scope_type"`
		CloseTicketOnComplete   bool            `json:"close_ticket_on_complete"`
		PurgeOnCompletion       bool            `json:"purge_on_completion"`
		Groups                  []string        `json:"groups"`
		NextWorkflowOnSuccess   string          `json:"next_workflow_on_success"`
		FinalizeSuccessCommand  string          `json:"finalize_success_command"`
		FinalizeSuccessScriptID string          `json:"finalize_success_script_id"`
		FinalizeFailureCommand  string          `json:"finalize_failure_command"`
		FinalizeFailureScriptID string          `json:"finalize_failure_script_id"`
		PauseEventCommand       string          `json:"pause_event_command"`
		PauseEventScriptID      string          `json:"pause_event_script_id"`
		FindingSchemas          json.RawMessage `json:"finding_schemas"`
		CreatedAt               time.Time       `json:"created_at"`
		UpdatedAt               time.Time       `json:"updated_at"`
	}{
		ID:                      w.ID,
		ProjectID:               w.ProjectID,
		Description:             w.Description,
		ScopeType:               scopeType,
		CloseTicketOnComplete:   w.CloseTicketOnComplete,
		PurgeOnCompletion:       w.PurgeOnCompletion,
		Groups:                  groups,
		NextWorkflowOnSuccess:   w.NextWorkflowOnSuccess,
		FinalizeSuccessCommand:  w.FinalizeSuccessCommand,
		FinalizeSuccessScriptID: w.FinalizeSuccessScriptID,
		FinalizeFailureCommand:  w.FinalizeFailureCommand,
		FinalizeFailureScriptID: w.FinalizeFailureScriptID,
		PauseEventCommand:       w.PauseEventCommand,
		PauseEventScriptID:      w.PauseEventScriptID,
		FindingSchemas:          w.GetFindingSchemas(),
		CreatedAt:               w.CreatedAt,
		UpdatedAt:               w.UpdatedAt,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for Workflow, mirroring
// MarshalJSON so that fields tagged json:"-" (groups, hook commands/scripts)
// survive a marshal→unmarshal round-trip (e.g. workflow export/import).
func (w *Workflow) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID                      string          `json:"id"`
		ProjectID               string          `json:"project_id"`
		Description             string          `json:"description"`
		ScopeType               string          `json:"scope_type"`
		CloseTicketOnComplete   bool            `json:"close_ticket_on_complete"`
		PurgeOnCompletion       bool            `json:"purge_on_completion"`
		Groups                  []string        `json:"groups"`
		NextWorkflowOnSuccess   string          `json:"next_workflow_on_success"`
		FinalizeSuccessCommand  string          `json:"finalize_success_command"`
		FinalizeSuccessScriptID string          `json:"finalize_success_script_id"`
		FinalizeFailureCommand  string          `json:"finalize_failure_command"`
		FinalizeFailureScriptID string          `json:"finalize_failure_script_id"`
		PauseEventCommand       string          `json:"pause_event_command"`
		PauseEventScriptID      string          `json:"pause_event_script_id"`
		FindingSchemas          json.RawMessage `json:"finding_schemas"`
		CreatedAt               time.Time       `json:"created_at"`
		UpdatedAt               time.Time       `json:"updated_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	w.ID = raw.ID
	w.ProjectID = raw.ProjectID
	w.Description = raw.Description
	w.ScopeType = raw.ScopeType
	w.CloseTicketOnComplete = raw.CloseTicketOnComplete
	w.PurgeOnCompletion = raw.PurgeOnCompletion
	w.SetGroups(raw.Groups)
	w.NextWorkflowOnSuccess = raw.NextWorkflowOnSuccess
	w.FinalizeSuccessCommand = raw.FinalizeSuccessCommand
	w.FinalizeSuccessScriptID = raw.FinalizeSuccessScriptID
	w.FinalizeFailureCommand = raw.FinalizeFailureCommand
	w.FinalizeFailureScriptID = raw.FinalizeFailureScriptID
	w.PauseEventCommand = raw.PauseEventCommand
	w.PauseEventScriptID = raw.PauseEventScriptID
	if len(raw.FindingSchemas) > 0 {
		w.FindingSchemas = string(raw.FindingSchemas)
	}
	w.CreatedAt = raw.CreatedAt
	w.UpdatedAt = raw.UpdatedAt
	return nil
}
