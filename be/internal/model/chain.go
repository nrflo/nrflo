package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// ChainStatus represents the status of a chain execution
type ChainStatus string

const (
	ChainStatusPending   ChainStatus = "pending"
	ChainStatusRunning   ChainStatus = "running"
	ChainStatusCompleted ChainStatus = "completed"
	ChainStatusFailed    ChainStatus = "failed"
	ChainStatusCanceled  ChainStatus = "canceled"
)

// ChainItemStatus represents the status of an item in a chain execution
type ChainItemStatus string

const (
	ChainItemPending   ChainItemStatus = "pending"
	ChainItemRunning   ChainItemStatus = "running"
	ChainItemCompleted ChainItemStatus = "completed"
	ChainItemFailed    ChainItemStatus = "failed"
	ChainItemSkipped   ChainItemStatus = "skipped"
	ChainItemCanceled  ChainItemStatus = "canceled"
)

// ChainExecution represents a chain of tickets to execute sequentially
type ChainExecution struct {
	ID           string      `json:"id"`
	ProjectID    string      `json:"project_id"`
	Name         string      `json:"name"`
	Status       ChainStatus `json:"status"`
	WorkflowName  string      `json:"workflow_name"`
	EpicTicketID  string      `json:"epic_ticket_id,omitempty"`
	CreatedBy     string      `json:"created_by"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	StartedAt    *time.Time  `json:"started_at,omitempty"`
	CompletedAt  *time.Time  `json:"completed_at,omitempty"`

	// Computed fields (from subqueries in list)
	TotalItems     int `json:"total_items"`
	CompletedItems int `json:"completed_items"`

	// Loaded via join, not stored in chain_executions table
	Items []*ChainExecutionItem `json:"items,omitempty"`

	// JSON-only field (not stored in DB), populated at read time
	Deps map[string][]string `json:"deps,omitempty"`
}

// ChainExecutionItem represents a single ticket in a chain execution
type ChainExecutionItem struct {
	ID                 string          `json:"id"`
	ChainID            string          `json:"chain_id"`
	TicketID           string          `json:"ticket_id"`
	TicketTitle        string          `json:"ticket_title"`
	Position           int             `json:"position"`
	Status             ChainItemStatus `json:"status"`
	WorkflowInstanceID sql.NullString  `json:"-"`
	StartedAt          sql.NullString  `json:"-"`
	EndedAt            sql.NullString  `json:"-"`
	TotalTokensUsed    int64           `json:"-"`
}

// MarshalJSON implements custom JSON marshaling for ChainExecutionItem
func (i ChainExecutionItem) MarshalJSON() ([]byte, error) {
	var wfiID *string
	if i.WorkflowInstanceID.Valid {
		wfiID = &i.WorkflowInstanceID.String
	}
	var startedAt *string
	if i.StartedAt.Valid {
		startedAt = &i.StartedAt.String
	}
	var endedAt *string
	if i.EndedAt.Valid {
		endedAt = &i.EndedAt.String
	}

	return json.Marshal(&struct {
		ID                 string          `json:"id"`
		ChainID            string          `json:"chain_id"`
		TicketID           string          `json:"ticket_id"`
		TicketTitle        string          `json:"ticket_title"`
		Position           int             `json:"position"`
		Status             ChainItemStatus `json:"status"`
		WorkflowInstanceID *string         `json:"workflow_instance_id,omitempty"`
		StartedAt          *string         `json:"started_at,omitempty"`
		EndedAt            *string         `json:"ended_at,omitempty"`
		TotalTokensUsed    int64           `json:"total_tokens_used,omitempty"`
	}{
		ID:                 i.ID,
		ChainID:            i.ChainID,
		TicketID:           i.TicketID,
		TicketTitle:        i.TicketTitle,
		Position:           i.Position,
		Status:             i.Status,
		WorkflowInstanceID: wfiID,
		StartedAt:          startedAt,
		EndedAt:            endedAt,
		TotalTokensUsed:    i.TotalTokensUsed,
	})
}

// ChainExecutionLock represents a lock on a ticket within a chain
type ChainExecutionLock struct {
	ProjectID string `json:"project_id"`
	TicketID  string `json:"ticket_id"`
	ChainID   string `json:"chain_id"`
}
