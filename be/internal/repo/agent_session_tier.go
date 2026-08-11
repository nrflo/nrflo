package repo

import (
	"database/sql"
	"time"
)

// UpdateTierResolution persists what actually resolved at spawn time: tier,
// resolved provider/execution_mode/effort, chain position, and the
// fallback_from JSON array of entries tried before the winner. Mirrors
// UpdateCost's targeted-UPDATE shape; nil error on 0 rows affected.
func (r *AgentSessionRepo) UpdateTierResolution(id string, tier *int, provider, execMode, effort string, chainPos int, fallbackFrom string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	var tierVal sql.NullInt64
	if tier != nil {
		tierVal = sql.NullInt64{Int64: int64(*tier), Valid: true}
	}
	_, err := r.db.Exec(
		`UPDATE agent_sessions SET tier = ?, resolved_provider = ?, resolved_execution_mode = ?, resolved_effort = ?, chain_position = ?, fallback_from = ?, updated_at = ? WHERE id = ?`,
		tierVal, provider, execMode, effort, chainPos,
		sql.NullString{String: fallbackFrom, Valid: fallbackFrom != ""}, now, id)
	return err
}

// CountTierResolutions returns landed-spawn counts per canonical chain
// position for tier, over sessions created since cutoff. Feeds the
// weighted-rotation deficit pick (service.WeightedChainOrder).
func (r *AgentSessionRepo) CountTierResolutions(tier int, since time.Time) (map[int]int, error) {
	rows, err := r.db.Query(
		`SELECT chain_position, COUNT(*) FROM agent_sessions
		 WHERE tier = ? AND chain_position IS NOT NULL AND created_at >= ?
		 GROUP BY chain_position`,
		tier, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[int]int{}
	for rows.Next() {
		var pos, n int
		if err := rows.Scan(&pos, &n); err != nil {
			return nil, err
		}
		counts[pos] = n
	}
	return counts, rows.Err()
}
