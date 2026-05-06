package model

import "time"

// WorkflowLayerPolicy defines the pass policy for a specific layer in a workflow.
type WorkflowLayerPolicy struct {
	ProjectID  string    `json:"project_id"`
	WorkflowID string    `json:"workflow_id"`
	Layer      int       `json:"layer"`
	PassPolicy string    `json:"pass_policy"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
