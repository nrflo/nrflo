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

	// Per-MTok USD pricing (nullable — a row with no seeded pricing has all
	// four nil). Drives PriceClass and cost accounting.
	PriceIn         *float64 `json:"price_in,omitempty"`
	PriceOut        *float64 `json:"price_out,omitempty"`
	PriceCacheWrite *float64 `json:"price_cache_write,omitempty"`
	PriceCacheRead  *float64 `json:"price_cache_read,omitempty"`

	// ReleaseDate is the provider's ISO (YYYY-MM-DD) release date, nullable —
	// ""=unknown. Drives newest-release-first ordering in the console picker.
	ReleaseDate string `json:"release_date,omitempty"`
}
