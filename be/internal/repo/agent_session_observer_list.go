package repo

import (
	"be/internal/model"
)

// ListActiveObservers returns observer sessions with status running or user_interactive.
// If projectID is non-empty, filters to that project.
func (r *AgentSessionRepo) ListActiveObservers(projectID string) ([]*model.AgentSession, error) {
	var (
		rows interface {
			Next() bool
			Scan(...interface{}) error
			Close() error
			Err() error
		}
		err error
	)

	if projectID != "" {
		rows, err = r.db.Query(`
			SELECT `+sessionCols+`
			FROM agent_sessions
			WHERE LOWER(project_id) = LOWER(?)
			AND kind = 'observer'
			AND status IN ('running', 'user_interactive')
			ORDER BY started_at DESC`, projectID)
	} else {
		rows, err = r.db.Query(`
			SELECT `+sessionCols+`
			FROM agent_sessions
			WHERE kind = 'observer'
			AND status IN ('running', 'user_interactive')
			ORDER BY started_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.AgentSession
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
