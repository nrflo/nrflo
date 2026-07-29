package service

import "be/internal/model"

// ListWorkflowInstances returns all workflow instances for a ticket
func (s *WorkflowService) ListWorkflowInstances(projectID, ticketID string) ([]*model.WorkflowInstance, error) {
	return s.wfiRepo.ListByTicket(projectID, ticketID)
}

// ListProjectWorkflowInstances returns all project-scoped workflow instances,
// excluding hidden internal workflows (e.g. _delegate_host, __spec_import__).
func (s *WorkflowService) ListProjectWorkflowInstances(projectID string) ([]*model.WorkflowInstance, error) {
	all, err := s.wfiRepo.ListByProjectScope(projectID)
	if err != nil {
		return nil, err
	}
	instances := all[:0]
	for _, wi := range all {
		if IsHiddenWorkflowName(wi.WorkflowID) {
			continue
		}
		instances = append(instances, wi)
	}
	return instances, nil
}
