package spawner

import (
	"context"
	"fmt"

	"be/internal/repo"
)

// ConsultHost is the hidden-host counterpart to Consult for a caller with no
// bound workflow instance (a console session, reached via
// orchestrator.APIConsultant -> console.Deps.Consultant): it resolves
// consultantID by searching the project's agent_definitions (local, then the
// reserved global namespace — repo.AgentDefinitionRepo.FindConsultant)
// instead of scoping to one caller-known workflow, and runs with no
// transcript (a console chat has no AgentSession-backed message history to
// summarize).
func (s *Spawner) ConsultHost(ctx context.Context, projectID, consultantID, question string) (string, error) {
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("consult: no database pool")
	}
	def, err := repo.NewAgentDefinitionRepo(pool, s.config.Clock).FindConsultant(projectID, consultantID)
	if err != nil {
		return "", fmt.Errorf("consult: %w", err)
	}

	return s.runConsult(ctx, consultRequest{
		ProjectID:    projectID,
		WorkflowName: def.WorkflowID,
		ScopeType:    "project",
		ConsultantID: consultantID,
		Def:          def,
		Question:     question,
	})
}
