package types

// ConsoleCatalog is the server-owned discovery surface used by native console
// clients before they create or resume a chat.
type ConsoleCatalog struct {
	ProjectID string                 `json:"project_id"`
	Engines   []ConsoleEngineOption  `json:"engines"`
	Sessions  []ConsoleSessionOption `json:"sessions"`
}

type ConsoleEngineOption struct {
	ID             string               `json:"id"`
	DisplayName    string               `json:"display_name"`
	Enabled        bool                 `json:"enabled"`
	DisabledReason string               `json:"disabled_reason,omitempty"`
	RequiresModel  bool                 `json:"requires_model"`
	Models         []ConsoleModelOption `json:"models"`
}

type ConsoleModelOption struct {
	ID              string `json:"id"`
	DisplayName     string `json:"display_name"`
	Provider        string `json:"provider,omitempty"`
	MappedModel     string `json:"mapped_model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type ConsoleSessionOption struct {
	SessionID   string `json:"session_id"`
	Engine      string `json:"engine"`
	Model       string `json:"model,omitempty"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at,omitempty"`
	ContextLeft *int   `json:"context_left,omitempty"`
}
