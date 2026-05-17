package service

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/types"
)

// FindingsService handles findings business logic
type FindingsService struct {
	clock       clock.Clock
	pool        *db.Pool
	findingRepo *repo.FindingRepo
}

// NewFindingsService creates a new findings service
func NewFindingsService(pool *db.Pool, clk clock.Clock) *FindingsService {
	return &FindingsService{
		pool:        pool,
		clock:       clk,
		findingRepo: repo.NewFindingRepo(pool, clk),
	}
}

// resolveWorkflowInstance returns the workflow instance ID.
func (s *FindingsService) resolveWorkflowInstance(instanceID string) (string, error) {
	if instanceID == "" {
		return "", fmt.Errorf("instance_id is required (NRF_WORKFLOW_INSTANCE_ID env var)")
	}
	return instanceID, nil
}

// loadSessionContext loads broadcast context and denorm fields for a session.
func (s *FindingsService) loadSessionContext(sessionID string) (BroadcastCtx, repo.Denorm, error) {
	if sessionID == "" {
		return BroadcastCtx{}, repo.Denorm{}, fmt.Errorf("session_id is required (NRF_SESSION_ID env var)")
	}
	var bctx BroadcastCtx
	var denorm repo.Denorm
	bctx.SessionID = sessionID
	var modelID sql.NullString
	err := s.pool.QueryRow(`
		SELECT s.project_id, s.ticket_id, wi.workflow_id, s.workflow_instance_id, s.agent_type, s.model_id
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		WHERE s.id = ?`, sessionID).Scan(
		&bctx.ProjectID, &bctx.TicketID, &bctx.Workflow, &denorm.WorkflowInstanceID, &bctx.AgentType, &modelID)
	if err == sql.ErrNoRows {
		return BroadcastCtx{}, repo.Denorm{}, fmt.Errorf("session %s not found", sessionID)
	}
	if err != nil {
		return BroadcastCtx{}, repo.Denorm{}, err
	}
	if modelID.Valid {
		bctx.ModelID = modelID.String
		denorm.ModelID = modelID.String
	}
	denorm.ProjectID = bctx.ProjectID
	denorm.AgentType = bctx.AgentType
	return bctx, denorm, nil
}

// normalizeJSONValue ensures the string is valid JSON; wraps as JSON string if not.
func normalizeJSONValue(v string) string {
	var parsed interface{}
	if err := json.Unmarshal([]byte(v), &parsed); err != nil {
		b, _ := json.Marshal(v)
		return string(b)
	}
	b, _ := json.Marshal(parsed)
	return string(b)
}

// rawToInterface converts map[string]json.RawMessage to map[string]interface{}.
func rawToInterface(raw map[string]json.RawMessage) map[string]interface{} {
	m := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		var parsed interface{}
		json.Unmarshal(v, &parsed) //nolint:errcheck
		m[k] = parsed
	}
	return m
}

// Add adds a finding to the current agent session
func (s *FindingsService) Add(req *types.FindingsAddRequest) (BroadcastCtx, error) {
	bctx, denorm, err := s.loadSessionContext(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}
	val := json.RawMessage(normalizeJSONValue(req.Value))
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	return bctx, s.findingRepo.Upsert("session", req.SessionID, req.Key, val, denorm, actor)
}

// AddBulk adds multiple findings to the current agent session in one operation
func (s *FindingsService) AddBulk(req *types.FindingsAddBulkRequest) (BroadcastCtx, error) {
	if len(req.KeyValues) == 0 {
		return BroadcastCtx{}, fmt.Errorf("at least one key-value pair is required")
	}
	bctx, denorm, err := s.loadSessionContext(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	for key, value := range req.KeyValues {
		val := json.RawMessage(normalizeJSONValue(value))
		if err := s.findingRepo.Upsert("session", req.SessionID, key, val, denorm, actor); err != nil {
			return BroadcastCtx{}, err
		}
	}
	return bctx, nil
}

// Get gets findings for an agent.
// If AgentType is omitted, reads the current session's own findings (requires SessionID).
// If AgentType is provided, reads cross-agent findings (requires InstanceID).
// If Layer is provided, reads findings keyed by agent_type for all agents in that layer.
func (s *FindingsService) Get(req *types.FindingsGetRequest) (interface{}, error) {
	keys := req.Keys
	if req.Key != "" && len(keys) == 0 {
		keys = []string{req.Key}
	}

	if req.Layer != nil {
		if req.AgentType != "" {
			return nil, fmt.Errorf("agent_type and layer are mutually exclusive")
		}
		return s.getByLayer(req.InstanceID, *req.Layer)
	}

	if req.AgentType == "" {
		if req.SessionID == "" {
			return nil, fmt.Errorf("session_id is required for own-session reads")
		}
		raw, err := s.findingRepo.GetOwn("session", req.SessionID)
		if err != nil {
			return map[string]interface{}{}, nil
		}
		return s.extractKeys(rawToInterface(raw), keys)
	}

	wfiID, err := s.resolveWorkflowInstance(req.InstanceID)
	if err != nil {
		return nil, err
	}

	if req.Model != "" {
		raw, err := s.findingRepo.GetByAgentModel(wfiID, req.AgentType, req.Model)
		if err != nil || len(raw) == 0 {
			return map[string]interface{}{}, nil
		}
		return s.extractKeys(rawToInterface(raw), keys)
	}

	byModel, err := s.findingRepo.GetByAgentAllModels(wfiID, req.AgentType)
	if err != nil || len(byModel) == 0 {
		return map[string]interface{}{}, nil
	}

	if len(byModel) == 1 {
		for _, m := range byModel {
			return s.extractKeys(rawToInterface(m), keys)
		}
	}

	if len(keys) > 0 {
		result := make(map[string]interface{})
		for modelKey, m := range byModel {
			extracted, err := s.extractKeys(rawToInterface(m), keys)
			if err == nil && extracted != nil {
				result[modelKey] = extracted
			}
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("finding key(s) not found")
		}
		return result, nil
	}

	result := make(map[string]interface{})
	for modelKey, m := range byModel {
		result[modelKey] = rawToInterface(m)
	}
	return result, nil
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
	bctx, denorm, err := s.loadSessionContext(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}
	val := json.RawMessage(normalizeJSONValue(req.Value))
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	return bctx, s.findingRepo.Append("session", req.SessionID, req.Key, val, denorm, actor)
}

// AppendBulk appends multiple values at once
func (s *FindingsService) AppendBulk(req *types.FindingsAppendBulkRequest) (BroadcastCtx, error) {
	if len(req.KeyValues) == 0 {
		return BroadcastCtx{}, fmt.Errorf("at least one key-value pair is required")
	}
	bctx, denorm, err := s.loadSessionContext(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	for key, value := range req.KeyValues {
		val := json.RawMessage(normalizeJSONValue(value))
		if err := s.findingRepo.Append("session", req.SessionID, key, val, denorm, actor); err != nil {
			return BroadcastCtx{}, err
		}
	}
	return bctx, nil
}

// Delete removes finding keys from the current agent session
func (s *FindingsService) Delete(req *types.FindingsDeleteRequest) (BroadcastCtx, int, error) {
	if len(req.Keys) == 0 {
		return BroadcastCtx{}, 0, fmt.Errorf("at least one key is required")
	}
	bctx, _, err := s.loadSessionContext(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, 0, nil // No session = nothing to delete
	}
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	deleted, err := s.findingRepo.DeleteKeys("session", req.SessionID, req.Keys, actor)
	if err != nil {
		return BroadcastCtx{}, 0, err
	}
	return bctx, len(deleted), nil
}

// getByLayer returns a map[agent_type]interface{} for all agent_definitions in the given layer.
func (s *FindingsService) getByLayer(instanceID string, layer int) (interface{}, error) {
	wfiID, err := s.resolveWorkflowInstance(instanceID)
	if err != nil {
		return nil, err
	}
	byAgent, err := s.findingRepo.GetByLayer(wfiID, layer)
	if err != nil {
		return nil, fmt.Errorf("layer findings query failed: %w", err)
	}
	result := make(map[string]interface{})
	for agentType, m := range byAgent {
		if m == nil {
			result[agentType] = nil
		} else {
			result[agentType] = rawToInterface(m)
		}
	}
	return result, nil
}
