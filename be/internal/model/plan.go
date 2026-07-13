package model

import "time"

// Plan lifecycle status (workflow_plans.status).
const (
	PlanStatusDraft     = "draft"
	PlanStatusApproved  = "approved"
	PlanStatusCancelled = "cancelled"
)

// Plan revision author (plan_revisions.author).
const (
	PlanAuthorPlanner = "planner"
	PlanAuthorCaller  = "caller"
)

// PlanRevision is a single append-only entry in a workflow instance's plan
// history. Revisions are never updated in place; the head state lives on
// WorkflowPlan.
type PlanRevision struct {
	InstanceID       string    `json:"instance_id"`
	Revision         int       `json:"revision"`
	Manifest         string    `json:"manifest"`
	Hash             string    `json:"hash"`
	Author           string    `json:"author"`
	PlannerSessionID string    `json:"planner_session_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// WorkflowPlan is the mutable head row tracking a workflow instance's plan
// lifecycle: current status and which revision (if any) was approved.
type WorkflowPlan struct {
	InstanceID       string    `json:"instance_id"`
	Status           string    `json:"status"`
	LatestRevision   int       `json:"latest_revision"`
	ApprovedRevision int       `json:"approved_revision"`
	Goal             string    `json:"goal"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
