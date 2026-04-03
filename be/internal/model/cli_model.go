package model

import "time"

// CLIModel represents a CLI model definition (global, not project-scoped)
type CLIModel struct {
	ID              string    `json:"id"`
	CLIType         string    `json:"cli_type"`
	DisplayName     string    `json:"display_name"`
	MappedModel     string    `json:"mapped_model"`
	ReasoningEffort string    `json:"reasoning_effort"`
	ContextLength   int       `json:"context_length"`
	ReadOnly        bool      `json:"read_only"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
