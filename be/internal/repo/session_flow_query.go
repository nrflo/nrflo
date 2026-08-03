package repo

import (
	"encoding/json"

	"be/internal/model"
)

// FirstSessionForInstance returns the earliest agent_sessions row bound to
// wfiID (by started_at, falling back to created_at) — used by
// service.BuildSessionFlow as the flow-graph entry node representing a
// sub-workflow/origin-attributed child instance, since the graph is
// session-rooted, not instance-rooted. Returns sql.ErrNoRows when the
// instance has no sessions yet.
func (r *AgentSessionRepo) FirstSessionForInstance(wfiID string) (*model.AgentSession, error) {
	row := r.db.QueryRow(`SELECT `+sessionCols+` FROM agent_sessions
		WHERE workflow_instance_id = ?
		ORDER BY COALESCE(started_at, created_at) ASC, created_at ASC
		LIMIT 1`, wfiID)
	return scanSession(row)
}

// CostTokenRollup sums cost_estimate and every tokens_json counter
// (input/output/cache_read/cache_write, sessioncost.go's flushCostSnapshot
// shape) over sessionIDs via the canonical json_each unnest
// (repo/system_agent_runs.go) — used by service.BuildSessionStats' subtree
// rollup. Empty sessionIDs returns (0, 0, nil).
func (r *AgentSessionRepo) CostTokenRollup(sessionIDs []string) (costUSD float64, tokens int64, err error) {
	if len(sessionIDs) == 0 {
		return 0, 0, nil
	}
	idsJSON, err := json.Marshal(sessionIDs)
	if err != nil {
		return 0, 0, err
	}
	err = r.db.QueryRow(`
		SELECT COALESCE(SUM(cost_estimate), 0),
		       COALESCE(SUM(
		           COALESCE(json_extract(tokens_json, '$.input_tokens'), 0) +
		           COALESCE(json_extract(tokens_json, '$.output_tokens'), 0) +
		           COALESCE(json_extract(tokens_json, '$.cache_read_tokens'), 0) +
		           COALESCE(json_extract(tokens_json, '$.cache_write_tokens'), 0)
		       ), 0)
		FROM agent_sessions
		WHERE id IN (SELECT value FROM json_each(?))`, string(idsJSON)).Scan(&costUSD, &tokens)
	return costUSD, tokens, err
}
