package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nrworkflow/internal/db"
	"nrworkflow/internal/types"
)

// FindingsService handles findings business logic
type FindingsService struct {
	pool *db.Pool
}

// NewFindingsService creates a new findings service
func NewFindingsService(pool *db.Pool) *FindingsService {
	return &FindingsService{pool: pool}
}

// Add adds a finding for an agent
func (s *FindingsService) Add(projectID, ticketID string, req *types.FindingsAddRequest) error {
	if req.Workflow == "" {
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

	stateRaw, ok := allState[req.Workflow]
	if !ok {
		return fmt.Errorf("workflow '%s' not found", req.Workflow)
	}

	state, _ := stateRaw.(map[string]interface{})

	// Get or create findings map
	findings, _ := state["findings"].(map[string]interface{})
	if findings == nil {
		findings = make(map[string]interface{})
	}

	// Get or create agent findings
	var agentKey string
	if req.Model != "" {
		agentKey = req.AgentType + ":" + req.Model
	} else {
		agentKey = req.AgentType
	}

	agentFindings, _ := findings[agentKey].(map[string]interface{})
	if agentFindings == nil {
		// Try without model suffix for backwards compatibility
		agentFindings, _ = findings[req.AgentType].(map[string]interface{})
		if agentFindings == nil {
			agentFindings = make(map[string]interface{})
		}
	}

	// Try to parse value as JSON
	var parsedValue interface{}
	if err := json.Unmarshal([]byte(req.Value), &parsedValue); err != nil {
		parsedValue = req.Value
	}

	agentFindings[req.Key] = parsedValue
	findings[agentKey] = agentFindings
	state["findings"] = findings
	allState[req.Workflow] = state

	stateJSON, _ := json.Marshal(allState)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.pool.Exec(
		"UPDATE tickets SET agents_state = ?, updated_at = ? WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		string(stateJSON), now, projectID, ticketID)
	if err != nil {
		return fmt.Errorf("failed to update: %w", err)
	}

	return nil
}

// Get gets findings for an agent
func (s *FindingsService) Get(projectID, ticketID string, req *types.FindingsGetRequest) (interface{}, error) {
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
		return nil, fmt.Errorf("ticket %s not initialized", ticketID)
	}

	var allState map[string]interface{}
	if err := json.Unmarshal([]byte(agentsStateStr), &allState); err != nil {
		return nil, fmt.Errorf("failed to parse state: %w", err)
	}

	stateRaw, ok := allState[req.Workflow]
	if !ok {
		return nil, fmt.Errorf("workflow '%s' not found", req.Workflow)
	}

	state, _ := stateRaw.(map[string]interface{})
	findings, _ := state["findings"].(map[string]interface{})
	if findings == nil {
		return map[string]interface{}{}, nil
	}

	// If specific model is requested, return only that agent's findings
	if req.Model != "" {
		agentKey := req.AgentType + ":" + req.Model
		agentFindings, _ := findings[agentKey].(map[string]interface{})
		if agentFindings == nil {
			return map[string]interface{}{}, nil
		}
		if req.Key != "" {
			value, ok := agentFindings[req.Key]
			if !ok {
				return nil, fmt.Errorf("finding '%s' not found", req.Key)
			}
			return value, nil
		}
		return agentFindings, nil
	}

	// No model specified - collect ALL agents' findings for this agent type
	// This supports parallel phases where multiple agents run with different models
	allAgentFindings := make(map[string]interface{})
	prefix := req.AgentType + ":"

	// Collect all model-keyed findings (e.g., "setup-analyzer:claude:sonnet")
	for k, v := range findings {
		if strings.HasPrefix(k, prefix) {
			// Extract model suffix as the key (e.g., "claude:sonnet" from "setup-analyzer:claude:sonnet")
			modelKey := strings.TrimPrefix(k, prefix)
			allAgentFindings[modelKey] = v
		}
	}

	// Also check for findings stored without model suffix (single agent case)
	if singleFindings, ok := findings[req.AgentType].(map[string]interface{}); ok {
		if len(allAgentFindings) == 0 {
			// Only single agent findings exist - return them directly
			if req.Key != "" {
				value, ok := singleFindings[req.Key]
				if !ok {
					return nil, fmt.Errorf("finding '%s' not found", req.Key)
				}
				return value, nil
			}
			return singleFindings, nil
		}
		// Both single and model-keyed exist - include single under "default" key
		allAgentFindings["default"] = singleFindings
	}

	if len(allAgentFindings) == 0 {
		return map[string]interface{}{}, nil
	}

	// If only one agent found, return its findings directly for backward compatibility
	if len(allAgentFindings) == 1 {
		for _, v := range allAgentFindings {
			agentFindings, _ := v.(map[string]interface{})
			if req.Key != "" {
				value, ok := agentFindings[req.Key]
				if !ok {
					return nil, fmt.Errorf("finding '%s' not found", req.Key)
				}
				return value, nil
			}
			return agentFindings, nil
		}
	}

	// Multiple agents - return grouped by model
	// If a specific key is requested, return that key from each agent
	if req.Key != "" {
		keyFindings := make(map[string]interface{})
		for modelKey, v := range allAgentFindings {
			agentFindings, _ := v.(map[string]interface{})
			if agentFindings != nil {
				if value, ok := agentFindings[req.Key]; ok {
					keyFindings[modelKey] = value
				}
			}
		}
		if len(keyFindings) == 0 {
			return nil, fmt.Errorf("finding '%s' not found", req.Key)
		}
		return keyFindings, nil
	}

	return allAgentFindings, nil
}
