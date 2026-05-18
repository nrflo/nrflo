package model

import "time"

// PythonScript represents a project-scoped Python script or api-mode tool stored in the database.
// Kind is either "agent" (default) or "tool". Tool rows have ToolDescription, InputSchema, TimeoutSec.
type PythonScript struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Code            string    `json:"code"`
	FilePath        string    `json:"file_path"`
	Kind            string    `json:"kind"`
	ToolDescription string    `json:"tool_description"`
	InputSchema     string    `json:"input_schema"`
	TimeoutSec      int       `json:"timeout_sec"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
