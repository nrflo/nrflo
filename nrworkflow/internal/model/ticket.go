package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Status represents the ticket status
type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusClosed     Status = "closed"
)

// IssueType represents the type of ticket
type IssueType string

const (
	IssueTypeBug     IssueType = "bug"
	IssueTypeFeature IssueType = "feature"
	IssueTypeTask    IssueType = "task"
	IssueTypeEpic    IssueType = "epic"
)

// Ticket represents a ticket in the system
type Ticket struct {
	ID          string         `json:"id"`
	ProjectID   string         `json:"project_id"`
	Title       string         `json:"title"`
	Description sql.NullString `json:"-"`
	Status      Status         `json:"status"`
	Priority    int            `json:"priority"`
	IssueType   IssueType      `json:"issue_type"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	ClosedAt    sql.NullTime   `json:"-"`
	CreatedBy   string         `json:"created_by"`
	CloseReason sql.NullString `json:"-"`
	AgentsState sql.NullString `json:"-"`
}

// MarshalJSON implements custom JSON marshaling for Ticket
func (t Ticket) MarshalJSON() ([]byte, error) {
	// Handle nullable fields
	var description *string
	if t.Description.Valid {
		description = &t.Description.String
	}

	var closedAt *time.Time
	if t.ClosedAt.Valid {
		closedAt = &t.ClosedAt.Time
	}

	var closeReason *string
	if t.CloseReason.Valid {
		closeReason = &t.CloseReason.String
	}

	var agentsState *string
	if t.AgentsState.Valid {
		agentsState = &t.AgentsState.String
	}

	return json.Marshal(&struct {
		ID          string     `json:"id"`
		ProjectID   string     `json:"project_id"`
		Title       string     `json:"title"`
		Description *string    `json:"description"`
		Status      Status     `json:"status"`
		Priority    int        `json:"priority"`
		IssueType   IssueType  `json:"issue_type"`
		CreatedAt   time.Time  `json:"created_at"`
		UpdatedAt   time.Time  `json:"updated_at"`
		ClosedAt    *time.Time `json:"closed_at"`
		CreatedBy   string     `json:"created_by"`
		CloseReason *string    `json:"close_reason"`
		AgentsState *string    `json:"agents_state"`
	}{
		ID:          t.ID,
		ProjectID:   t.ProjectID,
		Title:       t.Title,
		Description: description,
		Status:      t.Status,
		Priority:    t.Priority,
		IssueType:   t.IssueType,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		ClosedAt:    closedAt,
		CreatedBy:   t.CreatedBy,
		CloseReason: closeReason,
		AgentsState: agentsState,
	})
}

// Dependency represents a dependency between tickets
type Dependency struct {
	ProjectID   string    `json:"project_id"`
	IssueID     string    `json:"issue_id"`
	DependsOnID string    `json:"depends_on_id"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by"`
}
