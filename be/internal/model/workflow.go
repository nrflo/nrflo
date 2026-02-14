package model

import (
	"encoding/json"
	"time"
)

// Workflow represents a workflow definition stored in the database
type Workflow struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Description string    `json:"description"`
	ScopeType   string    `json:"scope_type"` // "ticket" or "project"
	Phases      string    `json:"-"`           // JSON array string
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MarshalJSON implements custom JSON marshaling for Workflow
func (w Workflow) MarshalJSON() ([]byte, error) {
	// Parse phases from JSON string to []interface{}
	var phases []interface{}
	if w.Phases != "" {
		_ = json.Unmarshal([]byte(w.Phases), &phases)
	}
	if phases == nil {
		phases = []interface{}{}
	}

	scopeType := w.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}

	return json.Marshal(&struct {
		ID          string        `json:"id"`
		ProjectID   string        `json:"project_id"`
		Description string        `json:"description"`
		ScopeType   string        `json:"scope_type"`
		Phases      []interface{} `json:"phases"`
		CreatedAt   time.Time     `json:"created_at"`
		UpdatedAt   time.Time     `json:"updated_at"`
	}{
		ID:          w.ID,
		ProjectID:   w.ProjectID,
		Description: w.Description,
		ScopeType:   scopeType,
		Phases:      phases,
		CreatedAt:   w.CreatedAt,
		UpdatedAt:   w.UpdatedAt,
	})
}
