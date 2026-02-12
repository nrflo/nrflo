package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Workflow represents a workflow definition stored in the database
type Workflow struct {
	ID          string         `json:"id"`
	ProjectID   string         `json:"project_id"`
	Description string         `json:"description"`
	Categories  sql.NullString `json:"-"` // JSON array string
	Phases      string         `json:"-"` // JSON array string
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// MarshalJSON implements custom JSON marshaling for Workflow
func (w Workflow) MarshalJSON() ([]byte, error) {
	// Parse categories from JSON string to []string
	var categories []string
	if w.Categories.Valid && w.Categories.String != "" {
		_ = json.Unmarshal([]byte(w.Categories.String), &categories)
	}
	if categories == nil {
		categories = []string{}
	}

	// Parse phases from JSON string to []interface{}
	var phases []interface{}
	if w.Phases != "" {
		_ = json.Unmarshal([]byte(w.Phases), &phases)
	}
	if phases == nil {
		phases = []interface{}{}
	}

	return json.Marshal(&struct {
		ID          string        `json:"id"`
		ProjectID   string        `json:"project_id"`
		Description string        `json:"description"`
		Categories  []string      `json:"categories"`
		Phases      []interface{} `json:"phases"`
		CreatedAt   time.Time     `json:"created_at"`
		UpdatedAt   time.Time     `json:"updated_at"`
	}{
		ID:          w.ID,
		ProjectID:   w.ProjectID,
		Description: w.Description,
		Categories:  categories,
		Phases:      phases,
		CreatedAt:   w.CreatedAt,
		UpdatedAt:   w.UpdatedAt,
	})
}
