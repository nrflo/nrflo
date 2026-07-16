package model

import "time"

// AgentDefinition represents an agent definition stored in the database
type AgentDefinition struct {
	ID                     string    `json:"id"`
	ProjectID              string    `json:"project_id"`
	WorkflowID             string    `json:"workflow_id"`
	Model                  string    `json:"model"`
	Timeout                int       `json:"timeout"`
	Prompt                 string    `json:"prompt"`
	RestartThreshold       *int      `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int      `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int      `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int      `json:"stall_running_timeout_sec,omitempty"`
	Tag                    string    `json:"tag"`
	LowConsumptionModel    string    `json:"low_consumption_model,omitempty"`
	Layer                  int       `json:"layer"`
	ExecutionMode          string    `json:"execution_mode"`
	Tools                  string    `json:"tools"`
	NativeTools            string    `json:"native_tools"` // claude cli_interactive only: CSV → --tools; "" = unrestricted, NativeToolsNone = disable all
	Sandbox                string    `json:"sandbox"`      // codex cli_interactive only: thread/start sandbox; "" = danger-full-access
	APIMaxIterations       *int      `json:"api_max_iterations,omitempty"`
	APIMaxTokens           *int      `json:"api_max_tokens,omitempty"`
	PythonScriptID         *string   `json:"python_script_id,omitempty"`
	ValidationCommands     string    `json:"validation_commands"`
	Consultant             bool      `json:"consultant"`
	NodeRole               string    `json:"node_role"`                  // static|planner|fanout_template; non-static defs never auto-execute as a phase
	Description            string    `json:"description"`                // required (non-empty) when node_role='fanout_template'; the plan catalog's selection text
	ReasoningEffort        *string   `json:"reasoning_effort,omitempty"` // nil = inherit from the model row; non-nil (incl. "") overrides it
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}
