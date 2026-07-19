package repo

import "time"

// UpdateCost persists the debounced running token/cost snapshot for a
// session. Mirrors UpdateContextLeft's targeted-UPDATE shape; nil error on 0
// rows affected (session not in DB, e.g. a session that ended before its
// first flush).
func (r *AgentSessionRepo) UpdateCost(id string, tokensJSON string, cost float64) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`UPDATE agent_sessions SET tokens_json = ?, cost_estimate = ?, updated_at = ? WHERE id = ?`,
		tokensJSON, cost, now, id)
	return err
}
