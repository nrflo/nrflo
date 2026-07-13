package service

import (
	"fmt"
	"time"

	"be/internal/model"
	"be/internal/repo"
)

// CreateSession creates an agent session
func (s *AgentService) CreateSession(session *model.AgentSession) error {
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	session.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	session.UpdatedAt = session.CreatedAt

	// node_id identifies the execution slot; fall back to phase (what the 000157
	// backfill uses) so a caller that only sets Phase still gets a usable node id
	// rather than an empty one that no node-keyed lookup would match.
	nodeID := session.NodeID
	if nodeID == "" {
		nodeID = session.Phase
	}

	_, err := s.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
			model_id, status, result, result_reason, pid,
			context_left, ancestor_session_id, spawn_command, prompt, system_prompt,
			restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.ProjectID,
		session.TicketID,
		session.WorkflowInstanceID,
		session.Phase,
		nodeID,
		session.AgentType,
		session.ModelID,
		session.Status,
		session.Result,
		session.ResultReason,
		session.PID,
		session.ContextLeft,
		session.AncestorSessionID,
		session.SpawnCommand,
		session.Prompt,
		session.SystemPrompt,
		session.RestartCount,
		session.StartedAt,
		session.EndedAt,
		now,
		now,
	)
	return err
}

// UpdateSessionStatus updates an agent session status
func (s *AgentService) UpdateSessionStatus(sessionID string, status model.AgentSessionStatus) error {
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.pool.Exec(
		"UPDATE agent_sessions SET status = ?, updated_at = ? WHERE id = ?",
		status, now, sessionID)
	return err
}

// GetSessionByID gets a single agent session by its ID (globally unique PK)
func (s *AgentService) GetSessionByID(sessionID string) (*model.AgentSession, error) {
	row := s.pool.QueryRow(`
		SELECT s.id, s.project_id, s.ticket_id, s.workflow_instance_id, s.phase, s.agent_type,
			s.model_id, s.status, s.result, s.result_reason, s.pid,
			s.context_left, s.ancestor_session_id, s.spawn_command, s.prompt, s.system_prompt,
			s.restart_count, s.started_at, s.ended_at, s.created_at, s.updated_at, wi.workflow_id
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		WHERE s.id = ?`, sessionID)

	session, err := scanSessionJoined(row)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// Load full messages from agent_messages table
	messages, msgErr := s.msgRepo.GetBySession(sessionID)
	if msgErr == nil && len(messages) > 0 {
		session.Messages = messages
		session.MessageCount = len(messages)
	} else {
		count, _ := s.msgRepo.CountBySession(sessionID)
		session.MessageCount = count
	}

	return session, nil
}

// GetSessionMessages returns paginated messages with timestamps for a session.
// When category is non-empty, only messages of that category are returned.
func (s *AgentService) GetSessionMessages(sessionID string, limit, offset int, category string) ([]repo.MessageWithTime, int, error) {
	// Validate session exists
	var exists int
	err := s.pool.QueryRow("SELECT 1 FROM agent_sessions WHERE id = ?", sessionID).Scan(&exists)
	if err != nil {
		return nil, 0, fmt.Errorf("session not found: %s", sessionID)
	}

	var total int
	if category != "" {
		total, err = s.msgRepo.CountBySessionFiltered(sessionID, category)
	} else {
		total, err = s.msgRepo.CountBySession(sessionID)
	}
	if err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = -1 // SQLite: LIMIT -1 returns all rows
	}

	var messages []repo.MessageWithTime
	if category != "" {
		messages, err = s.msgRepo.GetBySessionPaginatedFiltered(sessionID, category, limit, offset)
	} else {
		messages, err = s.msgRepo.GetBySessionPaginated(sessionID, limit, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	if messages == nil {
		messages = []repo.MessageWithTime{}
	}

	return messages, total, nil
}

// loadMessageCounts batch-loads message counts for a slice of sessions
func (s *AgentService) loadMessageCounts(sessions []*model.AgentSession) {
	if len(sessions) == 0 {
		return
	}

	ids := make([]string, len(sessions))
	for i, sess := range sessions {
		ids[i] = sess.ID
	}

	counts, err := s.msgRepo.GetCountsBySessionIDs(ids)
	if err != nil {
		return
	}

	for _, sess := range sessions {
		if count, ok := counts[sess.ID]; ok {
			sess.MessageCount = count
		}
	}
}
