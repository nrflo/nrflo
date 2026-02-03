package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"nrworkflow/internal/db"
	"nrworkflow/internal/model"
)

// AgentSessionRepo handles agent session CRUD operations
type AgentSessionRepo struct {
	db *db.DB
}

// NewAgentSessionRepo creates a new agent session repository
func NewAgentSessionRepo(database *db.DB) *AgentSessionRepo {
	return &AgentSessionRepo{db: database}
}

// Create creates a new agent session
func (r *AgentSessionRepo) Create(session *model.AgentSession) error {
	now := time.Now().UTC().Format(time.RFC3339)
	session.CreatedAt, _ = time.Parse(time.RFC3339, now)
	session.UpdatedAt = session.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, phase, workflow, agent_type, model_id, status, last_messages, message_stats, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		strings.ToLower(session.ProjectID),
		strings.ToLower(session.TicketID),
		session.Phase,
		session.Workflow,
		session.AgentType,
		session.ModelID,
		session.Status,
		session.LastMessages,
		session.MessageStats,
		now,
		now,
	)
	return err
}

// Get retrieves an agent session by ID
func (r *AgentSessionRepo) Get(id string) (*model.AgentSession, error) {
	session := &model.AgentSession{}
	var createdAt, updatedAt string

	err := r.db.QueryRow(`
		SELECT id, project_id, ticket_id, phase, workflow, agent_type, model_id, status, last_messages, message_stats, created_at, updated_at
		FROM agent_sessions WHERE id = ?`, id).Scan(
		&session.ID,
		&session.ProjectID,
		&session.TicketID,
		&session.Phase,
		&session.Workflow,
		&session.AgentType,
		&session.ModelID,
		&session.Status,
		&session.LastMessages,
		&session.MessageStats,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	session.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return session, nil
}

// GetByTicket retrieves agent sessions for a ticket, optionally filtered by phase
func (r *AgentSessionRepo) GetByTicket(projectID, ticketID string, phase string) ([]*model.AgentSession, error) {
	query := `
		SELECT id, project_id, ticket_id, phase, workflow, agent_type, model_id, status, last_messages, message_stats, created_at, updated_at
		FROM agent_sessions WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?)`
	args := []interface{}{projectID, ticketID}

	if phase != "" {
		query += " AND phase = ?"
		args = append(args, phase)
	}

	query += " ORDER BY created_at DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*model.AgentSession
	for rows.Next() {
		session := &model.AgentSession{}
		var createdAt, updatedAt string

		err := rows.Scan(
			&session.ID,
			&session.ProjectID,
			&session.TicketID,
			&session.Phase,
			&session.Workflow,
			&session.AgentType,
			&session.ModelID,
			&session.Status,
			&session.LastMessages,
			&session.MessageStats,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		session.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// UpdateMessages updates the last_messages JSON for a session
func (r *AgentSessionRepo) UpdateMessages(id string, messagesJSON string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := r.db.Exec(`
		UPDATE agent_sessions SET last_messages = ?, updated_at = ?
		WHERE id = ?`,
		messagesJSON, now, id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// UpdateStats updates the message_stats JSON for a session
func (r *AgentSessionRepo) UpdateStats(id string, statsJSON string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := r.db.Exec(`
		UPDATE agent_sessions SET message_stats = ?, updated_at = ?
		WHERE id = ?`,
		statsJSON, now, id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// UpdateStatus updates the status of a session
func (r *AgentSessionRepo) UpdateStatus(id string, status model.AgentSessionStatus) error {
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := r.db.Exec(`
		UPDATE agent_sessions SET status = ?, updated_at = ?
		WHERE id = ?`,
		status, now, id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// Delete deletes an agent session
func (r *AgentSessionRepo) Delete(id string) error {
	result, err := r.db.Exec("DELETE FROM agent_sessions WHERE id = ?", id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// DeleteByTicket deletes all agent sessions for a ticket
func (r *AgentSessionRepo) DeleteByTicket(projectID, ticketID string) error {
	_, err := r.db.Exec("DELETE FROM agent_sessions WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?)", projectID, ticketID)
	return err
}

// GetRecent retrieves the most recent agent sessions across all projects
func (r *AgentSessionRepo) GetRecent(limit int) ([]*model.AgentSession, error) {
	query := `
		SELECT id, project_id, ticket_id, phase, workflow, agent_type, model_id, status, last_messages, message_stats, created_at, updated_at
		FROM agent_sessions
		ORDER BY updated_at DESC
		LIMIT ?`

	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*model.AgentSession
	for rows.Next() {
		session := &model.AgentSession{}
		var createdAt, updatedAt string

		err := rows.Scan(
			&session.ID,
			&session.ProjectID,
			&session.TicketID,
			&session.Phase,
			&session.Workflow,
			&session.AgentType,
			&session.ModelID,
			&session.Status,
			&session.LastMessages,
			&session.MessageStats,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		session.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		sessions = append(sessions, session)
	}

	return sessions, nil
}
