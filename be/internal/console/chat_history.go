package console

import (
	"fmt"

	"be/internal/repo"
	"be/internal/service"
)

// ProjectHistory resolves projectID's recent console_chat 'user_input'
// message contents for the native console TUI's Up/Down recall seed
// (GET /api/v1/console/history). Mirrors ListSkills's existence check: an
// unknown project maps to service.ErrConsoleProjectNotFound.
func (s *ChatService) ProjectHistory(projectID string, limit int) ([]string, error) {
	exists, err := repo.NewProjectRepo(s.deps.Pool, s.deps.Clock).Exists(projectID)
	if err != nil {
		return nil, fmt.Errorf("check console project: %w", err)
	}
	if !exists {
		return nil, service.ErrConsoleProjectNotFound
	}
	return repo.NewAgentMessageRepo(s.deps.Pool, s.deps.Clock).ProjectConsoleUserInputs(projectID, limit)
}
