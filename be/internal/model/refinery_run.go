package model

import "time"

// RefineryRun is one durable fold attempt (success or failure) written by
// refinery.runFoldCore. Slot key is session_id for a console fold, or
// (workflow_instance_id, node_id) for an autonomous fold.
type RefineryRun struct {
	ID                 int64     `json:"id"`
	SessionID          string    `json:"session_id"`
	WorkflowInstanceID string    `json:"workflow_instance_id"`
	NodeID             string    `json:"node_id"`
	ProjectID          string    `json:"project_id"`
	Provider           string    `json:"provider"`
	Model              string    `json:"model"`
	PromptTokens       int       `json:"prompt_tokens"`
	OutputTokens       int       `json:"output_tokens"`
	Status             string    `json:"status"`
	Error              string    `json:"error"`
	FoldCount          int       `json:"fold_count"`
	FoldedAt           time.Time `json:"folded_at"`
}
