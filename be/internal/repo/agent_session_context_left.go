package repo

// GetContextLeftAndModel returns the stored context_left percentage for an
// agent session — treating a NULL (unreported) value as 100 (full context)
// so a fresh session never appears due for a fold — plus the session's model
// id for the refinery fold gates' per-model-tier threshold resolution. Lean
// two-column read split from agent_session.go, which is baselined
// shrink-only.
func (r *AgentSessionRepo) GetContextLeftAndModel(id string) (int, string, error) {
	var contextLeft int
	var modelID string
	err := r.db.QueryRow(`SELECT COALESCE(context_left, 100), COALESCE(model_id, '') FROM agent_sessions WHERE id = ?`, id).Scan(&contextLeft, &modelID)
	return contextLeft, modelID, err
}
