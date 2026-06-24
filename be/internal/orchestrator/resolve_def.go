package orchestrator

import (
	"fmt"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// resolveWorkflowDef resolves a workflow definition and its executable agent
// definitions, falling back to the reserved global project when the workflow is
// not defined under the selected project.
//
// Definitions may be global; execution is always project-scoped. The returned
// definitionProjectID is the project the definition was found under (the
// selected project, or service.GlobalProjectID) — callers MUST use it to load
// the workflow's layer policies and finding schemas so a global workflow's
// sub-definitions resolve from the same place.
func (o *Orchestrator) resolveWorkflowDef(q db.Querier, selectedProjectID, workflowName string) (*model.Workflow, []*model.AgentDefinition, string, error) {
	wfRepo := repo.NewWorkflowRepo(q, o.clock)
	defProjectID := selectedProjectID
	dbWorkflow, err := wfRepo.Get(selectedProjectID, workflowName)
	if err != nil {
		gwf, gerr := wfRepo.Get(service.GlobalProjectID, workflowName)
		if gerr != nil {
			return nil, nil, "", fmt.Errorf("workflow definition '%s' not found: %w", workflowName, err)
		}
		dbWorkflow, defProjectID = gwf, service.GlobalProjectID
	}
	adRepo := repo.NewAgentDefinitionRepo(q, o.clock)
	dbAgentDefs, err := adRepo.ListExecutable(defProjectID, dbWorkflow.ID)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to load agent definitions: %w", err)
	}
	return dbWorkflow, dbAgentDefs, defProjectID, nil
}
