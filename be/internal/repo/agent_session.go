package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/db"
	"be/internal/model"
)

// AgentSessionRepo handles agent session CRUD operations
type AgentSessionRepo struct {
	db db.Querier
}

// NewAgentSessionRepo creates a new agent session repository
func NewAgentSessionRepo(database db.Querier) *AgentSessionRepo {
	return &AgentSessionRepo{db: database}
}

const sessionCols = `id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
	model_id, status, result, result_reason, pid, findings,
	context_left, ancestor_session_id, spawn_command, prompt_context,
	restart_count, started_at, ended_at, created_at, updated_at`

func scanSession(scanner interface{ Scan(...interface{}) error }) (*model.AgentSession, error) {
	s := &model.AgentSession{}
	var createdAt, updatedAt string
	err := scanner.Scan(
		&s.ID, &s.ProjectID, &s.TicketID, &s.WorkflowInstanceID, &s.Phase, &s.AgentType,
		&s.ModelID, &s.Status, &s.Result, &s.ResultReason, &s.PID, &s.Findings,
		&s.ContextLeft, &s.AncestorSessionID, &s.SpawnCommand, &s.PromptContext,
		&s.RestartCount, &s.StartedAt, &s.EndedAt, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return s, nil
}

// sessionColsWithWorkflow returns columns for JOINed queries that include workflow_id
const sessionColsJoined = `s.id, s.project_id, s.ticket_id, s.workflow_instance_id, s.phase, s.agent_type,
	s.model_id, s.status, s.result, s.result_reason, s.pid, s.findings,
	s.context_left, s.ancestor_session_id, s.spawn_command, s.prompt_context,
	s.restart_count, s.started_at, s.ended_at, s.created_at, s.updated_at, wi.workflow_id`

func scanSessionJoined(scanner interface{ Scan(...interface{}) error }) (*model.AgentSession, error) {
	s := &model.AgentSession{}
	var createdAt, updatedAt string
	err := scanner.Scan(
		&s.ID, &s.ProjectID, &s.TicketID, &s.WorkflowInstanceID, &s.Phase, &s.AgentType,
		&s.ModelID, &s.Status, &s.Result, &s.ResultReason, &s.PID, &s.Findings,
		&s.ContextLeft, &s.AncestorSessionID, &s.SpawnCommand, &s.PromptContext,
		&s.RestartCount, &s.StartedAt, &s.EndedAt, &createdAt, &updatedAt, &s.Workflow,
	)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return s, nil
}

// Create creates a new agent session
func (r *AgentSessionRepo) Create(session *model.AgentSession) error {
	now := time.Now().UTC().Format(time.RFC3339)
	session.CreatedAt, _ = time.Parse(time.RFC3339, now)
	session.UpdatedAt = session.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO agent_sessions (`+sessionCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		strings.ToLower(session.ProjectID),
		strings.ToLower(session.TicketID),
		session.WorkflowInstanceID,
		session.Phase,
		session.AgentType,
		session.ModelID,
		session.Status,
		session.Result,
		session.ResultReason,
		session.PID,
		session.Findings,
		session.ContextLeft,
		session.AncestorSessionID,
		session.SpawnCommand,
		session.PromptContext,
		session.RestartCount,
		session.StartedAt,
		session.EndedAt,
		now,
		now,
	)
	return err
}

// Get retrieves an agent session by ID
func (r *AgentSessionRepo) Get(id string) (*model.AgentSession, error) {
	row := r.db.QueryRow(`SELECT `+sessionCols+` FROM agent_sessions WHERE id = ?`, id)
	s, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent session not found: %s", id)
	}
	return s, err
}

// GetByTicket retrieves agent sessions for a ticket
func (r *AgentSessionRepo) GetByTicket(projectID, ticketID string, phase string) ([]*model.AgentSession, error) {
	query := `SELECT ` + sessionCols + ` FROM agent_sessions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?)`
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
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// GetByProjectScope retrieves agent sessions for project-scoped workflows (empty ticket_id)
func (r *AgentSessionRepo) GetByProjectScope(projectID, phase string) ([]*model.AgentSession, error) {
	query := `SELECT ` + sessionCols + ` FROM agent_sessions
		WHERE LOWER(project_id) = LOWER(?) AND (ticket_id = '' OR ticket_id IS NULL)`
	args := []interface{}{projectID}

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
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// UpdateStatus updates the status of a session
func (r *AgentSessionRepo) UpdateStatus(id string, status model.AgentSessionStatus) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET status = ?, updated_at = ? WHERE id = ?`,
		status, now, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// UpdateStatusByWorkflowInstance bulk-updates agent session statuses for a workflow instance,
// excluding running and continued sessions.
func (r *AgentSessionRepo) UpdateStatusByWorkflowInstance(wfiID string, toStatus model.AgentSessionStatus) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET status = ?, updated_at = ?
		WHERE workflow_instance_id = ? AND status NOT IN ('running', 'continued')`,
		toStatus, now, wfiID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateResult updates the result and result_reason fields
func (r *AgentSessionRepo) UpdateResult(id, resultVal, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(
		`UPDATE agent_sessions SET result = ?, result_reason = ?, updated_at = ? WHERE id = ?`,
		sql.NullString{String: resultVal, Valid: resultVal != ""},
		sql.NullString{String: reason, Valid: reason != ""},
		now, id)
	return err
}

// UpdateFindings updates the findings JSON field
func (r *AgentSessionRepo) UpdateFindings(id string, findings string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(
		`UPDATE agent_sessions SET findings = ?, updated_at = ? WHERE id = ?`,
		sql.NullString{String: findings, Valid: findings != ""},
		now, id)
	return err
}

// SetEndedAt sets the ended_at timestamp
func (r *AgentSessionRepo) SetEndedAt(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(
		`UPDATE agent_sessions SET ended_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id)
	return err
}

// Delete deletes an agent session
func (r *AgentSessionRepo) Delete(id string) error {
	result, err := r.db.Exec("DELETE FROM agent_sessions WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// UpdateRestartCount updates the restart_count field
func (r *AgentSessionRepo) UpdateRestartCount(id string, count int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET restart_count = ?, updated_at = ? WHERE id = ?`,
		count, now, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// UpdateContextLeft updates the context_left percentage
func (r *AgentSessionRepo) UpdateContextLeft(id string, contextLeft int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET context_left = ?, updated_at = ? WHERE id = ?`,
		contextLeft, now, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// UpdateAncestorSession updates the ancestor_session_id
func (r *AgentSessionRepo) UpdateAncestorSession(id string, ancestorSessionID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET ancestor_session_id = ?, updated_at = ? WHERE id = ?`,
		sql.NullString{String: ancestorSessionID, Valid: ancestorSessionID != ""}, now, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// DeleteByTicket deletes all agent sessions for a ticket
func (r *AgentSessionRepo) DeleteByTicket(projectID, ticketID string) error {
	_, err := r.db.Exec("DELETE FROM agent_sessions WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?)", projectID, ticketID)
	return err
}

// ResetSessionsForCallback marks sessions as callback and clears their findings
// for the given workflow instance and phase names. Excludes running and continued sessions.
func (r *AgentSessionRepo) ResetSessionsForCallback(wfiID string, phases []string) error {
	if len(phases) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// Build placeholders for IN clause
	placeholders := make([]string, len(phases))
	args := make([]interface{}, 0, len(phases)+3)
	args = append(args, now, now, wfiID)
	for i, p := range phases {
		placeholders[i] = "?"
		args = append(args, p)
	}
	query := fmt.Sprintf(
		`UPDATE agent_sessions SET status = 'callback', findings = '{}', ended_at = COALESCE(ended_at, ?), updated_at = ?
		WHERE workflow_instance_id = ? AND phase IN (%s) AND status NOT IN ('running', 'continued')`,
		strings.Join(placeholders, ","))
	_, err := r.db.Exec(query, args...)
	return err
}

// GetRecent retrieves the most recent agent sessions
func (r *AgentSessionRepo) GetRecent(limit int) ([]*model.AgentSession, error) {
	rows, err := r.db.Query(`
		SELECT `+sessionColsJoined+`
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		ORDER BY s.updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*model.AgentSession
	for rows.Next() {
		s, err := scanSessionJoined(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}
