package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
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

// resolveWorkflowInstance finds the workflow instance ID for a ticket+workflow
func (s *FindingsService) resolveWorkflowInstance(projectID, ticketID, workflow string) (string, error) {
	var wfiID string
	err := s.pool.QueryRow(`
		SELECT id FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		projectID, ticketID, workflow).Scan(&wfiID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("workflow '%s' not found on %s", workflow, ticketID)
	}
	if err != nil {
		return "", err
	}
	return wfiID, nil
}

// findTargetSession finds the most recent session for agent_type+model in a workflow instance.
// Prefers running sessions over completed ones.
func (s *FindingsService) findTargetSession(wfiID, agentType, modelStr string) (string, sql.NullString, error) {
	query := `SELECT id, findings FROM agent_sessions
		WHERE workflow_instance_id = ? AND agent_type = ?`
	args := []interface{}{wfiID, agentType}

	if modelStr != "" {
		query += ` AND model_id = ?`
		args = append(args, modelStr)
	}

	query += ` ORDER BY CASE WHEN status = 'running' THEN 0 ELSE 1 END, created_at DESC LIMIT 1`

	var sessionID string
	var findings sql.NullString
	err := s.pool.QueryRow(query, args...).Scan(&sessionID, &findings)
	if err == sql.ErrNoRows {
		return "", sql.NullString{}, fmt.Errorf("no session found for agent %s", agentType)
	}
	return sessionID, findings, err
}

// updateSessionFindings writes the findings JSON to a session
func (s *FindingsService) updateSessionFindings(sessionID string, findings map[string]interface{}) error {
	data, _ := json.Marshal(findings)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.pool.Exec(
		`UPDATE agent_sessions SET findings = ?, updated_at = ? WHERE id = ?`,
		string(data), now, sessionID)
	return err
}

// parseFindings parses the findings JSON from a NullString
func parseFindings(ns sql.NullString) map[string]interface{} {
	if !ns.Valid || ns.String == "" {
		return make(map[string]interface{})
	}
	m := make(map[string]interface{})
	json.Unmarshal([]byte(ns.String), &m)
	return m
}

// Add adds a finding for an agent
func (s *FindingsService) Add(projectID, ticketID string, req *types.FindingsAddRequest) error {
	if req.Workflow == "" {
		return fmt.Errorf("workflow is required")
	}

	wfiID, err := s.resolveWorkflowInstance(projectID, ticketID, req.Workflow)
	if err != nil {
		return err
	}

	sessionID, findingsNS, err := s.findTargetSession(wfiID, req.AgentType, req.Model)
	if err != nil {
		return err
	}

	findings := parseFindings(findingsNS)

	var parsedValue interface{}
	if err := json.Unmarshal([]byte(req.Value), &parsedValue); err != nil {
		parsedValue = req.Value
	}

	findings[req.Key] = parsedValue
	return s.updateSessionFindings(sessionID, findings)
}

// AddBulk adds multiple findings for an agent in one operation
func (s *FindingsService) AddBulk(projectID, ticketID string, req *types.FindingsAddBulkRequest) error {
	if req.Workflow == "" {
		return fmt.Errorf("workflow is required")
	}
	if len(req.KeyValues) == 0 {
		return fmt.Errorf("at least one key-value pair is required")
	}

	wfiID, err := s.resolveWorkflowInstance(projectID, ticketID, req.Workflow)
	if err != nil {
		return err
	}

	sessionID, findingsNS, err := s.findTargetSession(wfiID, req.AgentType, req.Model)
	if err != nil {
		return err
	}

	findings := parseFindings(findingsNS)

	for key, value := range req.KeyValues {
		var parsedValue interface{}
		if err := json.Unmarshal([]byte(value), &parsedValue); err != nil {
			parsedValue = value
		}
		findings[key] = parsedValue
	}

	return s.updateSessionFindings(sessionID, findings)
}

// Get gets findings for an agent
func (s *FindingsService) Get(projectID, ticketID string, req *types.FindingsGetRequest) (interface{}, error) {
	if req.Workflow == "" {
		return nil, fmt.Errorf("workflow is required")
	}

	// Normalize: if single Key is set, add to Keys slice
	keys := req.Keys
	if req.Key != "" && len(keys) == 0 {
		keys = []string{req.Key}
	}

	wfiID, err := s.resolveWorkflowInstance(projectID, ticketID, req.Workflow)
	if err != nil {
		return nil, err
	}

	// If specific model requested, return that session's findings
	if req.Model != "" {
		_, findingsNS, err := s.findTargetSession(wfiID, req.AgentType, req.Model)
		if err != nil {
			return map[string]interface{}{}, nil
		}
		return s.extractKeys(parseFindings(findingsNS), keys)
	}

	// No model specified - collect findings from ALL sessions for this agent type
	rows, err := s.pool.Query(`
		SELECT model_id, findings FROM agent_sessions
		WHERE workflow_instance_id = ? AND agent_type = ? AND findings IS NOT NULL AND findings != ''
		ORDER BY created_at DESC`, wfiID, req.AgentType)
	if err != nil {
		return map[string]interface{}{}, nil
	}
	defer rows.Close()

	allAgentFindings := make(map[string]interface{})
	for rows.Next() {
		var modelID sql.NullString
		var findingsStr sql.NullString
		rows.Scan(&modelID, &findingsStr)

		if !findingsStr.Valid || findingsStr.String == "" {
			continue
		}

		var sessionFindings map[string]interface{}
		if json.Unmarshal([]byte(findingsStr.String), &sessionFindings) != nil {
			continue
		}

		if modelID.Valid && modelID.String != "" {
			allAgentFindings[modelID.String] = sessionFindings
		} else {
			allAgentFindings["default"] = sessionFindings
		}
	}

	if len(allAgentFindings) == 0 {
		return map[string]interface{}{}, nil
	}

	// If only one agent found, return its findings directly for backward compatibility
	if len(allAgentFindings) == 1 {
		for _, v := range allAgentFindings {
			agentFindings, _ := v.(map[string]interface{})
			return s.extractKeys(agentFindings, keys)
		}
	}

	// Multiple agents - return grouped by model
	if len(keys) > 0 {
		keyFindings := make(map[string]interface{})
		for modelKey, v := range allAgentFindings {
			agentFindings, _ := v.(map[string]interface{})
			if agentFindings != nil {
				extracted, err := s.extractKeys(agentFindings, keys)
				if err == nil && extracted != nil {
					keyFindings[modelKey] = extracted
				}
			}
		}
		if len(keyFindings) == 0 {
			return nil, fmt.Errorf("finding key(s) not found")
		}
		return keyFindings, nil
	}

	return allAgentFindings, nil
}

// extractKeys extracts specific keys from findings. If keys is empty, returns all findings.
func (s *FindingsService) extractKeys(findings map[string]interface{}, keys []string) (interface{}, error) {
	if len(keys) == 0 {
		return findings, nil
	}

	if len(keys) == 1 {
		value, ok := findings[keys[0]]
		if !ok {
			return nil, fmt.Errorf("finding '%s' not found", keys[0])
		}
		return value, nil
	}

	result := make(map[string]interface{})
	for _, key := range keys {
		if value, ok := findings[key]; ok {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("none of the requested keys found")
	}
	return result, nil
}

// Append appends a value to an existing finding (creating array if needed)
func (s *FindingsService) Append(projectID, ticketID string, req *types.FindingsAppendRequest) error {
	if req.Workflow == "" {
		return fmt.Errorf("workflow is required")
	}
	if req.Key == "" {
		return fmt.Errorf("key is required")
	}

	wfiID, err := s.resolveWorkflowInstance(projectID, ticketID, req.Workflow)
	if err != nil {
		return err
	}

	sessionID, findingsNS, err := s.findTargetSession(wfiID, req.AgentType, req.Model)
	if err != nil {
		return err
	}

	findings := parseFindings(findingsNS)

	var newValue interface{}
	if err := json.Unmarshal([]byte(req.Value), &newValue); err != nil {
		newValue = req.Value
	}

	findings[req.Key] = appendValue(findings[req.Key], newValue)
	return s.updateSessionFindings(sessionID, findings)
}

// AppendBulk appends multiple values at once
func (s *FindingsService) AppendBulk(projectID, ticketID string, req *types.FindingsAppendBulkRequest) error {
	if req.Workflow == "" {
		return fmt.Errorf("workflow is required")
	}
	if len(req.KeyValues) == 0 {
		return fmt.Errorf("at least one key-value pair is required")
	}

	wfiID, err := s.resolveWorkflowInstance(projectID, ticketID, req.Workflow)
	if err != nil {
		return err
	}

	sessionID, findingsNS, err := s.findTargetSession(wfiID, req.AgentType, req.Model)
	if err != nil {
		return err
	}

	findings := parseFindings(findingsNS)

	for key, value := range req.KeyValues {
		var newValue interface{}
		if err := json.Unmarshal([]byte(value), &newValue); err != nil {
			newValue = value
		}
		findings[key] = appendValue(findings[key], newValue)
	}

	return s.updateSessionFindings(sessionID, findings)
}

// Delete removes finding keys from an agent
func (s *FindingsService) Delete(projectID, ticketID string, req *types.FindingsDeleteRequest) (int, error) {
	if req.Workflow == "" {
		return 0, fmt.Errorf("workflow is required")
	}
	if len(req.Keys) == 0 {
		return 0, fmt.Errorf("at least one key is required")
	}

	wfiID, err := s.resolveWorkflowInstance(projectID, ticketID, req.Workflow)
	if err != nil {
		return 0, err
	}

	sessionID, findingsNS, err := s.findTargetSession(wfiID, req.AgentType, req.Model)
	if err != nil {
		return 0, nil // No session = nothing to delete
	}

	findings := parseFindings(findingsNS)

	deleted := 0
	for _, key := range req.Keys {
		if _, exists := findings[key]; exists {
			delete(findings, key)
			deleted++
		}
	}

	if deleted == 0 {
		return 0, nil
	}

	if err := s.updateSessionFindings(sessionID, findings); err != nil {
		return 0, err
	}
	return deleted, nil
}

// appendValue implements the append logic:
// - If existing is nil: return newValue as-is
// - If existing is array AND new is array: flatten (merge arrays)
// - If existing is array AND new is not array: append element
// - If existing is not array: convert to [existing, new] (or flatten if new is array)
func appendValue(existing, newValue interface{}) interface{} {
	if existing == nil {
		return newValue
	}

	existingArr, existingIsArr := existing.([]interface{})
	newArr, newIsArr := newValue.([]interface{})

	if existingIsArr {
		if newIsArr {
			return append(existingArr, newArr...)
		}
		return append(existingArr, newValue)
	}

	if newIsArr {
		result := []interface{}{existing}
		return append(result, newArr...)
	}

	return []interface{}{existing, newValue}
}

