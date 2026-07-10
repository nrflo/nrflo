package repo

import (
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// MessageEntry represents a message with its category for batch insertion
type MessageEntry struct {
	Content  string
	Category string // text, tool, subagent, skill
	Payload  string
}

// MessageWithTime represents a message with its creation timestamp
type MessageWithTime struct {
	Content   string `json:"content"`
	Category  string `json:"category"`
	Payload   string `json:"payload,omitempty"`
	CreatedAt string `json:"created_at"`
}

// AgentMessageRepo handles agent message CRUD operations
type AgentMessageRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewAgentMessageRepo creates a new agent message repository
func NewAgentMessageRepo(database db.Querier, clk clock.Clock) *AgentMessageRepo {
	return &AgentMessageRepo{db: database, clock: clk}
}

// InsertBatch inserts multiple messages in a single transaction, assigning
// consecutive seq values starting at MAX(seq)+1 for the session. The seq base
// is computed inside the write transaction — which holds the write lock from
// Begin under _txlock=immediate — so concurrent writers to the same session
// (output flush, hook events, user input) cannot race on it.
func (r *AgentMessageRepo) InsertBatch(sessionID string, messages []MessageEntry) error {
	if len(messages) == 0 {
		return nil
	}
	return db.WithBusyRetry(func() error {
		return r.insertBatchOnce(sessionID, messages)
	})
}

func (r *AgentMessageRepo) insertBatchOnce(sessionID string, messages []MessageEntry) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var seqStart int
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(seq), -1) + 1 FROM agent_messages WHERE session_id = ?`,
		sessionID,
	).Scan(&seqStart); err != nil {
		return fmt.Errorf("failed to compute next seq: %w", err)
	}

	stmt, err := tx.Prepare(`INSERT INTO agent_messages (session_id, seq, content, category, created_at, payload) VALUES (?, ?, ?, ?, ?, NULLIF(?, ""))`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	for i, msg := range messages {
		cat := msg.Category
		if cat == "" {
			cat = "text"
		}
		_, err := stmt.Exec(sessionID, seqStart+i, msg.Content, cat, now, msg.Payload)
		if err != nil {
			return fmt.Errorf("failed to insert message %d: %w", seqStart+i, err)
		}
	}

	return tx.Commit()
}

// SetToolEnded stamps ended_at into the payload of the PreToolUse row matching
// tool_use_id, closing the tool span. Only the earliest unclosed match is
// updated (a retried tool_use_id keeps its first span). Returns whether a row
// was updated.
func (r *AgentMessageRepo) SetToolEnded(sessionID, toolUseID, endedAt string) (bool, error) {
	result, err := r.db.Exec(`
		UPDATE agent_messages SET payload = json_set(payload, '$.ended_at', ?)
		WHERE id = (
			SELECT id FROM agent_messages
			WHERE session_id = ? AND payload IS NOT NULL
			  AND json_extract(payload, '$.tool_use_id') = ?
			  AND json_extract(payload, '$.ended_at') IS NULL
			ORDER BY seq LIMIT 1
		)`, endedAt, sessionID, toolUseID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

// GetBySession returns all messages for a session ordered by seq
func (r *AgentMessageRepo) GetBySession(sessionID string) ([]string, error) {
	rows, err := r.db.Query(
		`SELECT content FROM agent_messages WHERE session_id = ? ORDER BY seq ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		messages = append(messages, content)
	}
	return messages, nil
}

// GetBySessionPaginated returns messages with timestamps, with limit and offset
func (r *AgentMessageRepo) GetBySessionPaginated(sessionID string, limit, offset int) ([]MessageWithTime, error) {
	rows, err := r.db.Query(
		`SELECT content, category, COALESCE(payload, ""), created_at FROM agent_messages WHERE session_id = ? ORDER BY seq ASC LIMIT ? OFFSET ?`,
		sessionID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []MessageWithTime
	for rows.Next() {
		var msg MessageWithTime
		if err := rows.Scan(&msg.Content, &msg.Category, &msg.Payload, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// GetBySessionPaginatedFiltered returns messages filtered by category
func (r *AgentMessageRepo) GetBySessionPaginatedFiltered(sessionID, category string, limit, offset int) ([]MessageWithTime, error) {
	rows, err := r.db.Query(
		`SELECT content, category, COALESCE(payload, ""), created_at FROM agent_messages WHERE session_id = ? AND category = ? ORDER BY seq ASC LIMIT ? OFFSET ?`,
		sessionID, category, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []MessageWithTime
	for rows.Next() {
		var msg MessageWithTime
		if err := rows.Scan(&msg.Content, &msg.Category, &msg.Payload, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// CountBySessionFiltered returns the message count for a session filtered by category
func (r *AgentMessageRepo) CountBySessionFiltered(sessionID, category string) (int, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM agent_messages WHERE session_id = ? AND category = ?`,
		sessionID, category,
	).Scan(&count)
	return count, err
}

// CountBySession returns the total message count for a session
func (r *AgentMessageRepo) CountBySession(sessionID string) (int, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM agent_messages WHERE session_id = ?`,
		sessionID,
	).Scan(&count)
	return count, err
}

// GetCountsBySessionIDs returns message counts for multiple sessions in one query
func (r *AgentMessageRepo) GetCountsBySessionIDs(sessionIDs []string) (map[string]int, error) {
	if len(sessionIDs) == 0 {
		return map[string]int{}, nil
	}

	placeholders := make([]string, len(sessionIDs))
	args := make([]interface{}, len(sessionIDs))
	for i, id := range sessionIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		`SELECT session_id, COUNT(*) FROM agent_messages WHERE session_id IN (%s) GROUP BY session_id`,
		strings.Join(placeholders, ","),
	)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var sessionID string
		var count int
		if err := rows.Scan(&sessionID, &count); err != nil {
			return nil, err
		}
		counts[sessionID] = count
	}
	return counts, nil
}
