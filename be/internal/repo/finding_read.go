package repo

import (
	"database/sql"
	"encoding/json"
)

// GetOwn returns all findings for a scope/scope_id pair.
func (r *FindingRepo) GetOwn(scope, scopeID string) (map[string]json.RawMessage, error) {
	rows, err := r.db.Query(
		`SELECT key, value FROM findings WHERE scope=? AND scope_id=?`,
		scope, scopeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]json.RawMessage)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = json.RawMessage(v)
	}
	return result, rows.Err()
}

// GetByAgentModel returns findings for a specific agent_type + model_id combination
// within a workflow instance (scope=session rows).
func (r *FindingRepo) GetByAgentModel(wfiID, agentType, modelID string) (map[string]json.RawMessage, error) {
	rows, err := r.db.Query(
		`SELECT key, value FROM findings
		 WHERE scope='session' AND workflow_instance_id=? AND agent_type=? AND model_id=?`,
		wfiID, agentType, modelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]json.RawMessage)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = json.RawMessage(v)
	}
	return result, rows.Err()
}

// GetByAgentAllModels returns findings for all models of an agent_type, grouped by model key.
// The map key is the model_id, or "default" when model_id is absent.
func (r *FindingRepo) GetByAgentAllModels(wfiID, agentType string) (map[string]map[string]json.RawMessage, error) {
	rows, err := r.db.Query(
		`SELECT model_id, key, value FROM findings
		 WHERE scope='session' AND workflow_instance_id=? AND agent_type=?`,
		wfiID, agentType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]map[string]json.RawMessage)
	for rows.Next() {
		var modelID sql.NullString
		var k, v string
		if err := rows.Scan(&modelID, &k, &v); err != nil {
			return nil, err
		}
		modelKey := "default"
		if modelID.Valid && modelID.String != "" {
			modelKey = modelID.String
		}
		if result[modelKey] == nil {
			result[modelKey] = make(map[string]json.RawMessage)
		}
		result[modelKey][k] = json.RawMessage(v)
	}
	return result, rows.Err()
}

// GetByLayer returns findings for all agent_definitions at a layer, keyed by agent_type.
// Agents with no findings have a nil inner map.
func (r *FindingRepo) GetByLayer(wfiID string, layer int) (map[string]map[string]json.RawMessage, error) {
	rows, err := r.db.Query(`
		WITH wfi AS (SELECT project_id, workflow_id FROM workflow_instances WHERE id = ?)
		SELECT ad.id, f.key, f.value
		FROM agent_definitions ad, wfi
		LEFT JOIN findings f
		       ON f.scope = 'session'
		      AND f.workflow_instance_id = ?
		      AND f.agent_type = ad.id
		WHERE ad.project_id = wfi.project_id
		  AND ad.workflow_id = wfi.workflow_id
		  AND ad.layer = ?
		ORDER BY ad.id`,
		wfiID, wfiID, layer,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]map[string]json.RawMessage)
	for rows.Next() {
		var agentType string
		var k, v sql.NullString
		if err := rows.Scan(&agentType, &k, &v); err != nil {
			return nil, err
		}
		if !k.Valid {
			if _, exists := result[agentType]; !exists {
				result[agentType] = nil
			}
			continue
		}
		if result[agentType] == nil {
			result[agentType] = make(map[string]json.RawMessage)
		}
		result[agentType][k.String] = json.RawMessage(v.String)
	}
	return result, rows.Err()
}

// ListByWorkflowInstance returns session findings grouped by "agent_type:model_id" key.
// When model_id is empty the key is just "agent_type". Excludes system agents.
func (r *FindingRepo) ListByWorkflowInstance(wfiID string) (map[string]map[string]json.RawMessage, error) {
	rows, err := r.db.Query(`
		SELECT agent_type, model_id, key, value
		FROM findings
		WHERE scope = 'session' AND workflow_instance_id = ?
		  AND agent_type NOT IN ('context-saver', 'conflict-resolver')`,
		wfiID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]map[string]json.RawMessage)
	for rows.Next() {
		var agentType string
		var modelID sql.NullString
		var k, v string
		if err := rows.Scan(&agentType, &modelID, &k, &v); err != nil {
			return nil, err
		}
		mapKey := agentType
		if modelID.Valid && modelID.String != "" {
			mapKey = agentType + ":" + modelID.String
		}
		if result[mapKey] == nil {
			result[mapKey] = make(map[string]json.RawMessage)
		}
		result[mapKey][k] = json.RawMessage(v)
	}
	return result, rows.Err()
}

// GetSessionFindingByKey returns the value of a specific key from any session finding
// in the workflow instance, prioritizing completed sessions over running ones.
func (r *FindingRepo) GetSessionFindingByKey(wfiID, key string) (json.RawMessage, bool) {
	var value string
	err := r.db.QueryRow(`
		SELECT f.value FROM findings f
		JOIN agent_sessions s ON s.id = f.scope_id
		WHERE f.scope = 'session' AND f.workflow_instance_id = ? AND f.key = ?
		ORDER BY s.ended_at IS NULL, s.ended_at DESC
		LIMIT 1`,
		wfiID, key,
	).Scan(&value)
	if err != nil {
		return nil, false
	}
	return json.RawMessage(value), true
}

// findingAppendValue implements array-merge semantics (mirrors service.AppendValue).
func findingAppendValue(existing, newVal interface{}) interface{} {
	if existing == nil {
		return newVal
	}
	existArr, existIsArr := existing.([]interface{})
	newArr, newIsArr := newVal.([]interface{})
	if existIsArr {
		if newIsArr {
			return append(existArr, newArr...)
		}
		return append(existArr, newVal)
	}
	if newIsArr {
		return append([]interface{}{existing}, newArr...)
	}
	return []interface{}{existing, newVal}
}
