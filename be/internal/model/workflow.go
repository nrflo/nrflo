package model

import (
	"encoding/json"
	"time"
)

// Workflow represents a workflow definition stored in the database
type Workflow struct {
	ID                     string    `json:"id"`
	ProjectID              string    `json:"project_id"`
	Description            string    `json:"description"`
	ScopeType              string    `json:"scope_type"` // "ticket" or "project"
	CloseTicketOnComplete  bool      `json:"close_ticket_on_complete"`
	Phases                 string    `json:"-"`           // JSON array string
	Groups                 string    `json:"-"`           // JSON array of tag strings
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
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
	var phases []interface{}
	if w.Phases != "" {
		_ = json.Unmarshal([]byte(w.Phases), &phases)
	}
	if phases == nil {
		phases = []interface{}{}
	}

	groups := w.GetGroups()

	scopeType := w.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}

	return json.Marshal(&struct {
		ID                    string        `json:"id"`
		ProjectID             string        `json:"project_id"`
		Description           string        `json:"description"`
		ScopeType             string        `json:"scope_type"`
		CloseTicketOnComplete bool          `json:"close_ticket_on_complete"`
		Phases                []interface{} `json:"phases"`
		Groups                []string      `json:"groups"`
		CreatedAt             time.Time     `json:"created_at"`
		UpdatedAt             time.Time     `json:"updated_at"`
	}{
		ID:                    w.ID,
		ProjectID:             w.ProjectID,
		Description:           w.Description,
		ScopeType:             scopeType,
		CloseTicketOnComplete: w.CloseTicketOnComplete,
		Phases:                phases,
		Groups:                groups,
		CreatedAt:             w.CreatedAt,
		UpdatedAt:             w.UpdatedAt,
	})
}
