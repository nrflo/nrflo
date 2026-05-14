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
	db db.Querier
}

// NewAgentSessionRepo creates a new agent session repository
func NewAgentSessionRepo(database db.Querier, clk clock.Clock) *AgentSessionRepo {
	return &AgentSessionRepo{db: database, clock: clk}
}

const sessionCols = `id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
	model_id, status, result, result_reason, pid, findings,
	context_left, ancestor_session_id, spawn_command, prompt, system_prompt,
	restart_count, nudge_count, config, started_at, ended_at, spawn_token, effective_mode, created_at, updated_at`

func scanSession(scanner interface{ Scan(...interface{}) error }) (*model.AgentSession, error) {
	s := &model.AgentSession{}
	var createdAt, updatedAt string
	err := scanner.Scan(
		&s.ID, &s.ProjectID, &s.TicketID, &s.WorkflowInstanceID, &s.Phase, &s.AgentType,
		&s.ModelID, &s.Status, &s.Result, &s.ResultReason, &s.PID, &s.Findings,
		&s.ContextLeft, &s.AncestorSessionID, &s.SpawnCommand, &s.Prompt, &s.SystemPrompt,
		&s.RestartCount, &s.NudgeCount, &s.Config, &s.StartedAt, &s.EndedAt, &s.SpawnToken, &s.EffectiveMode, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return s, nil
}

// sessionColsWithWorkflow returns columns for JOINed queries that include workflow_id
const sessionColsJoined = `s.id, s.project_id, s.ticket_id, s.workflow_instance_id, s.phase, s.agent_type,
	s.model_id, s.status, s.result, s.result_reason, s.pid, s.findings,
	s.context_left, s.ancestor_session_id, s.spawn_command, s.prompt, s.system_prompt,
	s.restart_count, s.nudge_count, s.config, s.started_at, s.ended_at, s.spawn_token, s.effective_mode, s.created_at, s.updated_at, wi.workflow_id`

func scanSessionJoined(scanner interface{ Scan(...interface{}) error }) (*model.AgentSession, error) {
	s := &model.AgentSession{}
	var createdAt, updatedAt string
	err := scanner.Scan(
		&s.ID, &s.ProjectID, &s.TicketID, &s.WorkflowInstanceID, &s.Phase, &s.AgentType,
		&s.ModelID, &s.Status, &s.Result, &s.ResultReason, &s.PID, &s.Findings,
		&s.ContextLeft, &s.AncestorSessionID, &s.SpawnCommand, &s.Prompt, &s.SystemPrompt,
		&s.RestartCount, &s.NudgeCount, &s.Config, &s.StartedAt, &s.EndedAt, &s.SpawnToken, &s.EffectiveMode, &createdAt, &updatedAt, &s.Workflow,
	)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return s, nil
}

// Create creates a new agent session
func (r *AgentSessionRepo) Create(session *model.AgentSession) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	session.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	session.UpdatedAt = session.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO agent_sessions (`+sessionCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
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
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET status = ?, updated_at = ?
		WHERE workflow_instance_id = ? AND status NOT IN ('running', 'continued')`,
		toStatus, now, wfiID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// FailRunningByInstance marks all running sessions for a workflow instance as failed.
// Used to clean up orphaned sessions after server restart.
func (r *AgentSessionRepo) FailRunningByInstance(wfiID string) (int64, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET status = 'failed', result = 'fail', result_reason = 'server_restart', ended_at = ?, updated_at = ?
		WHERE workflow_instance_id = ? AND status = 'running'`,
		now, now, wfiID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// FailAllRunning marks all running or user_interactive sessions as failed with reason=server_shutdown.
func (r *AgentSessionRepo) FailAllRunning() (int64, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET status = 'failed', result = 'fail', result_reason = 'server_shutdown', ended_at = ?, updated_at = ?
		WHERE status IN ('running', 'user_interactive')`,
		now, now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateResult updates the result and result_reason fields
func (r *AgentSessionRepo) UpdateResult(id, resultVal, reason string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`UPDATE agent_sessions SET result = ?, result_reason = ?, updated_at = ? WHERE id = ?`,
		sql.NullString{String: resultVal, Valid: resultVal != ""},
		sql.NullString{String: reason, Valid: reason != ""},
		now, id)
	return err
}

// UpdateFindings updates the findings JSON field
func (r *AgentSessionRepo) UpdateFindings(id string, findings string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`UPDATE agent_sessions SET findings = ?, updated_at = ? WHERE id = ?`,
		sql.NullString{String: findings, Valid: findings != ""},
		now, id)
	return err
}

// SetEndedAt sets the ended_at timestamp
func (r *AgentSessionRepo) SetEndedAt(id string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
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
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
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

// UpdateContextLeft updates the context_left percentage.
// Returns nil on 0 rows affected (session not in DB, e.g. interactive sessions).
func (r *AgentSessionRepo) UpdateContextLeft(id string, contextLeft int) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`UPDATE agent_sessions SET context_left = ?, updated_at = ? WHERE id = ?`,
		contextLeft, now, id)
	return err
}

// UpdateAncestorSession updates the ancestor_session_id
func (r *AgentSessionRepo) UpdateAncestorSession(id string, ancestorSessionID string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
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
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
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

// ListFinishedFilter specifies criteria for ListFinished.
type ListFinishedFilter struct {
	ProjectID string
}

// ListFinished returns paginated finished agent sessions joined with workflow_instances
// and agent_definitions. Excludes active statuses (running, continued, callback, user_interactive).
func (r *AgentSessionRepo) ListFinished(f ListFinishedFilter, page, perPage int) ([]*model.AgentSessionLogRow, int, error) {
	var total int
	if err := r.db.QueryRow(
		`SELECT COUNT(*) FROM agent_sessions s
		WHERE LOWER(s.project_id) = LOWER(?)
		AND s.status NOT IN ('running', 'continued', 'callback', 'user_interactive')`,
		f.ProjectID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count agent session logs: %w", err)
	}

	if total == 0 {
		return nil, 0, nil
	}

	offset := (page - 1) * perPage
	rows, err := r.db.Query(`
		SELECT s.id, s.project_id, s.agent_type, s.model_id, s.status,
		       s.started_at, s.ended_at, s.updated_at, s.findings,
		       s.effective_mode,
		       wi.workflow_id, s.workflow_instance_id, wi.scheduled_task_id,
		       ad.execution_mode
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		LEFT JOIN agent_definitions ad
		       ON ad.project_id = s.project_id
		       AND ad.workflow_id = wi.workflow_id
		       AND ad.id = s.agent_type
		WHERE LOWER(s.project_id) = LOWER(?)
		AND s.status NOT IN ('running', 'continued', 'callback', 'user_interactive')
		ORDER BY COALESCE(s.ended_at, s.updated_at) DESC, s.id DESC
		LIMIT ? OFFSET ?`,
		f.ProjectID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list agent session logs: %w", err)
	}
	defer rows.Close()

	var result []*model.AgentSessionLogRow
	for rows.Next() {
		row := &model.AgentSessionLogRow{}
		err := rows.Scan(
			&row.SessionID, &row.ProjectID, &row.AgentType, &row.ModelID, &row.Status,
			&row.StartedAt, &row.EndedAt, &row.UpdatedAt, &row.Findings,
			&row.EffectiveMode,
			&row.WorkflowID, &row.WorkflowInstanceID, &row.ScheduledTaskID,
			&row.ExecutionMode,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan agent session log row: %w", err)
		}
		result = append(result, row)
	}
	return result, total, rows.Err()
}

// ListLiveByProject returns running/user_interactive sessions with pid > 0 for the given project,
// joined with workflow_instances and agent_definitions.
func (r *AgentSessionRepo) ListLiveByProject(projectID string) ([]*model.AgentSessionLogRow, error) {
	rows, err := r.db.Query(`
		SELECT s.id, s.project_id, s.agent_type, s.model_id, s.status,
		       s.started_at, s.ended_at, s.updated_at, s.findings,
		       s.effective_mode, s.pid,
		       wi.workflow_id, s.workflow_instance_id, wi.scheduled_task_id,
		       ad.execution_mode
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		LEFT JOIN agent_definitions ad
		       ON ad.project_id = s.project_id
		       AND ad.workflow_id = wi.workflow_id
		       AND ad.id = s.agent_type
		WHERE LOWER(s.project_id) = LOWER(?)
		AND s.status IN ('running', 'user_interactive')
		AND s.pid IS NOT NULL AND s.pid > 0
		ORDER BY s.started_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list live agent sessions: %w", err)
	}
	defer rows.Close()

	var result []*model.AgentSessionLogRow
	for rows.Next() {
		row := &model.AgentSessionLogRow{}
		err := rows.Scan(
			&row.SessionID, &row.ProjectID, &row.AgentType, &row.ModelID, &row.Status,
			&row.StartedAt, &row.EndedAt, &row.UpdatedAt, &row.Findings,
			&row.EffectiveMode, &row.PID,
			&row.WorkflowID, &row.WorkflowInstanceID, &row.ScheduledTaskID,
			&row.ExecutionMode,
		)
		if err != nil {
			return nil, fmt.Errorf("scan live agent session row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// CleanupOrphanedMessages deletes agent_messages whose session no longer exists
// (e.g. removed by CASCADE when a workflow_instance was deleted).
func (r *AgentSessionRepo) CleanupOrphanedMessages() (int64, error) {
	result, err := r.db.Exec(`
		DELETE FROM agent_messages
		WHERE session_id NOT IN (SELECT id FROM agent_sessions)`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateStatusToInteractiveCompleted sets status to interactive_completed, result to pass, and ended_at to now.
func (r *AgentSessionRepo) UpdateStatusToInteractiveCompleted(id string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET status = ?, result = ?, ended_at = ?, updated_at = ? WHERE id = ?`,
		model.AgentSessionInteractiveCompleted,
		sql.NullString{String: "pass", Valid: true},
		now, now, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// UpdateStatusToFailedWithReason sets status=failed, result=fail, result_reason=reason, ended_at=now.
func (r *AgentSessionRepo) UpdateStatusToFailedWithReason(id string, reason string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET status = ?, result = ?, result_reason = ?, ended_at = ?, updated_at = ? WHERE id = ?`,
		model.AgentSessionFailed,
		sql.NullString{String: "fail", Valid: true},
		sql.NullString{String: reason, Valid: true},
		now, now, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// CountRunning returns the number of currently running agent sessions across all projects.
func (r *AgentSessionRepo) CountRunning() (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM agent_sessions WHERE status = 'running'`).Scan(&count)
	return count, err
}

// GetRunning retrieves currently running agent sessions across all projects
func (r *AgentSessionRepo) GetRunning(limit int) ([]*model.AgentSession, error) {
	rows, err := r.db.Query(`
		SELECT `+sessionColsJoined+`
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		WHERE s.status = 'running'
		ORDER BY s.started_at ASC LIMIT ?`, limit)
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
