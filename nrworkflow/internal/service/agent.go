package service

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"nrworkflow/internal/db"
	"nrworkflow/internal/model"
	"nrworkflow/internal/repo"
	"nrworkflow/internal/types"
)

// AgentService handles agent business logic
type AgentService struct {
	pool        *db.Pool
	workflowSvc *WorkflowService
	msgRepo     *repo.AgentMessagePoolRepo
}

// NewAgentService creates a new agent service
func NewAgentService(pool *db.Pool) *AgentService {
	return &AgentService{
		pool:        pool,
		workflowSvc: NewWorkflowService(pool),
		msgRepo:     repo.NewAgentMessagePoolRepo(pool),
	}
}

// scanSessionJoined scans an agent session from a row that JOINs with workflow_instances
func scanSessionJoined(scanner interface{ Scan(...interface{}) error }) (*model.AgentSession, error) {
	s := &model.AgentSession{}
	var createdAt, updatedAt string
	err := scanner.Scan(
		&s.ID, &s.ProjectID, &s.TicketID, &s.WorkflowInstanceID, &s.Phase, &s.AgentType,
		&s.ModelID, &s.Status, &s.Result, &s.ResultReason, &s.PID, &s.Findings,
		&s.ContextLeft, &s.AncestorSessionID, &s.SpawnCommand, &s.PromptContext,
		&s.RawOutput, &s.StartedAt, &s.EndedAt, &createdAt, &updatedAt, &s.Workflow,
	)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return s, nil
}

// ListAgentTypes lists available agent types from workflow definitions in DB
func (s *AgentService) ListAgentTypes(projectID string) ([]string, error) {
	workflows, err := s.workflowSvc.ListWorkflowDefs(projectID)
	if err != nil {
		return nil, err
	}

	// Collect unique agents
	agentMap := make(map[string]bool)
	for _, wf := range workflows {
		for _, p := range wf.Phases {
			agentMap[p.Agent] = true
		}
	}

	var agents []string
	for agent := range agentMap {
		agents = append(agents, agent)
	}

	return agents, nil
}

// ActiveAgent represents an active agent
type ActiveAgent struct {
	Key        string      `json:"key"`
	AgentID    string      `json:"agent_id"`
	AgentType  string      `json:"agent_type"`
	ModelID    string      `json:"model_id"`
	CLI        string      `json:"cli"`
	Model      string      `json:"model"`
	PID        interface{} `json:"pid"`
	SessionID  string      `json:"session_id"`
	StartedAt  string      `json:"started_at"`
	ElapsedSec int         `json:"elapsed_sec"`
	Result     interface{} `json:"result"`
}

// GetActive gets active agents for a ticket
func (s *AgentService) GetActive(projectID, ticketID string, req *types.AgentActiveRequest) ([]ActiveAgent, error) {
	if req.Workflow == "" {
		return nil, fmt.Errorf("workflow is required")
	}

	// Find workflow instance
	var wfiID string
	err := s.pool.QueryRow(`
		SELECT id FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		projectID, ticketID, req.Workflow).Scan(&wfiID)
	if err != nil {
		return []ActiveAgent{}, nil
	}

	rows, err := s.pool.Query(`
		SELECT id, agent_type, model_id, pid, result, started_at
		FROM agent_sessions
		WHERE workflow_instance_id = ? AND status = 'running'`, wfiID)
	if err != nil {
		return []ActiveAgent{}, nil
	}
	defer rows.Close()

	var result []ActiveAgent
	for rows.Next() {
		var id, agentType string
		var modelID, agentResult, startedAt sql.NullString
		var pid sql.NullInt64
		rows.Scan(&id, &agentType, &modelID, &pid, &agentResult, &startedAt)

		key := agentType
		cli := ""
		modelName := ""
		modelIDStr := ""
		if modelID.Valid && modelID.String != "" {
			modelIDStr = modelID.String
			key = agentType + ":" + modelIDStr
			parts := strings.SplitN(modelIDStr, ":", 2)
			if len(parts) == 2 {
				cli = parts[0]
				modelName = parts[1]
			}
		}

		elapsed := 0
		if startedAt.Valid && startedAt.String != "" {
			if t, err := time.Parse(time.RFC3339, startedAt.String); err == nil {
				elapsed = int(time.Since(t).Seconds())
			}
		}

		var pidVal interface{}
		if pid.Valid {
			pidVal = pid.Int64
		}
		var resultVal interface{}
		if agentResult.Valid {
			resultVal = agentResult.String
		}

		result = append(result, ActiveAgent{
			Key:        key,
			AgentID:    id,
			AgentType:  agentType,
			ModelID:    modelIDStr,
			CLI:        cli,
			Model:      modelName,
			PID:        pidVal,
			SessionID:  id,
			StartedAt:  startedAt.String,
			ElapsedSec: elapsed,
			Result:     resultVal,
		})
	}

	return result, nil
}

// Kill kills active agents for a ticket
func (s *AgentService) Kill(projectID, ticketID string, req *types.AgentKillRequest) (int, error) {
	if req.Workflow == "" {
		return 0, fmt.Errorf("workflow is required")
	}

	var wfiID string
	err := s.pool.QueryRow(`
		SELECT id FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		projectID, ticketID, req.Workflow).Scan(&wfiID)
	if err != nil {
		return 0, fmt.Errorf("workflow '%s' not found", req.Workflow)
	}

	rows, err := s.pool.Query(`
		SELECT id, agent_type, model_id, pid
		FROM agent_sessions
		WHERE workflow_instance_id = ? AND status = 'running'`, wfiID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	killed := 0
	for rows.Next() {
		var id, agentType string
		var modelID sql.NullString
		var pid sql.NullInt64
		rows.Scan(&id, &agentType, &modelID, &pid)

		// Filter by model if specified
		if req.Model != "" {
			key := agentType + ":" + modelID.String
			if !strings.Contains(key, req.Model) && (!modelID.Valid || modelID.String != req.Model) {
				continue
			}
		}

		if pid.Valid && pid.Int64 > 0 {
			proc, err := os.FindProcess(int(pid.Int64))
			if err == nil {
				_ = proc.Signal(syscall.SIGTERM)
			}
		}
		killed++
	}

	if killed == 0 {
		return 0, fmt.Errorf("no active agents")
	}

	return killed, nil
}

// Complete marks an agent as completed
func (s *AgentService) Complete(projectID, ticketID string, req *types.AgentCompleteRequest) error {
	return s.setAgentResult(projectID, ticketID, req.Workflow, req.AgentType, "pass", req.Model)
}

// Fail marks an agent as failed
func (s *AgentService) Fail(projectID, ticketID string, req *types.AgentCompleteRequest) error {
	return s.setAgentResult(projectID, ticketID, req.Workflow, req.AgentType, "fail", req.Model)
}

// Continue marks an agent as needing context continuation
func (s *AgentService) Continue(projectID, ticketID string, req *types.AgentCompleteRequest) error {
	return s.setAgentResult(projectID, ticketID, req.Workflow, req.AgentType, "continue", req.Model)
}

func (s *AgentService) setAgentResult(projectID, ticketID, workflowName, agentType, result, modelID string) error {
	if workflowName == "" {
		return fmt.Errorf("workflow is required")
	}

	var wfiID string
	err := s.pool.QueryRow(`
		SELECT id FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		projectID, ticketID, workflowName).Scan(&wfiID)
	if err != nil {
		return fmt.Errorf("workflow '%s' not found", workflowName)
	}

	// Find matching active session
	query := `SELECT id FROM agent_sessions
		WHERE workflow_instance_id = ? AND agent_type = ? AND status = 'running'`
	args := []interface{}{wfiID, agentType}

	if modelID != "" {
		query += ` AND model_id = ?`
		args = append(args, modelID)
	}
	query += ` LIMIT 1`

	var sessionID string
	err = s.pool.QueryRow(query, args...).Scan(&sessionID)
	if err != nil {
		return fmt.Errorf("no active agent found for %s", agentType)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.pool.Exec(
		`UPDATE agent_sessions SET result = ?, updated_at = ? WHERE id = ?`,
		result, now, sessionID)
	return err
}

// GetRecentSessions gets recent agent sessions
func (s *AgentService) GetRecentSessions(projectID string, limit int) ([]*model.AgentSession, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.pool.Query(`
		SELECT s.id, s.project_id, s.ticket_id, s.workflow_instance_id, s.phase, s.agent_type,
			s.model_id, s.status, s.result, s.result_reason, s.pid, s.findings,
			s.context_left, s.ancestor_session_id, s.spawn_command, s.prompt_context,
			s.raw_output, s.started_at, s.ended_at, s.created_at, s.updated_at, wi.workflow_id
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		WHERE LOWER(s.project_id) = LOWER(?)
		ORDER BY s.created_at DESC
		LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*model.AgentSession
	for rows.Next() {
		session, err := scanSessionJoined(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	s.loadMessageCounts(sessions)
	return sessions, nil
}

// GetTicketSessions gets agent sessions for a ticket, optionally filtered by workflow
func (s *AgentService) GetTicketSessions(projectID, ticketID, workflow string) ([]*model.AgentSession, error) {
	query := `
		SELECT s.id, s.project_id, s.ticket_id, s.workflow_instance_id, s.phase, s.agent_type,
			s.model_id, s.status, s.result, s.result_reason, s.pid, s.findings,
			s.context_left, s.ancestor_session_id, s.spawn_command, s.prompt_context,
			s.raw_output, s.started_at, s.ended_at, s.created_at, s.updated_at, wi.workflow_id
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		WHERE LOWER(s.project_id) = LOWER(?) AND LOWER(s.ticket_id) = LOWER(?)`
	args := []interface{}{projectID, ticketID}

	if workflow != "" {
		query += ` AND LOWER(wi.workflow_id) = LOWER(?)`
		args = append(args, workflow)
	}
	query += ` ORDER BY s.created_at DESC`

	rows, err := s.pool.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*model.AgentSession
	for rows.Next() {
		session, err := scanSessionJoined(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	s.loadMessageCounts(sessions)
	return sessions, nil
}

// CreateSession creates an agent session
func (s *AgentService) CreateSession(session *model.AgentSession) error {
	now := time.Now().UTC().Format(time.RFC3339)
	session.CreatedAt, _ = time.Parse(time.RFC3339, now)
	session.UpdatedAt = session.CreatedAt

	_, err := s.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			model_id, status, result, result_reason, pid, findings,
			context_left, ancestor_session_id, spawn_command, prompt_context,
			raw_output, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.ProjectID,
		session.TicketID,
		session.WorkflowInstanceID,
		session.Phase,
		session.AgentType,
		session.ModelID,
		session.Status,
		session.Result,
		session.ResultReason,
		session.PID,
		session.Findings,
		session.ContextLeft,
		session.AncestorSessionID,
		session.SpawnCommand,
		session.PromptContext,
		session.RawOutput,
		session.StartedAt,
		session.EndedAt,
		now,
		now,
	)
	return err
}

// UpdateSessionStatus updates an agent session status
func (s *AgentService) UpdateSessionStatus(sessionID string, status model.AgentSessionStatus) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.pool.Exec(
		"UPDATE agent_sessions SET status = ?, updated_at = ? WHERE id = ?",
		status, now, sessionID)
	return err
}

// GetSessionByID gets a single agent session by its ID (globally unique PK)
func (s *AgentService) GetSessionByID(sessionID string) (*model.AgentSession, error) {
	row := s.pool.QueryRow(`
		SELECT s.id, s.project_id, s.ticket_id, s.workflow_instance_id, s.phase, s.agent_type,
			s.model_id, s.status, s.result, s.result_reason, s.pid, s.findings,
			s.context_left, s.ancestor_session_id, s.spawn_command, s.prompt_context,
			s.raw_output, s.started_at, s.ended_at, s.created_at, s.updated_at, wi.workflow_id
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

// GetSessionMessages returns paginated messages with timestamps for a session
func (s *AgentService) GetSessionMessages(sessionID string, limit, offset int) ([]repo.MessageWithTime, int, error) {
	// Validate session exists
	var exists int
	err := s.pool.QueryRow("SELECT 1 FROM agent_sessions WHERE id = ?", sessionID).Scan(&exists)
	if err != nil {
		return nil, 0, fmt.Errorf("session not found: %s", sessionID)
	}

	total, err := s.msgRepo.CountBySession(sessionID)
	if err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 100
	}

	messages, err := s.msgRepo.GetBySessionPaginated(sessionID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	if messages == nil {
		messages = []repo.MessageWithTime{}
	}

	return messages, total, nil
}

// GetSessionRawOutput retrieves raw stdout/stderr output for a session
func (s *AgentService) GetSessionRawOutput(sessionID string) (string, error) {
	var exists int
	err := s.pool.QueryRow("SELECT 1 FROM agent_sessions WHERE id = ?", sessionID).Scan(&exists)
	if err != nil {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}

	var rawOutput sql.NullString
	err = s.pool.QueryRow("SELECT raw_output FROM agent_sessions WHERE id = ?", sessionID).Scan(&rawOutput)
	if err != nil {
		return "", err
	}
	if !rawOutput.Valid {
		return "", nil
	}
	return rawOutput.String, nil
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

