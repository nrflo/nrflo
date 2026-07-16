package model

import "time"

// Model is one provider model with optional CLI and direct-API modes.
type Model struct {
	ID             string    `json:"id"`
	Provider       string    `json:"provider"`
	DisplayName    string    `json:"display_name"`
	CLIModel       string    `json:"cli_model"`
	APIModel       string    `json:"api_model"`
	CLIEfforts     []string  `json:"cli_efforts"`
	APIEfforts     []string  `json:"api_efforts"`
	CLIContext     int       `json:"cli_context"`
	APIContext     int       `json:"api_context"`
	FallbackModels string    `json:"fallback_models"`
	DefaultEffort  string    `json:"default_effort"`
	ReadOnly       bool      `json:"read_only"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
