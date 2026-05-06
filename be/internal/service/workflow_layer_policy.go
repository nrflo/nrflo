package service

import (
	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// WorkflowLayerPolicyService manages per-layer pass policies for workflows.
type WorkflowLayerPolicyService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewWorkflowLayerPolicyService creates a new WorkflowLayerPolicyService.
func NewWorkflowLayerPolicyService(pool *db.Pool, clk clock.Clock) *WorkflowLayerPolicyService {
	return &WorkflowLayerPolicyService{pool: pool, clock: clk}
}

// GetLayerPolicies returns a map of layer → pass_policy for the given workflow.
// Returns an empty map (not an error) when no policies are configured.
func (s *WorkflowLayerPolicyService) GetLayerPolicies(projectID, workflowID string) (map[int]string, error) {
	r := repo.NewWorkflowLayerPolicyRepo(s.pool, s.clock)
	rows, err := r.ListByWorkflow(projectID, workflowID)
	if err != nil {
		return nil, err
	}
	result := make(map[int]string, len(rows))
	for _, row := range rows {
		result[row.Layer] = row.PassPolicy
	}
	return result, nil
}

// SetLayerPolicy validates and upserts a pass policy for a specific layer.
// The policy is validated against the current agent count in that layer.
func (s *WorkflowLayerPolicyService) SetLayerPolicy(projectID, workflowID string, layer int, policy string) error {
	agentDefRepo := repo.NewAgentDefinitionRepo(s.pool, s.clock)
	defs, err := agentDefRepo.List(projectID, workflowID)
	if err != nil {
		return err
	}

	count := 0
	for _, d := range defs {
		if d.Layer == layer {
			count++
		}
	}

	if err := ValidateLayerPolicy(policy, count); err != nil {
		return err
	}

	r := repo.NewWorkflowLayerPolicyRepo(s.pool, s.clock)
	return r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Layer:      layer,
		PassPolicy: policy,
	})
}

// DeleteLayerPolicy removes the pass policy for a specific layer.
func (s *WorkflowLayerPolicyService) DeleteLayerPolicy(projectID, workflowID string, layer int) error {
	r := repo.NewWorkflowLayerPolicyRepo(s.pool, s.clock)
	return r.Delete(projectID, workflowID, layer)
}
