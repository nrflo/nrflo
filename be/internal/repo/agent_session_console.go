package repo

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/model"
)

// getByKind returns the session by id, restricted to the given kind.
func (r *AgentSessionRepo) getByKind(id, kind string) (*model.AgentSession, error) {
	row := r.db.QueryRow(`SELECT `+sessionCols+` FROM agent_sessions WHERE id = ? AND kind = ?`, id, kind)
	s, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// closeByKind marks a session interactive_completed, killing its bearer token
// via the GetByToken status filter. The kind guard is a security requirement:
// this must never terminate a row of a different kind. Returns rows-affected;
// 0 means already closed or not a row of this kind.
func (r *AgentSessionRepo) closeByKind(id, kind string) (int64, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET status = ?, result = ?, ended_at = ?, updated_at = ?
		WHERE id = ? AND kind = ? AND status = ?`,
		model.AgentSessionInteractiveCompleted,
		sql.NullString{String: "pass", Valid: true},
		now, now, id, kind, model.AgentSessionUserInteractive,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetConsole returns the session by id, restricted to kind='console'.
func (r *AgentSessionRepo) GetConsole(id string) (*model.AgentSession, error) {
	return r.getByKind(id, model.AgentSessionKindConsole)
}

// CloseConsole marks a console session interactive_completed. See closeByKind.
func (r *AgentSessionRepo) CloseConsole(id string) (int64, error) {
	return r.closeByKind(id, model.AgentSessionKindConsole)
}

// GetConsoleChat returns the session by id, restricted to kind='console_chat'.
func (r *AgentSessionRepo) GetConsoleChat(id string) (*model.AgentSession, error) {
	return r.getByKind(id, model.AgentSessionKindConsoleChat)
}

// CloseConsoleChat marks a console-chat session interactive_completed. See
// closeByKind. Chat lifetime is otherwise owned by console.ChatService, not
// the idle sweep (ExpireIdleConsoles stays restricted to kind='console').
func (r *AgentSessionRepo) CloseConsoleChat(id string) (int64, error) {
	return r.closeByKind(id, model.AgentSessionKindConsoleChat)
}

// ListConsoleChats returns this project's kind='console_chat' sessions, most
// recently started first. limit<=0 defaults to 50. The kind filter is a
// security boundary (see closeByKind's doc comment) — never widen it.
func (r *AgentSessionRepo) ListConsoleChats(projectID string, limit int) ([]*model.AgentSession, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(`SELECT `+sessionCols+` FROM agent_sessions
		WHERE kind = ? AND LOWER(project_id) = LOWER(?)
		ORDER BY started_at DESC LIMIT ?`,
		model.AgentSessionKindConsoleChat, projectID, limit)
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
	return sessions, rows.Err()
}

// SetConsoleYolo write-throughs a chat's per-session yolo override.
func (r *AgentSessionRepo) SetConsoleYolo(id string, on bool) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE agent_sessions SET console_yolo = ?, updated_at = ? WHERE id = ?`,
		sql.NullBool{Bool: on, Valid: true}, now, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// ExpireIdleConsoles closes every console session still user_interactive whose
// updated_at is older than cutoff (RFC3339Nano). Returns the number expired.
// Restricted to kind='console': chat rows are closed via the chats/{sid}/close
// route or server shutdown (ChatService.StopAll), never this sweep.
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
