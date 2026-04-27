package model

import (
	"encoding/json"
	"time"
)

// ToolDefinition represents a tool callable by an API-mode agent.
type ToolDefinition struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Endpoint    string          `json:"endpoint"`
	AuthMethod  string          `json:"auth_method"`
	AuthRef     *string         `json:"auth_ref,omitempty"`
	TimeoutSec  int             `json:"timeout_sec"`
	ProjectID   *string         `json:"project_id,omitempty"`
	WorkflowID  *string         `json:"workflow_id,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}
