package model

import "time"

// APIModel represents an API model definition (global, not project-scoped)
type APIModel struct {
	ID              string `json:"id"`
	Provider        string `json:"provider"`
	DisplayName     string `json:"display_name"`
	MappedModel     string `json:"mapped_model"`
	ReasoningEffort string `json:"reasoning_effort"`
	// SupportedEfforts lists the effort levels this model accepts (JSON
	// array column); empty means the model offers no effort selection.
	SupportedEfforts []string  `json:"supported_efforts"`
	ContextLength    int       `json:"context_length"`
	ReadOnly         bool      `json:"read_only"`
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
