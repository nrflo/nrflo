package model

import "time"

// SystemAgentDefinition represents a system-level agent definition not tied to any project or workflow
type SystemAgentDefinition struct {
	ID                     string    `json:"id"`
	Model                  string    `json:"model"`
	Timeout                int       `json:"timeout"`
	Prompt                 string    `json:"prompt"`
	RestartThreshold       *int      `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int      `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int      `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int      `json:"stall_running_timeout_sec,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}
