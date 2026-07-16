package service

import (
	"be/internal/model"
	"be/internal/repo"
)

// GetAgentDef retrieves a single agent definition (delegates to the repo,
// which owns the canonical column list + row scanning).
func (s *AgentDefinitionService) GetAgentDef(projectID, workflowID, id string) (*model.AgentDefinition, error) {
	return repo.NewAgentDefinitionRepo(s.pool, s.clock).Get(projectID, workflowID, id)
}

// ListAgentDefs retrieves all agent definitions for a workflow. Never returns
// a nil slice (handlers JSON-encode it as []).
func (s *AgentDefinitionService) ListAgentDefs(projectID, workflowID string) ([]*model.AgentDefinition, error) {
	defs, err := repo.NewAgentDefinitionRepo(s.pool, s.clock).List(projectID, workflowID)
	if err != nil {
		return nil, err
	}
	if defs == nil {
		defs = []*model.AgentDefinition{}
	}
	return defs, nil
}
