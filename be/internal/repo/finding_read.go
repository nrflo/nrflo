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
// Rows are ordered so the most-recently-ended session's value wins on key
// collision, collapsing multiple nodes/retries sharing one agent_type+model
// deterministically instead of by arbitrary map iteration.
func (r *FindingRepo) GetByAgentAllModels(wfiID, agentType string) (map[string]map[string]json.RawMessage, error) {
	rows, err := r.db.Query(
		`SELECT f.model_id, f.key, f.value
		 FROM findings f
		 JOIN agent_sessions s ON s.id = f.scope_id
		 WHERE f.scope='session' AND f.workflow_instance_id=? AND f.agent_type=?
		 ORDER BY (s.ended_at IS NULL) DESC, s.ended_at ASC`,
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

// GetByLayer returns findings for all executable nodes at a layer, keyed by
// node_id (== agent def id for static workflows; N distinct keys for a
// fan-out layer, one per sibling). Roster membership is the executable defs
// at the layer (consultant=0, node_role='static', matching ListExecutable) —
// a def with no session yet still renders with a nil map so the roster shows
// it as pending. A def's sessions are attributed to it via agent_type, then
// bucketed by their own node_id (falling back to phase for legacy rows).
// On key collision within a node (retries), the most-recently-ended
// session's value wins.
func (r *FindingRepo) GetByLayer(wfiID string, layer int) (map[string]map[string]json.RawMessage, error) {
	rows, err := r.db.Query(`
		WITH wfi AS (SELECT project_id, workflow_id FROM workflow_instances WHERE id = ?)
		SELECT node_id, key, value FROM (
			SELECT ad.id AS node_id, NULL AS key, NULL AS value, 0 AS running, NULL AS ended_at
			FROM agent_definitions ad, wfi
			WHERE ad.project_id = wfi.project_id
			  AND ad.workflow_id = wfi.workflow_id
			  AND ad.layer = ?
			  AND ad.consultant = 0
			  AND ad.node_role = 'static'
			UNION ALL
			SELECT COALESCE(NULLIF(s.node_id, ''), s.phase) AS node_id, f.key, f.value,
			       CASE WHEN s.ended_at IS NULL THEN 1 ELSE 0 END AS running, s.ended_at
			FROM agent_sessions s
			JOIN agent_definitions ad2, wfi
			  ON ad2.project_id = wfi.project_id
			 AND ad2.workflow_id = wfi.workflow_id
			 AND ad2.id = s.agent_type
			LEFT JOIN findings f
			       ON f.scope = 'session'
			      AND f.scope_id = s.id
			WHERE s.workflow_instance_id = ?
			  AND ad2.layer = ?
			  AND ad2.consultant = 0
			  AND ad2.node_role = 'static'
		)
		ORDER BY node_id, running DESC, ended_at ASC`,
		wfiID, layer, wfiID, layer,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]map[string]json.RawMessage)
	for rows.Next() {
		var nodeID string
		var k, v sql.NullString
		if err := rows.Scan(&nodeID, &k, &v); err != nil {
			return nil, err
		}
		if !k.Valid {
			if _, exists := result[nodeID]; !exists {
				result[nodeID] = nil
			}
			continue
		}
		if result[nodeID] == nil {
			result[nodeID] = make(map[string]json.RawMessage)
		}
		result[nodeID][k.String] = json.RawMessage(v.String)
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

// GetOwnKeyByActor returns the value of a single key on a scope/scope_id
// pair, but only when the row was last written by actorID — used to accept a
// genuine write while rejecting a copy carried forward under a different
// actor (e.g. continuation carry-forward).
func (r *FindingRepo) GetOwnKeyByActor(scope, scopeID, key, actorID string) (json.RawMessage, bool) {
	var value string
	err := r.db.QueryRow(
		`SELECT value FROM findings WHERE scope=? AND scope_id=? AND key=? AND updated_by=?`,
		scope, scopeID, key, actorID,
	).Scan(&value)
	if err != nil {
		return nil, false
	}
	return json.RawMessage(value), true
}

// findingAppendValue implements array-merge semantics for finding values.
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
