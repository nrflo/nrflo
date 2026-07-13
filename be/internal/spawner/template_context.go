package spawner

import (
	"be/internal/model"
	"be/internal/repo"
)

// resolveWFIID resolves a workflow instance ID from direct wfiID or ticket/project lookup.
func (s *Spawner) resolveWFIID(projectID, ticketID, workflowName, wfiID string) string {
	return s.resolveWFIIDFromPool(projectID, ticketID, workflowName, wfiID)
}

func (s *Spawner) resolveWFIIDFromPool(projectID, ticketID, workflowName, wfiID string) string {
	pool := s.pool()
	if pool == nil {
		return ""
	}
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.config.Clock)
	var wi *model.WorkflowInstance
	var err error
	if wfiID != "" {
		wi, err = wfiRepo.Get(wfiID)
	} else if ticketID != "" {
		wi, err = wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	} else {
		var instances []*model.WorkflowInstance
		instances, err = wfiRepo.ListActiveByProjectAndWorkflow(projectID, workflowName)
		if err == nil && len(instances) > 0 {
			wi = instances[len(instances)-1]
		}
	}
	if err != nil || wi == nil {
		return ""
	}
	return wi.ID
}
