package service

import (
	"strings"

	"be/internal/db"
)

// DynamicWorkflowAutoEnabledKey gates whether dynamic_workflow/the self-drafting
// plan boundary may auto-approve (mode=auto) instead of suspending at
// waiting_approval. Project override > global; default FALSE (inverse of
// SubworkflowToolsEnabled — auto-approving a caller-drafted plan and
// materializing it with no human in the loop is opt-in).
const DynamicWorkflowAutoEnabledKey = "dynamic_workflow_auto_enabled"

// DynamicWorkflow is the bundled, plan-driven global workflow that backs the
// dynamic_workflow/revise_plan/approve_plan tools when no specific workflow is
// named — seeded by EnsureGlobalDynamicWorkflow next to DeepResearchWorkflow.
const DynamicWorkflow = "dynamic"

// DynamicAutoEnabled reports whether mode=auto is permitted for a project: the
// self-drafting plan boundary auto-approves and materializes instead of
// suspending at waiting_approval. Re-checked per call (StartDynamicWorkflow,
// the plan boundary).
func DynamicAutoEnabled(pool *db.Pool, projectID string) bool {
	raw, err := pool.GetProjectConfig(projectID, DynamicWorkflowAutoEnabledKey)
	if err != nil || raw == "" {
		raw, err = pool.GetConfig(DynamicWorkflowAutoEnabledKey)
		if err != nil || raw == "" {
			return false
		}
	}
	return strings.EqualFold(strings.TrimSpace(raw), "true")
}
