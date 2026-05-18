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
	MaxFailRestarts        *int      `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int      `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int      `json:"stall_running_timeout_sec,omitempty"`
	Tag                    string    `json:"tag"`
	LowConsumptionModel    string    `json:"low_consumption_model,omitempty"`
	Layer                  int       `json:"layer"`
	ExecutionMode          string    `json:"execution_mode"`
	Tools                  string    `json:"tools"`
	APIMaxIterations       *int      `json:"api_max_iterations,omitempty"`
	PythonScriptID         *string   `json:"python_script_id,omitempty"`
	ValidationCommands     string    `json:"validation_commands"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
