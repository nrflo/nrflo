package ws

// Plan lifecycle event types (see service.PlanService / api/handlers_plan.go).
const (
	EventPlanDrafted   = "plan.drafted"
	EventPlanRevised   = "plan.revised"
	EventPlanApproved  = "plan.approved"
	EventPlanCancelled = "plan.cancelled"
)
