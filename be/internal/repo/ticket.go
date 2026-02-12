package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/db"
	"be/internal/model"
)

// TicketRepo handles ticket CRUD operations
type TicketRepo struct {
	db *db.DB
}

// NewTicketRepo creates a new ticket repository
func NewTicketRepo(database *db.DB) *TicketRepo {
	return &TicketRepo{db: database}
}

// Create creates a new ticket
func (r *TicketRepo) Create(ticket *model.Ticket) error {
	now := time.Now().UTC().Format(time.RFC3339)
	ticket.CreatedAt, _ = time.Parse(time.RFC3339, now)
	ticket.UpdatedAt = ticket.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO tickets (id, project_id, title, description, status, priority, issue_type, parent_ticket_id, created_at, updated_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(ticket.ID),
		strings.ToLower(ticket.ProjectID),
		ticket.Title,
		ticket.Description,
		ticket.Status,
		ticket.Priority,
		ticket.IssueType,
		ticket.ParentTicketID,
		now,
		now,
		ticket.CreatedBy,
	)
	return err
}

const ticketSelectCols = `id, project_id, title, description, status, priority, issue_type, parent_ticket_id, created_at, updated_at, closed_at, created_by, close_reason`

const ticketSelectColsPrefixed = `t.id, t.project_id, t.title, t.description, t.status, t.priority, t.issue_type, t.parent_ticket_id, t.created_at, t.updated_at, t.closed_at, t.created_by, t.close_reason`

func scanTicket(scanner interface{ Scan(...interface{}) error }) (*model.Ticket, error) {
	ticket := &model.Ticket{}
	var createdAt, updatedAt string
	var closedAt sql.NullString

	err := scanner.Scan(
		&ticket.ID,
		&ticket.ProjectID,
		&ticket.Title,
		&ticket.Description,
		&ticket.Status,
		&ticket.Priority,
		&ticket.IssueType,
		&ticket.ParentTicketID,
		&createdAt,
		&updatedAt,
		&closedAt,
		&ticket.CreatedBy,
		&ticket.CloseReason,
	)
	if err != nil {
		return nil, err
	}

	ticket.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	ticket.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if closedAt.Valid {
		t, _ := time.Parse(time.RFC3339, closedAt.String)
		ticket.ClosedAt = sql.NullTime{Time: t, Valid: true}
	}

	return ticket, nil
}

// Get retrieves a ticket by project ID and ticket ID
func (r *TicketRepo) Get(projectID, ticketID string) (*model.Ticket, error) {
	row := r.db.QueryRow(`
		SELECT `+ticketSelectCols+`
		FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`, projectID, ticketID)
	ticket, err := scanTicket(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ticket not found: %s", ticketID)
	}
	return ticket, err
}

// ListFilter contains filter options for listing tickets
type ListFilter struct {
	ProjectID   string
	Status      string
	IssueType   string
	BlockedOnly bool
}

// List retrieves tickets with optional filters
func (r *TicketRepo) List(filter *ListFilter) ([]*model.Ticket, error) {
	query := "SELECT " + ticketSelectCols + " FROM tickets WHERE LOWER(project_id) = LOWER(?)"
	args := []interface{}{filter.ProjectID}

	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.IssueType != "" {
		query += " AND issue_type = ?"
		args = append(args, filter.IssueType)
	}

	query += " ORDER BY created_at DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*model.Ticket
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}

	return tickets, nil
}

// UpdateFields contains fields that can be updated
type UpdateFields struct {
	Title          *string
	Description    *string
	Status         *string
	Priority       *int
	IssueType      *string
	ParentTicketID *string
}

// Update updates a ticket
func (r *TicketRepo) Update(projectID, ticketID string, fields *UpdateFields) error {
	// First check if ticket exists
	_, err := r.Get(projectID, ticketID)
	if err != nil {
		return err
	}

	updates := []string{}
	args := []interface{}{}

	if fields.Title != nil {
		updates = append(updates, "title = ?")
		args = append(args, *fields.Title)
	}
	if fields.Description != nil {
		updates = append(updates, "description = ?")
		args = append(args, *fields.Description)
	}
	if fields.Status != nil {
		updates = append(updates, "status = ?")
		args = append(args, *fields.Status)
	}
	if fields.Priority != nil {
		updates = append(updates, "priority = ?")
		args = append(args, *fields.Priority)
	}
	if fields.IssueType != nil {
		updates = append(updates, "issue_type = ?")
		args = append(args, *fields.IssueType)
	}
	if fields.ParentTicketID != nil {
		updates = append(updates, "parent_ticket_id = ?")
		parentID := strings.TrimSpace(*fields.ParentTicketID)
		if parentID == "" {
			args = append(args, nil)
		} else {
			args = append(args, strings.ToLower(parentID))
		}
	}

	if len(updates) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	updates = append(updates, "updated_at = ?")
	args = append(args, now)
	args = append(args, projectID)
	args = append(args, ticketID)

	query := "UPDATE tickets SET "
	for i, u := range updates {
		if i > 0 {
			query += ", "
		}
		query += u
	}
	query += " WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)"

	_, err = r.db.Exec(query, args...)
	return err
}

// Close closes a ticket with an optional reason
func (r *TicketRepo) Close(projectID, ticketID string, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	var closeReason interface{}
	if reason != "" {
		closeReason = reason
	}

	result, err := r.db.Exec(`
		UPDATE tickets SET status = 'closed', closed_at = ?, close_reason = ?, updated_at = ?
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		now, closeReason, now, projectID, ticketID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}
	return nil
}

// Reopen reopens a closed ticket by setting status back to open
func (r *TicketRepo) Reopen(projectID, ticketID string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := r.db.Exec(`
		UPDATE tickets SET status = 'open', closed_at = NULL, close_reason = NULL, updated_at = ?
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		now, projectID, ticketID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}
	return nil
}

// Delete deletes a ticket
func (r *TicketRepo) Delete(projectID, ticketID string) error {
	result, err := r.db.Exec("DELETE FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)", projectID, ticketID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}
	return nil
}
