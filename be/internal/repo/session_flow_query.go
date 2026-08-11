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

// SessionsForInstance returns every agent_sessions row bound to wfiID in
// start order — used by service.BuildSessionFlow to expand an
// origin-attributed instance (e.g. the reusable _refinery_fold host, where
// each fold is a new one-off session) into one flow node per session instead
// of freezing on the earliest.
func (r *AgentSessionRepo) SessionsForInstance(wfiID string) ([]*model.AgentSession, error) {
	rows, err := r.db.Query(`SELECT `+sessionCols+` FROM agent_sessions
		WHERE workflow_instance_id = ?
		ORDER BY COALESCE(started_at, created_at) ASC, created_at ASC`, wfiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.AgentSession
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CacheRollup sums the per-kind token counters over sessionIDs and computes
// the would-be cost with NO prompt caching: every prompt token
// (input + cache_read + cache_write) at the session model's full price_in,
// output at price_out. Sessions whose model row is missing or unpriced
// contribute 0 to noCacheCost, mirroring PricingKnown's cost-0 semantics.
func (r *AgentSessionRepo) CacheRollup(sessionIDs []string) (input, cacheRead, cacheWrite, output int64, noCacheCostUSD float64, err error) {
	if len(sessionIDs) == 0 {
		return 0, 0, 0, 0, 0, nil
	}
	idsJSON, err := json.Marshal(sessionIDs)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	err = r.db.QueryRow(`
		SELECT COALESCE(SUM(COALESCE(json_extract(s.tokens_json, '$.input_tokens'), 0)), 0),
		       COALESCE(SUM(COALESCE(json_extract(s.tokens_json, '$.cache_read_tokens'), 0)), 0),
		       COALESCE(SUM(COALESCE(json_extract(s.tokens_json, '$.cache_write_tokens'), 0)), 0),
		       COALESCE(SUM(COALESCE(json_extract(s.tokens_json, '$.output_tokens'), 0)), 0),
		       COALESCE(SUM(
		           (COALESCE(json_extract(s.tokens_json, '$.input_tokens'), 0) +
		            COALESCE(json_extract(s.tokens_json, '$.cache_read_tokens'), 0) +
		            COALESCE(json_extract(s.tokens_json, '$.cache_write_tokens'), 0)) / 1e6 * COALESCE(m.price_in, 0) +
		           COALESCE(json_extract(s.tokens_json, '$.output_tokens'), 0) / 1e6 * COALESCE(m.price_out, 0)
		       ), 0)
		FROM agent_sessions s
		LEFT JOIN models m ON m.id = s.model_id
		WHERE s.id IN (SELECT value FROM json_each(?))`, string(idsJSON)).
		Scan(&input, &cacheRead, &cacheWrite, &output, &noCacheCostUSD)
	return input, cacheRead, cacheWrite, output, noCacheCostUSD, err
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
