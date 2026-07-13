package ws

// Plan lifecycle event types (see service.PlanService / api/handlers_plan.go).
const (
	EventPlanDrafted   = "plan.drafted"
	EventPlanRevised   = "plan.revised"
	EventPlanApproved  = "plan.approved"
	EventPlanCancelled = "plan.cancelled"

	// EventPlanMaterialized fires once an approved plan's nodes are written to
	// workflow_instance_nodes (orchestrator plan boundary or Approve).
	EventPlanMaterialized = "plan.materialized"
	// EventPlanWaiting fires when the orchestrator suspends a run at the plan
	// boundary (payload carries instance_id + the derived plan status).
	EventPlanWaiting = "workflow.plan_waiting"
)
