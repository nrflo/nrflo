package model

import "time"

// SystemAgentDefinition represents a system-level agent definition not tied to any project or workflow
type SystemAgentDefinition struct {
	ID                     string    `json:"id"`
	Role                   string    `json:"role"`
	ExecutionMode          string    `json:"execution_mode"`
	Model                  string    `json:"model"`
	Timeout                int       `json:"timeout"`
	Prompt                 string    `json:"prompt"`
	Tools                  string    `json:"tools"`
	APIMaxIterations       *int      `json:"api_max_iterations,omitempty"`
	APIMaxTokens           *int      `json:"api_max_tokens,omitempty"`
	RestartThreshold       *int      `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int      `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int      `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int      `json:"stall_running_timeout_sec,omitempty"`
	ReasoningEffort        *string   `json:"reasoning_effort,omitempty"` // nil = inherit from the model row; non-nil (incl. "") overrides it
	Tier                   *int      `json:"tier,omitempty"`             // nil = untiered; else selects a tier_models fallback chain
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}
