package repo

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/model"
)

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

// UpdateRateLimitUntil persists rate-limit state after a rate-limited exit.
// ts is an RFC3339Nano timestamp for when the rate limit is expected to lift.
// retryCount is the 1-based count after this retry; lastRetryClass is the
// matched pattern string (stored in last_retry_class for observability).
func (r *AgentSessionRepo) UpdateRateLimitUntil(id, ts string, retryCount int, lastRetryClass string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`UPDATE agent_sessions SET rate_limit_until_ts = ?, rate_limit_retry_count = ?, last_retry_class = ?, updated_at = ? WHERE id = ?`,
		sql.NullString{String: ts, Valid: ts != ""},
		retryCount,
		sql.NullString{String: lastRetryClass, Valid: lastRetryClass != ""},
		now, id)
	return err
}

// GetRateLimitResetTs returns the anticipated subscription reset timestamp
// (RFC3339) recorded from the statusline rate_limits payload, or "" when unset.
func (r *AgentSessionRepo) GetRateLimitResetTs(id string) (string, error) {
	var ts sql.NullString
	err := r.db.QueryRow(`SELECT rate_limit_reset_ts FROM agent_sessions WHERE id = ?`, id).Scan(&ts)
	if err != nil {
		return "", err
	}
	return ts.String, nil
}

// SetEndedAt sets the ended_at timestamp
func (r *AgentSessionRepo) SetEndedAt(id string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`UPDATE agent_sessions SET ended_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id)
	return err
}

// SetSpawnRuntime records the runtime pid and final spawn command on a session
// row once its backend has started. A zero pid or empty spawnCommand leaves the
// existing column untouched (api agents have no child pid; some backends set
// spawn_command before Start, others only during it).
func (r *AgentSessionRepo) SetSpawnRuntime(id string, pid int, spawnCommand string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	res, err := r.db.Exec(`
		UPDATE agent_sessions
		SET pid = CASE WHEN ? > 0 THEN ? ELSE pid END,
		    spawn_command = CASE WHEN ? <> '' THEN ? ELSE spawn_command END,
		    updated_at = ?
		WHERE id = ?`,
		pid, pid, spawnCommand, spawnCommand, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
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
