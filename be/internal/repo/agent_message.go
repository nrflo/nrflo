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
}

// MessageWithTime represents a message with its creation timestamp
type MessageWithTime struct {
	Content   string `json:"content"`
	Category  string `json:"category"`
	CreatedAt string `json:"created_at"`
}

// AgentMessageRepo handles agent message CRUD operations
type AgentMessageRepo struct {
	clock clock.Clock
	db db.Querier
}

// NewAgentMessageRepo creates a new agent message repository
func NewAgentMessageRepo(database db.Querier, clk clock.Clock) *AgentMessageRepo {
	return &AgentMessageRepo{db: database, clock: clk}
}

// InsertBatch inserts multiple messages in a single transaction
func (r *AgentMessageRepo) InsertBatch(sessionID string, seqStart int, messages []MessageEntry) error {
	if len(messages) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO agent_messages (session_id, seq, content, category, created_at) VALUES (?, ?, ?, ?, ?)`)
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
		_, err := stmt.Exec(sessionID, seqStart+i, msg.Content, cat, now)
		if err != nil {
			return fmt.Errorf("failed to insert message %d: %w", seqStart+i, err)
		}
	}

	return tx.Commit()
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
		`SELECT content, category, created_at FROM agent_messages WHERE session_id = ? ORDER BY seq ASC LIMIT ? OFFSET ?`,
		sessionID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []MessageWithTime
	for rows.Next() {
		var msg MessageWithTime
		if err := rows.Scan(&msg.Content, &msg.Category, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// GetBySessionPaginatedFiltered returns messages filtered by category
func (r *AgentMessageRepo) GetBySessionPaginatedFiltered(sessionID, category string, limit, offset int) ([]MessageWithTime, error) {
	rows, err := r.db.Query(
		`SELECT content, category, created_at FROM agent_messages WHERE session_id = ? AND category = ? ORDER BY seq ASC LIMIT ? OFFSET ?`,
		sessionID, category, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []MessageWithTime
	for rows.Next() {
		var msg MessageWithTime
		if err := rows.Scan(&msg.Content, &msg.Category, &msg.CreatedAt); err != nil {
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

// AgentMessagePoolRepo handles agent message operations using the connection pool
type AgentMessagePoolRepo struct {
	clock clock.Clock
	pool *db.Pool
}

// NewAgentMessagePoolRepo creates a new agent message pool repository
func NewAgentMessagePoolRepo(pool *db.Pool, clk clock.Clock) *AgentMessagePoolRepo {
	return &AgentMessagePoolRepo{pool: pool, clock: clk}
}

// InsertBatch inserts multiple messages in a single transaction
func (r *AgentMessagePoolRepo) InsertBatch(sessionID string, seqStart int, messages []MessageEntry) error {
	if len(messages) == 0 {
		return nil
	}

	tx, err := r.pool.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO agent_messages (session_id, seq, content, category, created_at) VALUES (?, ?, ?, ?, ?)`)
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
		_, err := stmt.Exec(sessionID, seqStart+i, msg.Content, cat, now)
		if err != nil {
			return fmt.Errorf("failed to insert message %d: %w", seqStart+i, err)
		}
	}

	return tx.Commit()
}

// GetBySession returns all messages for a session ordered by seq
func (r *AgentMessagePoolRepo) GetBySession(sessionID string) ([]string, error) {
	rows, err := r.pool.Query(
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
func (r *AgentMessagePoolRepo) GetBySessionPaginated(sessionID string, limit, offset int) ([]MessageWithTime, error) {
	rows, err := r.pool.Query(
		`SELECT content, category, created_at FROM agent_messages WHERE session_id = ? ORDER BY seq ASC LIMIT ? OFFSET ?`,
		sessionID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []MessageWithTime
	for rows.Next() {
		var msg MessageWithTime
		if err := rows.Scan(&msg.Content, &msg.Category, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// GetBySessionPaginatedFiltered returns messages filtered by category
func (r *AgentMessagePoolRepo) GetBySessionPaginatedFiltered(sessionID, category string, limit, offset int) ([]MessageWithTime, error) {
	rows, err := r.pool.Query(
		`SELECT content, category, created_at FROM agent_messages WHERE session_id = ? AND category = ? ORDER BY seq ASC LIMIT ? OFFSET ?`,
		sessionID, category, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []MessageWithTime
	for rows.Next() {
		var msg MessageWithTime
		if err := rows.Scan(&msg.Content, &msg.Category, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// CountBySessionFiltered returns the message count for a session filtered by category
func (r *AgentMessagePoolRepo) CountBySessionFiltered(sessionID, category string) (int, error) {
	var count int
	err := r.pool.QueryRow(
		`SELECT COUNT(*) FROM agent_messages WHERE session_id = ? AND category = ?`,
		sessionID, category,
	).Scan(&count)
	return count, err
}

// CountBySession returns the total message count for a session
func (r *AgentMessagePoolRepo) CountBySession(sessionID string) (int, error) {
	var count int
	err := r.pool.QueryRow(
		`SELECT COUNT(*) FROM agent_messages WHERE session_id = ?`,
		sessionID,
	).Scan(&count)
	return count, err
}

// GetCountsBySessionIDs returns message counts for multiple sessions in one query
func (r *AgentMessagePoolRepo) GetCountsBySessionIDs(sessionIDs []string) (map[string]int, error) {
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

	rows, err := r.pool.Query(query, args...)
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
