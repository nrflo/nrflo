package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// FindingsService handles findings business logic
type FindingsService struct {
	clock clock.Clock
	pool  *db.Pool
}

// NewFindingsService creates a new findings service
func NewFindingsService(pool *db.Pool, clk clock.Clock) *FindingsService {
	return &FindingsService{pool: pool, clock: clk}
}

// resolveWorkflowInstance returns the workflow instance ID.
// Requires instanceID from NRF_WORKFLOW_INSTANCE_ID env var (set by spawner).
func (s *FindingsService) resolveWorkflowInstance(instanceID string) (string, error) {
	if instanceID == "" {
		return "", fmt.Errorf("instance_id is required (NRF_WORKFLOW_INSTANCE_ID env var)")
	}
	return instanceID, nil
}

// loadSession loads broadcast context and current findings for a session in one query.
// Used by write operations — requires session_id (NRF_SESSION_ID env var).
func (s *FindingsService) loadSession(sessionID string) (BroadcastCtx, sql.NullString, error) {
	if sessionID == "" {
		return BroadcastCtx{}, sql.NullString{}, fmt.Errorf("session_id is required (NRF_SESSION_ID env var)")
	}
	var bctx BroadcastCtx
	var findings sql.NullString
	var modelID sql.NullString
	bctx.SessionID = sessionID
	err := s.pool.QueryRow(`
		SELECT s.findings, s.project_id, s.ticket_id, wi.workflow_id, s.agent_type, s.model_id
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		WHERE s.id = ?`, sessionID).Scan(
		&findings, &bctx.ProjectID, &bctx.TicketID, &bctx.Workflow, &bctx.AgentType, &modelID)
	if err == sql.ErrNoRows {
		return BroadcastCtx{}, sql.NullString{}, fmt.Errorf("session %s not found", sessionID)
	}
	if modelID.Valid {
		bctx.ModelID = modelID.String
	}
	return bctx, findings, err
}

// updateSessionFindings writes the findings JSON to a session
func (s *FindingsService) updateSessionFindings(sessionID string, findings map[string]interface{}) error {
	data, _ := json.Marshal(findings)
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
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

// Add adds a finding to the current agent session
func (s *FindingsService) Add(req *types.FindingsAddRequest) (BroadcastCtx, error) {
	bctx, findingsNS, err := s.loadSession(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}

	findings := parseFindings(findingsNS)

	var parsedValue interface{}
	if err := json.Unmarshal([]byte(req.Value), &parsedValue); err != nil {
		parsedValue = req.Value
	}

	findings[req.Key] = parsedValue
	return bctx, s.updateSessionFindings(req.SessionID, findings)
}

// AddBulk adds multiple findings to the current agent session in one operation
func (s *FindingsService) AddBulk(req *types.FindingsAddBulkRequest) (BroadcastCtx, error) {
	if len(req.KeyValues) == 0 {
		return BroadcastCtx{}, fmt.Errorf("at least one key-value pair is required")
	}

	bctx, findingsNS, err := s.loadSession(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}

	findings := parseFindings(findingsNS)

	for key, value := range req.KeyValues {
		var parsedValue interface{}
		if err := json.Unmarshal([]byte(value), &parsedValue); err != nil {
			parsedValue = value
		}
		findings[key] = parsedValue
	}

	return bctx, s.updateSessionFindings(req.SessionID, findings)
}

// Get gets findings for an agent.
// If AgentType is omitted, reads the current session's own findings (requires SessionID).
// If AgentType is provided, reads cross-agent findings (requires InstanceID).
// If Layer is provided, reads findings keyed by agent_type for all agents in that layer (mutually exclusive with AgentType).
func (s *FindingsService) Get(req *types.FindingsGetRequest) (interface{}, error) {
	// Normalize: if single Key is set, add to Keys slice
	keys := req.Keys
	if req.Key != "" && len(keys) == 0 {
		keys = []string{req.Key}
	}

	// Layer-keyed read — returns map[agent_type]findings for all agents in the layer
	if req.Layer != nil {
		if req.AgentType != "" {
			return nil, fmt.Errorf("agent_type and layer are mutually exclusive")
		}
		return s.getByLayer(req.InstanceID, *req.Layer)
	}

	// Own-session read (no agent_type provided)
	if req.AgentType == "" {
		if req.SessionID == "" {
			return nil, fmt.Errorf("session_id is required for own-session reads")
		}
		var findingsNS sql.NullString
		err := s.pool.QueryRow(`SELECT findings FROM agent_sessions WHERE id = ?`, req.SessionID).Scan(&findingsNS)
		if err != nil {
			return map[string]interface{}{}, nil
		}
		return s.extractKeys(parseFindings(findingsNS), keys)
	}

	// Cross-agent read — requires instance_id
	wfiID, err := s.resolveWorkflowInstance(req.InstanceID)
	if err != nil {
		return nil, err
	}

	// If specific model requested, return that session's findings
	if req.Model != "" {
		var sid string
		var findingsNS sql.NullString
		err := s.pool.QueryRow(`
			SELECT id, findings FROM agent_sessions
			WHERE workflow_instance_id = ? AND agent_type = ? AND model_id = ? AND status != 'callback'
			ORDER BY CASE WHEN status = 'running' THEN 0 ELSE 1 END, created_at DESC LIMIT 1`,
			wfiID, req.AgentType, req.Model).Scan(&sid, &findingsNS)
		if err != nil {
			return map[string]interface{}{}, nil
		}
		return s.extractKeys(parseFindings(findingsNS), keys)
	}

	// No model specified — collect findings from ALL sessions for this agent type
	rows, err := s.pool.Query(`
		SELECT model_id, findings FROM agent_sessions
		WHERE workflow_instance_id = ? AND agent_type = ? AND status != 'callback'
		  AND findings IS NOT NULL AND findings != ''
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

	// Multiple agents — return grouped by model
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
func (s *FindingsService) Append(req *types.FindingsAppendRequest) (BroadcastCtx, error) {
	if req.Key == "" {
		return BroadcastCtx{}, fmt.Errorf("key is required")
	}

	bctx, findingsNS, err := s.loadSession(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}

	findings := parseFindings(findingsNS)

	var newValue interface{}
	if err := json.Unmarshal([]byte(req.Value), &newValue); err != nil {
		newValue = req.Value
	}

	findings[req.Key] = AppendValue(findings[req.Key], newValue)
	return bctx, s.updateSessionFindings(req.SessionID, findings)
}

// AppendBulk appends multiple values at once
func (s *FindingsService) AppendBulk(req *types.FindingsAppendBulkRequest) (BroadcastCtx, error) {
	if len(req.KeyValues) == 0 {
		return BroadcastCtx{}, fmt.Errorf("at least one key-value pair is required")
	}

	bctx, findingsNS, err := s.loadSession(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}

	findings := parseFindings(findingsNS)

	for key, value := range req.KeyValues {
		var newValue interface{}
		if err := json.Unmarshal([]byte(value), &newValue); err != nil {
			newValue = value
		}
		findings[key] = AppendValue(findings[key], newValue)
	}

	return bctx, s.updateSessionFindings(req.SessionID, findings)
}

// Delete removes finding keys from the current agent session
func (s *FindingsService) Delete(req *types.FindingsDeleteRequest) (BroadcastCtx, int, error) {
	if len(req.Keys) == 0 {
		return BroadcastCtx{}, 0, fmt.Errorf("at least one key is required")
	}

	bctx, findingsNS, err := s.loadSession(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, 0, nil // No session = nothing to delete
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
		return bctx, 0, nil
	}

	if err := s.updateSessionFindings(req.SessionID, findings); err != nil {
		return BroadcastCtx{}, 0, err
	}
	return bctx, deleted, nil
}

// getByLayer returns a flat map[agent_type]interface{} for all agent_definitions in the
// given layer. Values are parsed findings from the latest terminal session, or nil when
// no terminal session exists for that agent yet.
func (s *FindingsService) getByLayer(instanceID string, layer int) (interface{}, error) {
	wfiID, err := s.resolveWorkflowInstance(instanceID)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(`
		WITH wfi AS (
			SELECT id, project_id, workflow_id FROM workflow_instances WHERE id = ?
		),
		latest AS (
			SELECT s.agent_type, s.findings,
				ROW_NUMBER() OVER (PARTITION BY s.agent_type ORDER BY s.created_at DESC) AS rn
			FROM agent_sessions s, wfi
			WHERE s.workflow_instance_id = wfi.id
			  AND s.status != 'callback'
			  AND s.findings IS NOT NULL
			  AND s.findings != ''
		)
		SELECT ad.id, latest.findings
		FROM agent_definitions ad, wfi
		LEFT JOIN latest ON latest.agent_type = ad.id AND latest.rn = 1
		WHERE ad.project_id = wfi.project_id
		  AND ad.workflow_id = wfi.workflow_id
		  AND ad.layer = ?
		ORDER BY ad.id`,
		wfiID, layer)
	if err != nil {
		return nil, fmt.Errorf("layer findings query failed: %w", err)
	}
	defer rows.Close()

	result := make(map[string]interface{})
	for rows.Next() {
		var agentType string
		var findingsStr sql.NullString
		if err := rows.Scan(&agentType, &findingsStr); err != nil {
			continue
		}
		if !findingsStr.Valid || findingsStr.String == "" {
			result[agentType] = nil
			continue
		}
		var m map[string]interface{}
		if json.Unmarshal([]byte(findingsStr.String), &m) != nil {
			result[agentType] = nil
			continue
		}
		result[agentType] = m
	}
	return result, nil
}

// appendValue implements the append logic:
// - If existing is nil: return newValue as-is
// - If existing is array AND new is array: flatten (merge arrays)
// - If existing is array AND new is not array: append element
// - If existing is not array: convert to [existing, new] (or flatten if new is array)
func AppendValue(existing, newValue interface{}) interface{} {
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
