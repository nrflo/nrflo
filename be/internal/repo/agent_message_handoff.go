package repo

// TailMessage is a newest-N transcript row projection for the handoff
// composer's "Recent Uncompressed Context" section — leaner than
// MessageWithTime (no created_at) since the tail is rendered verbatim, not
// time-sorted for display.
type TailMessage struct {
	Seq      int
	Category string
	Content  string
	Payload  string
}

// GetBySessionTail returns the newest `limit` rows for a session, ordered
// ascending (oldest of the tail first) after the fetch, so callers can join
// them straight into a chronological rendering. Covered by
// idx_agent_messages_session(session_id, seq).
func (r *AgentMessageRepo) GetBySessionTail(sessionID string, limit int) ([]TailMessage, error) {
	rows, err := r.db.Query(
		`SELECT seq, category, content, COALESCE(payload, "") FROM agent_messages
		 WHERE session_id = ? ORDER BY seq DESC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []TailMessage
	for rows.Next() {
		var msg TailMessage
		if err := rows.Scan(&msg.Seq, &msg.Category, &msg.Content, &msg.Payload); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}
