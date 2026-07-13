package types

import "encoding/json"

// PlanAnswer is a caller-supplied answer to a planner question, threaded back
// into the planner prompt on a feedback-driven re-plan.
type PlanAnswer struct {
	QuestionID string `json:"question_id"`
	Answer     string `json:"answer"`
}

// PlanReviseRequest is the request for POST /workflow-instances/{iid}/plan/revise.
// Exactly one of Manifest (caller-edited manifest, stored as-is after
// validation) or Feedback/Answers (re-runs the planner agent) should be set.
// Revision must match the current head's latest_revision or the call is
// rejected as stale (409).
type PlanReviseRequest struct {
	Revision int             `json:"revision"`
	Manifest json.RawMessage `json:"manifest,omitempty"`
	Goal     string          `json:"goal,omitempty"`
	Feedback string          `json:"feedback,omitempty"`
	Answers  []PlanAnswer    `json:"answers,omitempty"`
}

// PlanApproveRequest is the request for POST /workflow-instances/{iid}/plan/approve.
// Revision must match the current head's latest_revision or the call is
// rejected as stale (409).
type PlanApproveRequest struct {
	Revision int `json:"revision"`
}
