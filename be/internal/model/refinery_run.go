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

	// ChainPosition/ExecutionMode/FallbackFrom record which chain entry this
	// attempt landed on: 0 = primary, execution_mode is that entry's mode
	// (api/cli_interactive), fallback_from is the JSON-marshaled prefix of
	// entries attempted and failed before this one (empty at position 0).
	ChainPosition int    `json:"chain_position"`
	FallbackFrom  string `json:"fallback_from,omitempty"`
	ExecutionMode string `json:"execution_mode"`
}
