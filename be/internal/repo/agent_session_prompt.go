package repo

// GetPrompt returns the stored prompt for an agent session, or "" if unset.
// Lean single-column read used by the autonomous fold to cache an immutable
// task anchor — split from agent_session.go, which is baselined shrink-only.
func (r *AgentSessionRepo) GetPrompt(id string) (string, error) {
	var prompt string
	err := r.db.QueryRow(`SELECT COALESCE(prompt, "") FROM agent_sessions WHERE id = ?`, id).Scan(&prompt)
	return prompt, err
}
