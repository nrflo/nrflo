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

// GetLayerPauseAfter returns a map of layer → pause_after for the given workflow.
func (s *WorkflowLayerPolicyService) GetLayerPauseAfter(projectID, workflowID string) (map[int]bool, error) {
	r := repo.NewWorkflowLayerPolicyRepo(s.pool, s.clock)
	return r.GetLayerPauseAfter(projectID, workflowID)
}

// SetLayerPolicy validates and upserts a pass policy for a specific layer.
// The policy is validated against the current agent count in that layer.
// Preserves the existing pause_after value for the layer.
func (s *WorkflowLayerPolicyService) SetLayerPolicy(projectID, workflowID string, layer int, policy string) error {
	agentDefRepo := repo.NewAgentDefinitionRepo(s.pool, s.clock)
	defs, err := agentDefRepo.ListExecutable(projectID, workflowID)
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
	pauseMap, err := r.GetLayerPauseAfter(projectID, workflowID)
	if err != nil {
		return err
	}

	return r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Layer:      layer,
		PassPolicy: policy,
		PauseAfter: pauseMap[layer],
	})
}

// SetLayerPauseAfter upserts the pause_after flag for a specific layer.
// Preserves the existing pass_policy value for the layer (defaulting to "any").
func (s *WorkflowLayerPolicyService) SetLayerPauseAfter(projectID, workflowID string, layer int, pauseAfter bool) error {
	r := repo.NewWorkflowLayerPolicyRepo(s.pool, s.clock)
	rows, err := r.ListByWorkflow(projectID, workflowID)
	if err != nil {
		return err
	}

	passPolicy := "any"
	for _, row := range rows {
		if row.Layer == layer {
			passPolicy = row.PassPolicy
			break
		}
	}

	return r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Layer:      layer,
		PassPolicy: passPolicy,
		PauseAfter: pauseAfter,
	})
}

// DeleteLayerPolicy removes the pass policy for a specific layer.
func (s *WorkflowLayerPolicyService) DeleteLayerPolicy(projectID, workflowID string, layer int) error {
	r := repo.NewWorkflowLayerPolicyRepo(s.pool, s.clock)
	return r.Delete(projectID, workflowID, layer)
}
