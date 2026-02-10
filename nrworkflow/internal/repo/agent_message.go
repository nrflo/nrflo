package repo

import (
	"fmt"
	"strings"
	"time"

	"nrworkflow/internal/db"
)

// AgentMessageRepo handles agent message CRUD operations
type AgentMessageRepo struct {
	db *db.DB
}

// NewAgentMessageRepo creates a new agent message repository
func NewAgentMessageRepo(database *db.DB) *AgentMessageRepo {
	return &AgentMessageRepo{db: database}
}

// InsertBatch inserts multiple messages in a single transaction
func (r *AgentMessageRepo) InsertBatch(sessionID string, seqStart int, messages []string) error {
	if len(messages) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for i, msg := range messages {
		_, err := stmt.Exec(sessionID, seqStart+i, msg, now)
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

// GetBySessionPaginated returns messages with limit and offset
func (r *AgentMessageRepo) GetBySessionPaginated(sessionID string, limit, offset int) ([]string, error) {
	rows, err := r.db.Query(
		`SELECT content FROM agent_messages WHERE session_id = ? ORDER BY seq ASC LIMIT ? OFFSET ?`,
		sessionID, limit, offset,
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
	pool *db.Pool
}

// NewAgentMessagePoolRepo creates a new agent message pool repository
func NewAgentMessagePoolRepo(pool *db.Pool) *AgentMessagePoolRepo {
	return &AgentMessagePoolRepo{pool: pool}
}

// InsertBatch inserts multiple messages in a single transaction
func (r *AgentMessagePoolRepo) InsertBatch(sessionID string, seqStart int, messages []string) error {
	if len(messages) == 0 {
		return nil
	}

	tx, err := r.pool.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for i, msg := range messages {
		_, err := stmt.Exec(sessionID, seqStart+i, msg, now)
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

// GetBySessionPaginated returns messages with limit and offset
func (r *AgentMessagePoolRepo) GetBySessionPaginated(sessionID string, limit, offset int) ([]string, error) {
	rows, err := r.pool.Query(
		`SELECT content FROM agent_messages WHERE session_id = ? ORDER BY seq ASC LIMIT ? OFFSET ?`,
		sessionID, limit, offset,
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
