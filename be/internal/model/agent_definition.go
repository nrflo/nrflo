package model

import "time"

// AgentDefinition represents an agent definition stored in the database
type AgentDefinition struct {
	ID               string    `json:"id"`
	ProjectID        string    `json:"project_id"`
	WorkflowID       string    `json:"workflow_id"`
	Model            string    `json:"model"`
	Timeout          int       `json:"timeout"`
	Prompt           string    `json:"prompt"`
	RestartThreshold *int      `json:"restart_threshold,omitempty"`
	MaxFailRestarts  *int      `json:"max_fail_restarts,omitempty"`
	Tag              string    `json:"tag"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
