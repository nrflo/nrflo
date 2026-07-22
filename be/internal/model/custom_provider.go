package model

import "time"

// CustomProvider is a registered BYO OpenAI-compatible API provider (a local
// Ollama/LM Studio/llama.cpp server, or any other OpenAI-compatible
// endpoint). APIWire selects the wire protocol used to talk to it: the
// non-stateful Responses API (default) or Chat Completions.
type CustomProvider struct {
	Name      string    `json:"name"`
	BaseURL   string    `json:"base_url"`
	APIKey    string    `json:"api_key"`
	APIWire   string    `json:"api_wire"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
