package model

import "time"

// Consult is one durable consult tracking row (migration 000217), mirroring
// Delegation, so a consult child — which carries no caller column on
// agent_sessions — gains a stable consult_id and caller linkage. Written once
// before the consultant is spawned and marked terminal once the outcome is
// known.
type Consult struct {
	ID                 string
	CallerSessionID    string
	WorkflowInstanceID string
	ProjectID          string
	ConsultantID       string
	Question           string
	ChildSessionID     string
	Status             string
	Error              string
	CreatedAt          time.Time
	CompletedAt        *time.Time
}
