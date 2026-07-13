package repo

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// GetByNode returns findings attributed to a single execution node (read-time
// attribution: findings join agent_sessions on scope_id and are keyed by the
// session's node_id, falling back to phase when node_id is empty — the same
// fallback CreateSession uses for legacy/pre-DYNWF-2 session rows). The second
// return value reports whether any session exists for the node at all, so
// callers can distinguish "unknown node" from "known node, no findings yet".
//
// Rows are ordered so the preferred session (most recently ended, completed
// over running) is scanned last and wins per key — the map-building
// equivalent of GetSessionFindingByKey's LIMIT-1 ordering.
func (r *FindingRepo) GetByNode(wfiID, nodeID string) (map[string]json.RawMessage, bool, error) {
	var probe int
	err := r.db.QueryRow(`
		SELECT 1 FROM agent_sessions
		WHERE workflow_instance_id = ? AND COALESCE(NULLIF(node_id,''), phase) = ?
		LIMIT 1`,
		wfiID, nodeID,
	).Scan(&probe)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	rows, err := r.db.Query(`
		SELECT f.key, f.value
		FROM findings f
		JOIN agent_sessions s ON s.id = f.scope_id
		WHERE f.scope = 'session'
		  AND f.workflow_instance_id = ?
		  AND COALESCE(NULLIF(s.node_id,''), s.phase) = ?
		ORDER BY (s.ended_at IS NULL) DESC, s.ended_at ASC`,
		wfiID, nodeID,
	)
	if err != nil {
		return nil, true, err
	}
	defer rows.Close()

	result := make(map[string]json.RawMessage)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, true, err
		}
		result[k] = json.RawMessage(v)
	}
	return result, true, rows.Err()
}
