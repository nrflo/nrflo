package repo

// GetContextLeft returns the stored context_left percentage for an agent
// session, treating a NULL (unreported) value as 100 (full context) so a
// fresh session never appears due for a fold. Lean single-column read used
// by the refinery autonomous fold gate — split from agent_session.go, which
// is baselined shrink-only.
func (r *AgentSessionRepo) GetContextLeft(id string) (int, error) {
	var contextLeft int
	err := r.db.QueryRow(`SELECT COALESCE(context_left, 100) FROM agent_sessions WHERE id = ?`, id).Scan(&contextLeft)
	return contextLeft, err
}
