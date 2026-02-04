package repo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nrworkflow/internal/db"
	"nrworkflow/internal/model"
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
		INSERT INTO tickets (id, project_id, title, description, status, priority, issue_type, created_at, updated_at, created_by, agents_state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(ticket.ID),
		strings.ToLower(ticket.ProjectID),
		ticket.Title,
		ticket.Description,
		ticket.Status,
		ticket.Priority,
		ticket.IssueType,
		now,
		now,
		ticket.CreatedBy,
		ticket.AgentsState,
	)
	return err
}

// Get retrieves a ticket by project ID and ticket ID
func (r *TicketRepo) Get(projectID, ticketID string) (*model.Ticket, error) {
	ticket := &model.Ticket{}
	var createdAt, updatedAt string
	var closedAt sql.NullString

	err := r.db.QueryRow(`
		SELECT id, project_id, title, description, status, priority, issue_type, created_at, updated_at, closed_at, created_by, close_reason, agents_state
		FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`, projectID, ticketID).Scan(
		&ticket.ID,
		&ticket.ProjectID,
		&ticket.Title,
		&ticket.Description,
		&ticket.Status,
		&ticket.Priority,
		&ticket.IssueType,
		&createdAt,
		&updatedAt,
		&closedAt,
		&ticket.CreatedBy,
		&ticket.CloseReason,
		&ticket.AgentsState,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ticket not found: %s", ticketID)
	}
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

// GetWithUpdatedAt retrieves a ticket and its updated_at timestamp for CAS operations
func (r *TicketRepo) GetWithUpdatedAt(projectID, ticketID string) (*model.Ticket, string, error) {
	ticket := &model.Ticket{}
	var createdAt, updatedAt string
	var closedAt sql.NullString

	err := r.db.QueryRow(`
		SELECT id, project_id, title, description, status, priority, issue_type, created_at, updated_at, closed_at, created_by, close_reason, agents_state
		FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`, projectID, ticketID).Scan(
		&ticket.ID,
		&ticket.ProjectID,
		&ticket.Title,
		&ticket.Description,
		&ticket.Status,
		&ticket.Priority,
		&ticket.IssueType,
		&createdAt,
		&updatedAt,
		&closedAt,
		&ticket.CreatedBy,
		&ticket.CloseReason,
		&ticket.AgentsState,
	)
	if err == sql.ErrNoRows {
		return nil, "", fmt.Errorf("ticket not found: %s", ticketID)
	}
	if err != nil {
		return nil, "", err
	}

	ticket.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	ticket.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if closedAt.Valid {
		t, _ := time.Parse(time.RFC3339, closedAt.String)
		ticket.ClosedAt = sql.NullTime{Time: t, Valid: true}
	}

	return ticket, updatedAt, nil
}

// CompareAndSwapAgentsState atomically updates agents_state if updated_at matches
func (r *TicketRepo) CompareAndSwapAgentsState(projectID, ticketID, expectedUpdatedAt, newState string) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.db.Exec(`
		UPDATE tickets
		SET agents_state = ?, updated_at = ?
		WHERE LOWER(project_id) = LOWER(?)
		AND LOWER(id) = LOWER(?)
		AND updated_at = ?`,
		newState, now, projectID, ticketID, expectedUpdatedAt)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// ListFilter contains filter options for listing tickets
type ListFilter struct {
	ProjectID string
	Status    string
	IssueType string
}

// List retrieves tickets with optional filters
func (r *TicketRepo) List(filter *ListFilter) ([]*model.Ticket, error) {
	query := "SELECT id, project_id, title, description, status, priority, issue_type, created_at, updated_at, closed_at, created_by, close_reason, agents_state FROM tickets WHERE LOWER(project_id) = LOWER(?)"
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
		ticket := &model.Ticket{}
		var createdAt, updatedAt string
		var closedAt sql.NullString

		err := rows.Scan(
			&ticket.ID,
			&ticket.ProjectID,
			&ticket.Title,
			&ticket.Description,
			&ticket.Status,
			&ticket.Priority,
			&ticket.IssueType,
			&createdAt,
			&updatedAt,
			&closedAt,
			&ticket.CreatedBy,
			&ticket.CloseReason,
			&ticket.AgentsState,
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

		tickets = append(tickets, ticket)
	}

	return tickets, nil
}

// UpdateFields contains fields that can be updated
type UpdateFields struct {
	Title       *string
	Description *string
	Status      *string
	Priority    *int
	IssueType   *string
	AgentsState *string
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
	if fields.AgentsState != nil {
		updates = append(updates, "agents_state = ?")
		args = append(args, *fields.AgentsState)
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

// Search performs FTS5 search on tickets within a project
func (r *TicketRepo) Search(projectID, query string) ([]*model.Ticket, error) {
	rows, err := r.db.Query(`
		SELECT t.id, t.project_id, t.title, t.description, t.status, t.priority, t.issue_type, t.created_at, t.updated_at, t.closed_at, t.created_by, t.close_reason, t.agents_state
		FROM tickets t
		INNER JOIN tickets_fts fts ON t.project_id = fts.project_id AND t.id = fts.id
		WHERE fts.project_id = ? AND tickets_fts MATCH ?
		ORDER BY rank`, strings.ToLower(projectID), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*model.Ticket
	for rows.Next() {
		ticket := &model.Ticket{}
		var createdAt, updatedAt string
		var closedAt sql.NullString

		err := rows.Scan(
			&ticket.ID,
			&ticket.ProjectID,
			&ticket.Title,
			&ticket.Description,
			&ticket.Status,
			&ticket.Priority,
			&ticket.IssueType,
			&createdAt,
			&updatedAt,
			&closedAt,
			&ticket.CreatedBy,
			&ticket.CloseReason,
			&ticket.AgentsState,
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

		tickets = append(tickets, ticket)
	}

	return tickets, nil
}

// PendingTicket is a ticket with blocked status info
type PendingTicket struct {
	*model.Ticket
	IsBlocked bool     `json:"is_blocked"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// MarshalJSON implements custom JSON marshaling for PendingTicket
func (pt PendingTicket) MarshalJSON() ([]byte, error) {
	// Get the ticket's marshaled form first
	ticketJSON, err := pt.Ticket.MarshalJSON()
	if err != nil {
		return nil, err
	}

	// Unmarshal into a map so we can add our fields
	var result map[string]interface{}
	if err := json.Unmarshal(ticketJSON, &result); err != nil {
		return nil, err
	}

	// Add the blocked info
	result["is_blocked"] = pt.IsBlocked
	if len(pt.BlockedBy) > 0 {
		result["blocked_by"] = pt.BlockedBy
	}

	return json.Marshal(result)
}

// GetPendingWithBlockedInfo returns non-closed tickets with their blocked status
func (r *TicketRepo) GetPendingWithBlockedInfo(projectID string, limit int) ([]*PendingTicket, error) {
	rows, err := r.db.Query(`
		SELECT t.id, t.project_id, t.title, t.description, t.status, t.priority, t.issue_type,
		       t.created_at, t.updated_at, t.closed_at, t.created_by, t.close_reason, t.agents_state
		FROM tickets t
		WHERE LOWER(t.project_id) = LOWER(?) AND t.status != 'closed'
		ORDER BY t.priority ASC, t.created_at ASC
		LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*PendingTicket
	for rows.Next() {
		ticket := &model.Ticket{}
		var createdAt, updatedAt string
		var closedAt sql.NullString

		err := rows.Scan(
			&ticket.ID,
			&ticket.ProjectID,
			&ticket.Title,
			&ticket.Description,
			&ticket.Status,
			&ticket.Priority,
			&ticket.IssueType,
			&createdAt,
			&updatedAt,
			&closedAt,
			&ticket.CreatedBy,
			&ticket.CloseReason,
			&ticket.AgentsState,
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

		tickets = append(tickets, &PendingTicket{Ticket: ticket})
	}

	// Now get blocker info for each ticket
	for _, pt := range tickets {
		blockers, err := r.getOpenBlockers(pt.ProjectID, pt.ID)
		if err != nil {
			return nil, err
		}
		pt.BlockedBy = blockers
		pt.IsBlocked = len(blockers) > 0
	}

	return tickets, nil
}

// getOpenBlockers returns IDs of open tickets that block the given ticket
func (r *TicketRepo) getOpenBlockers(projectID, ticketID string) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT blocker.id
		FROM dependencies d
		INNER JOIN tickets blocker ON d.project_id = blocker.project_id AND d.depends_on_id = blocker.id
		WHERE LOWER(d.project_id) = LOWER(?) AND LOWER(d.issue_id) = LOWER(?) AND blocker.status != 'closed'`, projectID, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blockers []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		blockers = append(blockers, id)
	}
	return blockers, nil
}

// GetRecentlyClosed returns recently closed tickets
func (r *TicketRepo) GetRecentlyClosed(projectID string, limit int) ([]*model.Ticket, error) {
	rows, err := r.db.Query(`
		SELECT id, project_id, title, description, status, priority, issue_type,
		       created_at, updated_at, closed_at, created_by, close_reason, agents_state
		FROM tickets
		WHERE LOWER(project_id) = LOWER(?) AND status = 'closed'
		ORDER BY closed_at DESC
		LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*model.Ticket
	for rows.Next() {
		ticket := &model.Ticket{}
		var createdAt, updatedAt string
		var closedAt sql.NullString

		err := rows.Scan(
			&ticket.ID,
			&ticket.ProjectID,
			&ticket.Title,
			&ticket.Description,
			&ticket.Status,
			&ticket.Priority,
			&ticket.IssueType,
			&createdAt,
			&updatedAt,
			&closedAt,
			&ticket.CreatedBy,
			&ticket.CloseReason,
			&ticket.AgentsState,
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

		tickets = append(tickets, ticket)
	}

	return tickets, nil
}

// GetReady returns tickets that are not blocked by any open dependencies
func (r *TicketRepo) GetReady(projectID string) ([]*model.Ticket, error) {
	rows, err := r.db.Query(`
		SELECT t.id, t.project_id, t.title, t.description, t.status, t.priority, t.issue_type, t.created_at, t.updated_at, t.closed_at, t.created_by, t.close_reason, t.agents_state
		FROM tickets t
		WHERE LOWER(t.project_id) = LOWER(?) AND t.status != 'closed'
		AND NOT EXISTS (
			SELECT 1 FROM dependencies d
			INNER JOIN tickets blocker ON d.project_id = blocker.project_id AND d.depends_on_id = blocker.id
			WHERE d.project_id = t.project_id AND d.issue_id = t.id AND blocker.status != 'closed'
		)
		ORDER BY t.priority ASC, t.created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*model.Ticket
	for rows.Next() {
		ticket := &model.Ticket{}
		var createdAt, updatedAt string
		var closedAt sql.NullString

		err := rows.Scan(
			&ticket.ID,
			&ticket.ProjectID,
			&ticket.Title,
			&ticket.Description,
			&ticket.Status,
			&ticket.Priority,
			&ticket.IssueType,
			&createdAt,
			&updatedAt,
			&closedAt,
			&ticket.CreatedBy,
			&ticket.CloseReason,
			&ticket.AgentsState,
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

		tickets = append(tickets, ticket)
	}

	return tickets, nil
}
