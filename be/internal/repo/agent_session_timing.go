package repo

import "time"

// UpdateTimeBuckets persists the debounced running timing-bucket snapshot
// for a session. Mirrors UpdateCost's targeted-UPDATE shape; nil error on 0
// rows affected (session not in DB, e.g. a session that ended before its
// first flush).
func (r *AgentSessionRepo) UpdateTimeBuckets(id string, bucketsJSON string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`UPDATE agent_sessions SET time_buckets_json = ?, updated_at = ? WHERE id = ?`,
		bucketsJSON, now, id)
	return err
}
