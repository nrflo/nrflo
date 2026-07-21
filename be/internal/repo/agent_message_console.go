package repo

import (
	"be/internal/model"
)

// ProjectConsoleUserInputs returns projectID's most recent 'user_input'
// message contents across all of its kind='console_chat' sessions
// (agent_session_console.go's security boundary — never widen the kind
// filter), oldest→newest, consecutive-deduped. limit<=0 or >100 clamps to
// 100. Used to seed the native console TUI's Up/Down recall history from a
// project-scoped aggregate rather than a single session's tail
// (consoleui/input_history.go).
func (r *AgentMessageRepo) ProjectConsoleUserInputs(projectID string, limit int) ([]string, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := r.db.Query(`
		SELECT m.content FROM agent_messages m
		JOIN agent_sessions s ON s.id = m.session_id
		WHERE s.kind = ? AND LOWER(s.project_id) = LOWER(?) AND m.category = 'user_input'
		ORDER BY m.created_at DESC, m.seq DESC LIMIT ?`,
		model.AgentSessionKindConsoleChat, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var desc []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		desc = append(desc, content)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(desc))
	for i := len(desc) - 1; i >= 0; i-- {
		content := desc[i]
		if len(out) > 0 && out[len(out)-1] == content {
			continue
		}
		out = append(out, content)
	}
	return out, nil
}
