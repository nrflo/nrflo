package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// AgentSessionRepo handles agent session CRUD operations
type AgentSessionRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewAgentSessionRepo creates a new agent session repository
func NewAgentSessionRepo(database db.Querier, clk clock.Clock) *AgentSessionRepo {
	return &AgentSessionRepo{db: database, clock: clk}
}

// Create creates a new agent session
func (r *AgentSessionRepo) Create(session *model.AgentSession) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	session.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	session.UpdatedAt = session.CreatedAt

	kind := session.Kind
	if kind == "" {
		kind = model.AgentSessionKindWorkflowAgent
	}
	_, err := r.db.Exec(`
		INSERT INTO agent_sessions (`+sessionCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		strings.ToLower(session.ProjectID),
		strings.ToLower(session.TicketID),
		sql.NullString{String: session.WorkflowInstanceID, Valid: session.WorkflowInstanceID != ""},
		session.Phase,
		session.NodeID,
		session.AgentType,
		session.ModelID,
		session.Status,
		session.Result,
		session.ResultReason,
		session.PID,
		session.ContextLeft,
		session.AncestorSessionID,
		session.SpawnCommand,
		session.Prompt,
		session.SystemPrompt,
		session.RestartCount,
		0, // nudge_count defaults to 0
		session.Config,
		session.StartedAt,
		session.EndedAt,
		session.SpawnToken,
		session.EffectiveMode,
		now,
		now,
		session.RateLimitRetryCount,
		session.RateLimitUntilTs,
		session.LastRetryClass,
		kind,
		session.ObserverScope, session.ObserverWorkflowID, session.ConsoleEngine, session.ConsoleProfile, session.ConsoleYolo,
		session.SiblingOriginSessionID,
	)
	return err
}

// GetByToken returns the session matching the bearer token, only if its status
// indicates the session is still active (running or user_interactive). Returns
// (nil, nil) when no row matches — callers treat that as "invalid token".
func (r *AgentSessionRepo) GetByToken(token string) (*model.AgentSession, error) {
	if token == "" {
		return nil, nil
	}
	row := r.db.QueryRow(`SELECT `+sessionCols+` FROM agent_sessions
		WHERE spawn_token = ? AND status IN ('running', 'user_interactive')`, token)
	s, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateSpawnToken sets the bearer token for an existing session. Used when
// the PTY take-control flow resumes a session and needs a fresh token to inject
// into the agent's environment.
func (r *AgentSessionRepo) UpdateSpawnToken(id, token string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET spawn_token = ?, updated_at = ? WHERE id = ?`,
		sql.NullString{String: token, Valid: token != ""}, now, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// SetEffectiveMode updates the effective_mode column for an existing session.
func (r *AgentSessionRepo) SetEffectiveMode(id, mode string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET effective_mode = ?, updated_at = ? WHERE id = ?`,
		sql.NullString{String: mode, Valid: mode != ""}, now, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
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

// GetByProjectScope retrieves agent sessions for project-scoped workflows (empty ticket_id).
// Console/observer rows also carry an empty ticket_id, so the kind filter is what excludes them.
func (r *AgentSessionRepo) GetByProjectScope(projectID, phase string) ([]*model.AgentSession, error) {
	query := `SELECT ` + sessionCols + ` FROM agent_sessions
		WHERE LOWER(project_id) = LOWER(?) AND (ticket_id = '' OR ticket_id IS NULL)
		AND kind = '` + model.AgentSessionKindWorkflowAgent + `'`
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
