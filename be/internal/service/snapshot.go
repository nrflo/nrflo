package service

import (
	"strings"

	"be/internal/ws"
)

// WorkflowSnapshotProvider implements ws.SnapshotProvider using WorkflowService.
type WorkflowSnapshotProvider struct {
	svc *WorkflowService
}

// NewWorkflowSnapshotProvider creates a snapshot provider backed by the workflow service.
func NewWorkflowSnapshotProvider(svc *WorkflowService) *WorkflowSnapshotProvider {
	return &WorkflowSnapshotProvider{svc: svc}
}

// BuildSnapshot builds snapshot chunks for a given project+ticket scope.
// For ticket-scoped subscriptions, it returns workflow state, active agents, agent history, and findings.
// For project-wide subscriptions (empty ticketID), it returns all workflow instances.
func (p *WorkflowSnapshotProvider) BuildSnapshot(projectID, ticketID string) ([]ws.SnapshotChunk, error) {
	projectID = strings.ToLower(projectID)
	ticketID = strings.ToLower(ticketID)

	var chunks []ws.SnapshotChunk

	if ticketID != "" {
		return p.buildTicketSnapshot(projectID, ticketID)
	}

	// Project-wide: include all active workflow instances
	instances, err := p.svc.wfiRepo.ListByProjectScope(projectID)
	if err != nil {
		return nil, err
	}
	for _, wi := range instances {
		state := p.svc.buildV4State(wi)
		chunks = append(chunks, ws.SnapshotChunk{
			Entity: ws.EntityWorkflowState,
			Data:   state,
		})
	}
	return chunks, nil
}

func (p *WorkflowSnapshotProvider) buildTicketSnapshot(projectID, ticketID string) ([]ws.SnapshotChunk, error) {
	instances, err := p.svc.ListWorkflowInstances(projectID, ticketID)
	if err != nil {
		return nil, err
	}

	var chunks []ws.SnapshotChunk
	for _, wi := range instances {
		state := p.svc.buildV4State(wi)
		chunks = append(chunks, ws.SnapshotChunk{
			Entity: ws.EntityWorkflowState,
			Data:   state,
		})
	}
	return chunks, nil
}
