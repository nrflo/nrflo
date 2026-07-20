package service

import "database/sql"

// GetSessionProjectID returns the project_id for a session, or ("", nil) when not found.
func (s *AgentService) GetSessionProjectID(sessionID string) (string, error) {
	var projectID string
	err := s.pool.QueryRow(`SELECT project_id FROM agent_sessions WHERE id = ?`, sessionID).Scan(&projectID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return projectID, nil
}

// GetSessionKind returns the agent_sessions.kind for sessionID ("" when the
// session is not found), letting hook handlers scope behavior to a specific
// session kind (e.g. workflow_agent) without paying for the full joined
// GetSessionByID load.
func (s *AgentService) GetSessionKind(sessionID string) (string, error) {
	var kind string
	err := s.pool.QueryRow(`SELECT kind FROM agent_sessions WHERE id = ?`, sessionID).Scan(&kind)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return kind, nil
}
