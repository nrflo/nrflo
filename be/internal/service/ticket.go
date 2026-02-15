package service

import (
	"database/sql"
	"fmt"
	"os/user"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/id"
	"be/internal/model"
	"be/internal/types"
)

const ticketCols = `id, project_id, title, description, status, priority, issue_type, created_at, updated_at, closed_at, created_by, close_reason`
const ticketColsT = `t.id, t.project_id, t.title, t.description, t.status, t.priority, t.issue_type, t.created_at, t.updated_at, t.closed_at, t.created_by, t.close_reason`

func scanTicketRow(scanner interface{ Scan(...interface{}) error }) (*model.Ticket, error) {
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
		&createdAt,
		&updatedAt,
		&closedAt,
		&ticket.CreatedBy,
		&ticket.CloseReason,
	)
	if err != nil {
		return nil, err
	}

	ticket.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	ticket.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if closedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, closedAt.String)
		ticket.ClosedAt = sql.NullTime{Time: t, Valid: true}
	}

	return ticket, nil
}

// TicketService handles ticket business logic
type TicketService struct {
	clock clock.Clock
	pool   *db.Pool
}

// NewTicketService creates a new ticket service
func NewTicketService(pool *db.Pool, clk clock.Clock) *TicketService {
	return &TicketService{pool: pool, clock: clk}
}

// Create creates a new ticket
func (s *TicketService) Create(projectID string, req *types.TicketCreateRequest) (*model.Ticket, error) {
	// Verify project exists
	var projectExists bool
	err := s.pool.QueryRow("SELECT EXISTS(SELECT 1 FROM projects WHERE LOWER(id) = LOWER(?))", projectID).Scan(&projectExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check project: %w", err)
	}
	if !projectExists {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	// Use provided ID or generate one
	var ticketID string
	if req.ID != "" {
		ticketID = strings.ToLower(req.ID)
	} else {
		gen := id.New(strings.ToUpper(projectID))
		var err error
		ticketID, err = gen.Generate()
		if err != nil {
			return nil, fmt.Errorf("failed to generate ID: %w", err)
		}
	}

	// Get current user
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}
	username := currentUser.Username

	// Default issue type
	issueType := model.IssueType(req.Type)
	if issueType == "" {
		issueType = model.IssueTypeTask
	}
	switch issueType {
	case model.IssueTypeBug, model.IssueTypeFeature, model.IssueTypeTask, model.IssueTypeEpic:
		// valid
	default:
		return nil, fmt.Errorf("invalid issue type: %s (must be bug, feature, task, or epic)", req.Type)
	}

	// Default priority
	priority := req.Priority
	if priority == 0 {
		priority = 2
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	ticket := &model.Ticket{
		ID:        ticketID,
		ProjectID: projectID,
		Title:     req.Title,
		Status:    model.StatusOpen,
		Priority:  priority,
		IssueType: issueType,
		CreatedBy: username,
	}

	if req.Description != "" {
		ticket.Description = sql.NullString{String: req.Description, Valid: true}
	}

	_, err = s.pool.Exec(`
		INSERT INTO tickets (id, project_id, title, description, status, priority, issue_type, created_at, updated_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create ticket: %w", err)
	}

	ticket.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	ticket.UpdatedAt = ticket.CreatedAt

	return ticket, nil
}

// Get retrieves a ticket by ID
func (s *TicketService) Get(projectID, ticketID string) (*model.Ticket, error) {
	row := s.pool.QueryRow(`
		SELECT `+ticketCols+`
		FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`, projectID, ticketID)
	ticket, err := scanTicketRow(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ticket not found: %s", ticketID)
	}
	return ticket, err
}

// List lists tickets with optional filters
func (s *TicketService) List(projectID string, req *types.TicketListRequest) ([]*model.Ticket, error) {
	query := "SELECT " + ticketCols + " FROM tickets WHERE LOWER(project_id) = LOWER(?)"
	args := []interface{}{projectID}

	if req != nil {
		if req.Status != "" {
			query += " AND status = ?"
			args = append(args, req.Status)
		}
		if req.Type != "" {
			query += " AND issue_type = ?"
			args = append(args, req.Type)
		}
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*model.Ticket
	for rows.Next() {
		ticket, err := scanTicketRow(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}

	return tickets, nil
}

// Update updates a ticket
func (s *TicketService) Update(projectID, ticketID string, req *types.TicketUpdateRequest) error {
	// First check if ticket exists
	_, err := s.Get(projectID, ticketID)
	if err != nil {
		return err
	}

	updates := []string{}
	args := []interface{}{}

	if req.Title != nil {
		updates = append(updates, "title = ?")
		args = append(args, *req.Title)
	}
	if req.Description != nil {
		updates = append(updates, "description = ?")
		args = append(args, *req.Description)
	}
	if req.Status != nil {
		updates = append(updates, "status = ?")
		args = append(args, *req.Status)
	}
	if req.Priority != nil {
		updates = append(updates, "priority = ?")
		args = append(args, *req.Priority)
	}
	if req.Type != nil {
		updates = append(updates, "issue_type = ?")
		args = append(args, *req.Type)
	}

	if len(updates) == 0 {
		return nil
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
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

	_, err = s.pool.Exec(query, args...)
	return err
}

// SetInProgress sets a ticket's status to in_progress, but only if currently open.
func (s *TicketService) SetInProgress(projectID, ticketID string) error {
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.pool.Exec(`
		UPDATE tickets SET status = ?, updated_at = ?
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?) AND status = ?`,
		model.StatusInProgress, now, projectID, ticketID, model.StatusOpen)
	return err
}

// Close closes a ticket
func (s *TicketService) Close(projectID, ticketID string, reason string) error {
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	var closeReason interface{}
	if reason != "" {
		closeReason = reason
	}

	result, err := s.pool.Exec(`
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

// Reopen reopens a ticket by setting status back to open and clearing close metadata.
func (s *TicketService) Reopen(projectID, ticketID string) error {
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.pool.Exec(`
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
func (s *TicketService) Delete(projectID, ticketID string) error {
	result, err := s.pool.Exec("DELETE FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)", projectID, ticketID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}
	return nil
}
