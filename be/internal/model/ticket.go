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
	ID             string         `json:"id"`
	ProjectID      string         `json:"project_id"`
	Title          string         `json:"title"`
	Description    sql.NullString `json:"-"`
	Status         Status         `json:"status"`
	Priority       int            `json:"priority"`
	IssueType      IssueType      `json:"issue_type"`
	ParentTicketID sql.NullString `json:"-"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	ClosedAt       sql.NullTime   `json:"-"`
	CreatedBy      string         `json:"created_by"`
	CloseReason    sql.NullString `json:"-"`
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

	var parentTicketID *string
	if t.ParentTicketID.Valid {
		parentTicketID = &t.ParentTicketID.String
	}

	return json.Marshal(&struct {
		ID             string     `json:"id"`
		ProjectID      string     `json:"project_id"`
		Title          string     `json:"title"`
		Description    *string    `json:"description"`
		Status         Status     `json:"status"`
		Priority       int        `json:"priority"`
		IssueType      IssueType  `json:"issue_type"`
		ParentTicketID *string    `json:"parent_ticket_id"`
		CreatedAt      time.Time  `json:"created_at"`
		UpdatedAt      time.Time  `json:"updated_at"`
		ClosedAt       *time.Time `json:"closed_at"`
		CreatedBy      string     `json:"created_by"`
		CloseReason    *string    `json:"close_reason"`
	}{
		ID:             t.ID,
		ProjectID:      t.ProjectID,
		Title:          t.Title,
		Description:    description,
		Status:         t.Status,
		Priority:       t.Priority,
		IssueType:      t.IssueType,
		ParentTicketID: parentTicketID,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		ClosedAt:       closedAt,
		CreatedBy:      t.CreatedBy,
		CloseReason:    closeReason,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for Ticket
func (t *Ticket) UnmarshalJSON(data []byte) error {
	var aux struct {
		ID             string     `json:"id"`
		ProjectID      string     `json:"project_id"`
		Title          string     `json:"title"`
		Description    *string    `json:"description"`
		Status         Status     `json:"status"`
		Priority       int        `json:"priority"`
		IssueType      IssueType  `json:"issue_type"`
		ParentTicketID *string    `json:"parent_ticket_id"`
		CreatedAt      time.Time  `json:"created_at"`
		UpdatedAt      time.Time  `json:"updated_at"`
		ClosedAt       *time.Time `json:"closed_at"`
		CreatedBy      string     `json:"created_by"`
		CloseReason    *string    `json:"close_reason"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	t.ID = aux.ID
	t.ProjectID = aux.ProjectID
	t.Title = aux.Title
	t.Status = aux.Status
	t.Priority = aux.Priority
	t.IssueType = aux.IssueType
	t.CreatedAt = aux.CreatedAt
	t.UpdatedAt = aux.UpdatedAt
	t.CreatedBy = aux.CreatedBy

	if aux.Description != nil {
		t.Description = sql.NullString{String: *aux.Description, Valid: true}
	}
	if aux.ParentTicketID != nil {
		t.ParentTicketID = sql.NullString{String: *aux.ParentTicketID, Valid: true}
	}
	if aux.ClosedAt != nil {
		t.ClosedAt = sql.NullTime{Time: *aux.ClosedAt, Valid: true}
	}
	if aux.CloseReason != nil {
		t.CloseReason = sql.NullString{String: *aux.CloseReason, Valid: true}
	}

	return nil
}

// Dependency represents a dependency between tickets
type Dependency struct {
	ProjectID     string    `json:"project_id"`
	IssueID       string    `json:"issue_id"`
	DependsOnID   string    `json:"depends_on_id"`
	Type          string    `json:"type"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     string    `json:"created_by"`
	DependsOnTitle string   `json:"depends_on_title,omitempty"`
	IssueTitle     string   `json:"issue_title,omitempty"`
}
