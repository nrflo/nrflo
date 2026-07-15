package service

import (
	"encoding/json"

	"github.com/google/uuid"

	"be/internal/model"
	"be/internal/repo"
)

// buildWorkflowInstance creates a WorkflowInstance from a workflow definition.
func (s *WorkflowService) buildWorkflowInstance(projectID, workflowName string, wf *WorkflowDef) *model.WorkflowInstance {
	defProjectID := wf.defProjectID
	if defProjectID == "" {
		defProjectID = projectID
	}
	return &model.WorkflowInstance{
		ID:                uuid.New().String(),
		ProjectID:         projectID,
		DefProjectID:      defProjectID,
		WorkflowID:        workflowName,
		Status:            model.WorkflowInstanceActive,
		RetryCount:        0,
		PurgeOnCompletion: wf.PurgeOnCompletion,
	}
}

// seedFindingsAfterCreate writes seed findings via FindingRepo after wfi creation.
func (s *WorkflowService) seedFindingsAfterCreate(wi *model.WorkflowInstance, seed map[string]string) {
	if len(seed) == 0 {
		return
	}
	denorm := repo.Denorm{ProjectID: wi.ProjectID, WorkflowInstanceID: wi.ID}
	actor := repo.Actor{Source: "system"}
	for k, v := range seed {
		val := json.RawMessage(normalizeJSONValue(v))
		s.findingRepo.Upsert("workflow_instance", wi.ID, k, val, denorm, actor) //nolint:errcheck
	}
}
