package model

import "time"

// AgentStepCursor tracks a stepwise agent's progress through its steps
// snapshot for one (workflow_instance_id, node_id) pair. StepsSnapshot and
// Completed are kept as raw JSON text (mirrors AgentDefinition.ValidationCommands)
// rather than decoded, since callers round-trip them without needing every field.
type AgentStepCursor struct {
	WorkflowInstanceID string    `json:"workflow_instance_id"`
	NodeID             string    `json:"node_id"`
	StepsSnapshot      string    `json:"steps_snapshot"`
	Revision           int       `json:"revision"`
	CurrentIndex       int       `json:"current_index"`
	Completed          string    `json:"completed"`
	Rejections         string    `json:"rejections"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// CompletedStep is one entry in an AgentStepCursor.Completed JSON array: the
// evidence trail recorded when a step's completion was accepted.
type CompletedStep struct {
	StepID       string   `json:"step_id"`
	EvidenceKeys []string `json:"evidence_keys,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	SessionID    string   `json:"session_id,omitempty"`
	CompletedAt  string   `json:"completed_at"`
}
