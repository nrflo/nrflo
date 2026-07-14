package repo

import (
	"database/sql"
	"time"

	"be/internal/model"
)

// GetConsole returns the session by id, restricted to kind='console'.
func (r *AgentSessionRepo) GetConsole(id string) (*model.AgentSession, error) {
	row := r.db.QueryRow(`SELECT `+sessionCols+` FROM agent_sessions WHERE id = ? AND kind = 'console'`, id)
	s, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// CloseConsole marks a console session interactive_completed, killing its
// bearer token via the GetByToken status filter. The kind guard is a security
// requirement: this must never terminate a workflow-agent session. Returns
// rows-affected; 0 means already closed or not a console row.
func (r *AgentSessionRepo) CloseConsole(id string) (int64, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET status = ?, result = ?, ended_at = ?, updated_at = ?
		WHERE id = ? AND kind = 'console' AND status = ?`,
		model.AgentSessionInteractiveCompleted,
		sql.NullString{String: "pass", Valid: true},
		now, now, id, model.AgentSessionUserInteractive,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ExpireIdleConsoles closes every console session still user_interactive whose
// updated_at is older than cutoff (RFC3339Nano). Returns the number expired.
func (r *AgentSessionRepo) ExpireIdleConsoles(cutoff string) (int64, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET status = ?, result = ?, result_reason = ?, ended_at = ?, updated_at = ?
		WHERE kind = 'console' AND status = ? AND updated_at < ?`,
		model.AgentSessionInteractiveCompleted,
		sql.NullString{String: "pass", Valid: true},
		sql.NullString{String: "console_idle_expired", Valid: true},
		now, now, model.AgentSessionUserInteractive, cutoff,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
