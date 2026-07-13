package types

// AgentDefCreateRequest is the request for creating an agent definition
type AgentDefCreateRequest struct {
	ID                     string    `json:"id"`
	Model                  string    `json:"model,omitempty"`
	Timeout                int       `json:"timeout,omitempty"`
	Prompt                 string    `json:"prompt"`
	Layer                  int       `json:"layer"`
	RestartThreshold       *int      `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int      `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int      `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int      `json:"stall_running_timeout_sec,omitempty"`
	Tag                    string    `json:"tag,omitempty"`
	LowConsumptionModel    string    `json:"low_consumption_model,omitempty"`
	ExecutionMode          string    `json:"execution_mode,omitempty"`
	Tools                  string    `json:"tools,omitempty"`
	APIMaxIterations       *int      `json:"api_max_iterations,omitempty"`
	APIMaxTokens           *int      `json:"api_max_tokens,omitempty"`
	PythonScriptID         *string   `json:"python_script_id,omitempty"`
	ValidationCommands     *[]string `json:"validation_commands,omitempty"`
	Consultant             bool      `json:"consultant,omitempty"`
	NodeRole               string    `json:"node_role,omitempty"`
	Description            string    `json:"description,omitempty"`
}

// AgentDefUpdateRequest is the request for updating an agent definition
type AgentDefUpdateRequest struct {
	Model                  *string   `json:"model,omitempty"`
	Timeout                *int      `json:"timeout,omitempty"`
	Prompt                 *string   `json:"prompt,omitempty"`
	Layer                  *int      `json:"layer,omitempty"`
	RestartThreshold       *int      `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int      `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int      `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int      `json:"stall_running_timeout_sec,omitempty"`
	Tag                    *string   `json:"tag,omitempty"`
	LowConsumptionModel    *string   `json:"low_consumption_model,omitempty"`
	ExecutionMode          *string   `json:"execution_mode,omitempty"`
	Tools                  *string   `json:"tools,omitempty"`
	APIMaxIterations       *int      `json:"api_max_iterations,omitempty"`
	APIMaxTokens           *int      `json:"api_max_tokens,omitempty"`
	PythonScriptID         *string   `json:"python_script_id,omitempty"`
	ValidationCommands     *[]string `json:"validation_commands,omitempty"`
	Consultant             *bool     `json:"consultant,omitempty"`
	NodeRole               *string   `json:"node_role,omitempty"`
	Description            *string   `json:"description,omitempty"`
}

// SystemAgentDefCreateRequest is the request for creating a system agent definition
type SystemAgentDefCreateRequest struct {
	ID                     string `json:"id"`
	Role                   string `json:"role,omitempty"`
	ExecutionMode          string `json:"execution_mode,omitempty"`
	Model                  string `json:"model,omitempty"`
	Timeout                int    `json:"timeout,omitempty"`
	Prompt                 string `json:"prompt"`
	Tools                  string `json:"tools,omitempty"`
	APIMaxIterations       *int   `json:"api_max_iterations,omitempty"`
	APIMaxTokens           *int   `json:"api_max_tokens,omitempty"`
	RestartThreshold       *int   `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int   `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int   `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int   `json:"stall_running_timeout_sec,omitempty"`
}

// SystemAgentDefUpdateRequest is the request for updating a system agent definition
type SystemAgentDefUpdateRequest struct {
	Role                   *string `json:"role,omitempty"`
	ExecutionMode          *string `json:"execution_mode,omitempty"`
	Model                  *string `json:"model,omitempty"`
	Timeout                *int    `json:"timeout,omitempty"`
	Prompt                 *string `json:"prompt,omitempty"`
	Tools                  *string `json:"tools,omitempty"`
	APIMaxIterations       *int    `json:"api_max_iterations,omitempty"`
	APIMaxTokens           *int    `json:"api_max_tokens,omitempty"`
	RestartThreshold       *int    `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int    `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int    `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int    `json:"stall_running_timeout_sec,omitempty"`
}
