package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"nrworkflow/internal/db"
	"nrworkflow/internal/model"
	"nrworkflow/internal/types"
)

// AgentService handles agent business logic
type AgentService struct {
	pool *db.Pool
}

// NewAgentService creates a new agent service
func NewAgentService(pool *db.Pool) *AgentService {
	return &AgentService{pool: pool}
}

// ListAgentTypes lists available agent types from workflow configs
func (s *AgentService) ListAgentTypes(projectRoot string) ([]string, error) {
	config, err := LoadMergedWorkflowConfig(projectRoot)
	if err != nil {
		return nil, err
	}

	// Collect unique agents
	agentMap := make(map[string]bool)
	for _, wf := range config.Workflows {
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

	// Get ticket
	var agentsStateStr string
	err := s.pool.QueryRow("SELECT COALESCE(agents_state, '') FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, ticketID).Scan(&agentsStateStr)
	if err != nil {
		return nil, fmt.Errorf("ticket not found: %s", ticketID)
	}

	if agentsStateStr == "" {
		return []ActiveAgent{}, nil
	}

	var allState map[string]interface{}
	if err := json.Unmarshal([]byte(agentsStateStr), &allState); err != nil {
		return []ActiveAgent{}, nil
	}

	stateRaw, ok := allState[req.Workflow]
	if !ok {
		return []ActiveAgent{}, nil
	}

	state, _ := stateRaw.(map[string]interface{})
	activeAgents, _ := state["active_agents"].(map[string]interface{})

	var result []ActiveAgent
	for key, agentRaw := range activeAgents {
		agent, _ := agentRaw.(map[string]interface{})

		elapsed := 0
		if startedAt, ok := agent["started_at"].(string); ok && startedAt != "" {
			if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
				elapsed = int(time.Since(t).Seconds())
			}
		}

		sessionID, _ := agent["session_id"].(string)
		startedAt, _ := agent["started_at"].(string)
		agentID, _ := agent["agent_id"].(string)
		agentType, _ := agent["agent_type"].(string)
		modelID, _ := agent["model_id"].(string)
		cli, _ := agent["cli"].(string)
		modelName, _ := agent["model"].(string)

		result = append(result, ActiveAgent{
			Key:        key,
			AgentID:    agentID,
			AgentType:  agentType,
			ModelID:    modelID,
			CLI:        cli,
			Model:      modelName,
			PID:        agent["pid"],
			SessionID:  sessionID,
			StartedAt:  startedAt,
			ElapsedSec: elapsed,
			Result:     agent["result"],
		})
	}

	return result, nil
}

// Kill kills active agents for a ticket
func (s *AgentService) Kill(projectID, ticketID string, req *types.AgentKillRequest) (int, error) {
	if req.Workflow == "" {
		return 0, fmt.Errorf("workflow is required")
	}

	// Get ticket
	var agentsStateStr string
	err := s.pool.QueryRow("SELECT COALESCE(agents_state, '') FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, ticketID).Scan(&agentsStateStr)
	if err != nil {
		return 0, fmt.Errorf("ticket not found: %s", ticketID)
	}

	if agentsStateStr == "" {
		return 0, fmt.Errorf("no active agents")
	}

	var allState map[string]interface{}
	if err := json.Unmarshal([]byte(agentsStateStr), &allState); err != nil {
		return 0, err
	}

	stateRaw, ok := allState[req.Workflow]
	if !ok {
		return 0, fmt.Errorf("workflow '%s' not found", req.Workflow)
	}

	state, _ := stateRaw.(map[string]interface{})
	activeAgents, _ := state["active_agents"].(map[string]interface{})

	if len(activeAgents) == 0 {
		return 0, fmt.Errorf("no active agents")
	}

	killed := 0
	for key, agentRaw := range activeAgents {
		agent, _ := agentRaw.(map[string]interface{})

		// Filter by model if specified
		if req.Model != "" {
			modelID, _ := agent["model_id"].(string)
			if !strings.Contains(key, req.Model) && modelID != req.Model {
				continue
			}
		}

		// Try to kill by PID
		if pidFloat, ok := agent["pid"].(float64); ok && pidFloat > 0 {
			pid := int(pidFloat)
			proc, err := os.FindProcess(pid)
			if err == nil {
				_ = proc.Signal(syscall.SIGTERM)
			}
		}

		killed++
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

func (s *AgentService) setAgentResult(projectID, ticketID, workflowName, agentType, result, modelID string) error {
	if workflowName == "" {
		return fmt.Errorf("workflow is required")
	}

	// Get ticket
	var agentsStateStr string
	err := s.pool.QueryRow("SELECT COALESCE(agents_state, '') FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, ticketID).Scan(&agentsStateStr)
	if err != nil {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}

	if agentsStateStr == "" {
		return fmt.Errorf("ticket %s not initialized", ticketID)
	}

	var allState map[string]interface{}
	if err := json.Unmarshal([]byte(agentsStateStr), &allState); err != nil {
		return fmt.Errorf("failed to parse state: %w", err)
	}

	stateRaw, ok := allState[workflowName]
	if !ok {
		return fmt.Errorf("workflow '%s' not found", workflowName)
	}

	state, _ := stateRaw.(map[string]interface{})
	activeAgents, _ := state["active_agents"].(map[string]interface{})

	// Find agent by type and optionally model
	for key, agentRaw := range activeAgents {
		agent, _ := agentRaw.(map[string]interface{})
		at, _ := agent["agent_type"].(string)

		if at != agentType {
			continue
		}

		if modelID != "" {
			mid, _ := agent["model_id"].(string)
			if !strings.Contains(key, modelID) && mid != modelID {
				continue
			}
		}

		agent["result"] = result
		activeAgents[key] = agent
		state["active_agents"] = activeAgents
		break
	}

	allState[workflowName] = state
	stateJSON, _ := json.Marshal(allState)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.pool.Exec(
		"UPDATE tickets SET agents_state = ?, updated_at = ? WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		string(stateJSON), now, projectID, ticketID)
	if err != nil {
		return err
	}

	return nil
}

// GetRecentSessions gets recent agent sessions
func (s *AgentService) GetRecentSessions(projectID string, limit int) ([]*model.AgentSession, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.pool.Query(`
		SELECT id, project_id, ticket_id, phase, workflow, agent_type, model_id, status, last_messages, message_stats, created_at, updated_at
		FROM agent_sessions
		WHERE LOWER(project_id) = LOWER(?)
		ORDER BY created_at DESC
		LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*model.AgentSession
	for rows.Next() {
		session := &model.AgentSession{}
		var createdAt, updatedAt string

		err := rows.Scan(
			&session.ID,
			&session.ProjectID,
			&session.TicketID,
			&session.Phase,
			&session.Workflow,
			&session.AgentType,
			&session.ModelID,
			&session.Status,
			&session.LastMessages,
			&session.MessageStats,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		session.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// GetTicketSessions gets agent sessions for a ticket
func (s *AgentService) GetTicketSessions(projectID, ticketID string) ([]*model.AgentSession, error) {
	rows, err := s.pool.Query(`
		SELECT id, project_id, ticket_id, phase, workflow, agent_type, model_id, status, last_messages, message_stats, created_at, updated_at
		FROM agent_sessions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?)
		ORDER BY created_at DESC`, projectID, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*model.AgentSession
	for rows.Next() {
		session := &model.AgentSession{}
		var createdAt, updatedAt string

		err := rows.Scan(
			&session.ID,
			&session.ProjectID,
			&session.TicketID,
			&session.Phase,
			&session.Workflow,
			&session.AgentType,
			&session.ModelID,
			&session.Status,
			&session.LastMessages,
			&session.MessageStats,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		session.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// CreateSession creates an agent session
func (s *AgentService) CreateSession(session *model.AgentSession) error {
	now := time.Now().UTC().Format(time.RFC3339)
	session.CreatedAt, _ = time.Parse(time.RFC3339, now)
	session.UpdatedAt = session.CreatedAt

	_, err := s.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, phase, workflow, agent_type, model_id, status, last_messages, message_stats, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.ProjectID,
		session.TicketID,
		session.Phase,
		session.Workflow,
		session.AgentType,
		session.ModelID,
		session.Status,
		session.LastMessages,
		session.MessageStats,
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

// UpdateSessionMessages updates an agent session's messages
func (s *AgentService) UpdateSessionMessages(sessionID string, messages string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.pool.Exec(
		"UPDATE agent_sessions SET last_messages = ?, updated_at = ? WHERE id = ?",
		sql.NullString{String: messages, Valid: messages != ""}, now, sessionID)
	return err
}

// UpdateSessionStats updates an agent session's stats
func (s *AgentService) UpdateSessionStats(sessionID string, stats string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.pool.Exec(
		"UPDATE agent_sessions SET message_stats = ?, updated_at = ? WHERE id = ?",
		sql.NullString{String: stats, Valid: stats != ""}, now, sessionID)
	return err
}
