package types

import (
	"encoding/json"

	"be/internal/model"
)

// AgentDefCreateRequest is the request for creating an agent definition
type AgentDefCreateRequest struct {
	ID                     string                  `json:"id"`
	Model                  string                  `json:"model,omitempty"`
	Timeout                int                     `json:"timeout,omitempty"`
	Prompt                 string                  `json:"prompt"`
	Layer                  int                     `json:"layer"`
	RestartThreshold       *int                    `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int                    `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int                    `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int                    `json:"stall_running_timeout_sec,omitempty"`
	ContextBudgetTokens    *int                    `json:"context_budget_tokens,omitempty"`
	Tag                    string                  `json:"tag,omitempty"`
	LowConsumptionModel    string                  `json:"low_consumption_model,omitempty"`
	ExecutionMode          string                  `json:"execution_mode,omitempty"`
	Tools                  string                  `json:"tools,omitempty"`
	NativeTools            string                  `json:"native_tools,omitempty"`
	Sandbox                string                  `json:"sandbox,omitempty"`
	APIMaxIterations       *int                    `json:"api_max_iterations,omitempty"`
	APIMaxTokens           *int                    `json:"api_max_tokens,omitempty"`
	PythonScriptID         *string                 `json:"python_script_id,omitempty"`
	ValidationCommands     *[]string               `json:"validation_commands,omitempty"`
	Consultant             bool                    `json:"consultant,omitempty"`
	NodeRole               string                  `json:"node_role,omitempty"`
	Description            string                  `json:"description,omitempty"`
	ReasoningEffort        *string                 `json:"reasoning_effort,omitempty"`
	SystemTemplateID       string                  `json:"system_template_id,omitempty"`
	Tier                   *int                    `json:"tier,omitempty"`
	PromptMode             string                  `json:"prompt_mode,omitempty"`
	Steps                  *[]model.StepDefinition `json:"steps,omitempty"`
}

// AgentDefUpdateRequest is the request for updating an agent definition
type AgentDefUpdateRequest struct {
	Model                  *string                 `json:"model,omitempty"`
	Timeout                *int                    `json:"timeout,omitempty"`
	Prompt                 *string                 `json:"prompt,omitempty"`
	Layer                  *int                    `json:"layer,omitempty"`
	RestartThreshold       *int                    `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int                    `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int                    `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int                    `json:"stall_running_timeout_sec,omitempty"`
	ContextBudgetTokens    *int                    `json:"context_budget_tokens,omitempty"`
	Tag                    *string                 `json:"tag,omitempty"`
	LowConsumptionModel    *string                 `json:"low_consumption_model,omitempty"`
	ExecutionMode          *string                 `json:"execution_mode,omitempty"`
	Tools                  *string                 `json:"tools,omitempty"`
	NativeTools            *string                 `json:"native_tools,omitempty"`
	Sandbox                *string                 `json:"sandbox,omitempty"`
	APIMaxIterations       *int                    `json:"api_max_iterations,omitempty"`
	APIMaxTokens           *int                    `json:"api_max_tokens,omitempty"`
	PythonScriptID         *string                 `json:"python_script_id,omitempty"`
	ValidationCommands     *[]string               `json:"validation_commands,omitempty"`
	Consultant             *bool                   `json:"consultant,omitempty"`
	NodeRole               *string                 `json:"node_role,omitempty"`
	Description            *string                 `json:"description,omitempty"`
	ReasoningEffort        *string                 `json:"reasoning_effort,omitempty"`
	SystemTemplateID       *string                 `json:"system_template_id,omitempty"`
	Tier                   *int                    `json:"tier,omitempty"`
	PromptMode             *string                 `json:"prompt_mode,omitempty"`
	Steps                  *[]model.StepDefinition `json:"steps,omitempty"`
	// TierClear is set when the request body explicitly sends `"tier": null`
	// (as opposed to omitting the field), distinguishing "clear the tier"
	// from "leave tier untouched" — see UnmarshalJSON below.
	TierClear bool `json:"-"`
}

// UnmarshalJSON distinguishes an explicit `"tier": null` (clear) from an
// omitted `tier` field (leave untouched); both otherwise unmarshal Tier to
// the same nil *int.
func (r *AgentDefUpdateRequest) UnmarshalJSON(data []byte) error {
	type alias AgentDefUpdateRequest
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*r = AgentDefUpdateRequest(a)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["tier"]; ok && string(v) == "null" {
		r.TierClear = true
	}
	return nil
}

// SystemAgentDefCreateRequest is the request for creating a system agent definition
type SystemAgentDefCreateRequest struct {
	ID                     string  `json:"id"`
	Role                   string  `json:"role,omitempty"`
	ExecutionMode          string  `json:"execution_mode,omitempty"`
	Model                  string  `json:"model,omitempty"`
	Timeout                int     `json:"timeout,omitempty"`
	Prompt                 string  `json:"prompt"`
	Tools                  string  `json:"tools,omitempty"`
	APIMaxIterations       *int    `json:"api_max_iterations,omitempty"`
	APIMaxTokens           *int    `json:"api_max_tokens,omitempty"`
	RestartThreshold       *int    `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int    `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int    `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int    `json:"stall_running_timeout_sec,omitempty"`
	ReasoningEffort        *string `json:"reasoning_effort,omitempty"`
	Tier                   *int    `json:"tier,omitempty"`
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
	ReasoningEffort        *string `json:"reasoning_effort,omitempty"`
	Tier                   *int    `json:"tier,omitempty"`
}
