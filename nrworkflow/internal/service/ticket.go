package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os/user"
	"strings"
	"time"

	"nrworkflow/internal/db"
	"nrworkflow/internal/id"
	"nrworkflow/internal/model"
	"nrworkflow/internal/types"
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

	ticket.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	ticket.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if closedAt.Valid {
		t, _ := time.Parse(time.RFC3339, closedAt.String)
		ticket.ClosedAt = sql.NullTime{Time: t, Valid: true}
	}

	return ticket, nil
}

// TicketService handles ticket business logic
type TicketService struct {
	pool *db.Pool
}

// NewTicketService creates a new ticket service
func NewTicketService(pool *db.Pool) *TicketService {
	return &TicketService{pool: pool}
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

	now := time.Now().UTC().Format(time.RFC3339)

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

	ticket.CreatedAt, _ = time.Parse(time.RFC3339, now)
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

	_, err = s.pool.Exec(query, args...)
	return err
}

// SetInProgress sets a ticket's status to in_progress, but only if currently open.
// Returns nil if the ticket is not open (no-op).
func (s *TicketService) SetInProgress(projectID, ticketID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.pool.Exec(`
		UPDATE tickets SET status = ?, updated_at = ?
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?) AND status = ?`,
		model.StatusInProgress, now, projectID, ticketID, model.StatusOpen)
	return err
}

// Close closes a ticket
func (s *TicketService) Close(projectID, ticketID string, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)

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

// Search searches tickets
func (s *TicketService) Search(projectID, query string) ([]*model.Ticket, error) {
	rows, err := s.pool.Query(`
		SELECT `+ticketColsT+`
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
		ticket, err := scanTicketRow(rows)
		if err != nil {
			return nil, err
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
	ticketJSON, err := pt.Ticket.MarshalJSON()
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(ticketJSON, &result); err != nil {
		return nil, err
	}

	result["is_blocked"] = pt.IsBlocked
	if len(pt.BlockedBy) > 0 {
		result["blocked_by"] = pt.BlockedBy
	}

	return json.Marshal(result)
}

// GetReady returns tickets that are not blocked
func (s *TicketService) GetReady(projectID string) ([]*model.Ticket, error) {
	rows, err := s.pool.Query(`
		SELECT `+ticketColsT+`
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
		ticket, err := scanTicketRow(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}

	return tickets, nil
}

// GetStatus returns ticket status summary
func (s *TicketService) GetStatus(projectID string, pendingLimit, completedLimit int) (map[string]interface{}, error) {
	// Get pending tickets
	rows, err := s.pool.Query(`
		SELECT `+ticketColsT+`
		FROM tickets t
		WHERE LOWER(t.project_id) = LOWER(?) AND t.status != 'closed'
		ORDER BY t.priority ASC, t.created_at ASC
		LIMIT ?`, projectID, pendingLimit)
	if err != nil {
		return nil, err
	}

	var pending []*PendingTicket
	for rows.Next() {
		ticket, err := scanTicketRow(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		pending = append(pending, &PendingTicket{Ticket: ticket})
	}
	rows.Close()

	// Get blockers for each pending ticket
	for _, pt := range pending {
		blockers, err := s.getOpenBlockers(pt.ProjectID, pt.ID)
		if err != nil {
			return nil, err
		}
		pt.BlockedBy = blockers
		pt.IsBlocked = len(blockers) > 0
	}

	// Get completed tickets
	rows, err = s.pool.Query(`
		SELECT `+ticketCols+`
		FROM tickets
		WHERE LOWER(project_id) = LOWER(?) AND status = 'closed'
		ORDER BY closed_at DESC
		LIMIT ?`, projectID, completedLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var completed []*model.Ticket
	for rows.Next() {
		ticket, err := scanTicketRow(rows)
		if err != nil {
			return nil, err
		}
		completed = append(completed, ticket)
	}

	return map[string]interface{}{
		"pending":   pending,
		"completed": completed,
	}, nil
}

func (s *TicketService) getOpenBlockers(projectID, ticketID string) ([]string, error) {
	rows, err := s.pool.Query(`
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

// AddDependency adds a dependency between tickets
func (s *TicketService) AddDependency(projectID, child, parent string) error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	_, err = s.pool.Exec(`
		INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_at, created_by)
		VALUES (?, ?, ?, 'blocks', ?, ?)`,
		strings.ToLower(projectID),
		strings.ToLower(child),
		strings.ToLower(parent),
		now,
		currentUser.Username,
	)
	return err
}

// RemoveDependency removes a dependency between tickets
func (s *TicketService) RemoveDependency(projectID, child, parent string) error {
	result, err := s.pool.Exec(
		"DELETE FROM dependencies WHERE LOWER(project_id) = LOWER(?) AND LOWER(issue_id) = LOWER(?) AND LOWER(depends_on_id) = LOWER(?)",
		projectID, child, parent)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("dependency not found")
	}
	return nil
}
