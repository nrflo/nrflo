package repo

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/model"
)

// UpdateStatusEnded sets status and ended_at for a session.
func (r *AgentSessionRepo) UpdateStatusEnded(id string, status model.AgentSessionStatus) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET status = ?, ended_at = ?, updated_at = ? WHERE id = ?`,
		status, now, now, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
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

// GetRunning retrieves running and rate-limited (continued+non-null rate_limit_until_ts) agent
// sessions across all projects. Callers are responsible for filtering continued rows by
// comparing rate_limit_until_ts against clock.Now().
func (r *AgentSessionRepo) GetRunning(limit int) ([]*model.AgentSession, error) {
	rows, err := r.db.Query(`
		SELECT `+sessionColsJoined+`
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		WHERE (s.status = 'running' OR (s.status = 'continued' AND s.rate_limit_until_ts IS NOT NULL))
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
