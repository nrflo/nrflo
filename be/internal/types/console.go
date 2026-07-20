package types

// ConsoleCatalog is the server-owned discovery surface used by native console
// clients before they create or resume a chat.
type ConsoleCatalog struct {
	ProjectID string                 `json:"project_id"`
	Engines   []ConsoleEngineOption  `json:"engines"`
	Sessions  []ConsoleSessionOption `json:"sessions"`
	Profiles  []ConsoleProfileOption `json:"profiles"`
}

// ConsoleProfileOption is one built-in console.Profile, as surfaced to a
// chat-creation picker (both server value objects and this type live at the
// types/console boundary so neither the api nor consoleui/UI package needs a
// console.Profile import).
type ConsoleProfileOption struct {
	Name                string `json:"name"`
	DisplayName         string `json:"display_name"`
	Description         string `json:"description"`
	DefaultEngine       string `json:"default_engine"`
	DefaultModelID      string `json:"default_model_id"`
	DefaultEffort       string `json:"default_effort,omitempty"`
	ContextBudgetTokens int    `json:"context_budget_tokens"`
	RefineryDefault     bool   `json:"refinery_default"`
	SystemTemplateID    string `json:"system_template_id,omitempty"`
}

type ConsoleEngineOption struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	// Kind is "cli" or "api"; Brand is the model-family grouping key
	// ("claude"/"gpt") pickers group by. The api engine mixes families, so
	// its Brand is empty and each of its models carries one instead.
	Kind           string               `json:"kind"`
	Brand          string               `json:"brand,omitempty"`
	Enabled        bool                 `json:"enabled"`
	DisabledReason string               `json:"disabled_reason,omitempty"`
	RequiresModel  bool                 `json:"requires_model"`
	Models         []ConsoleModelOption `json:"models"`
}

type ConsoleModelOption struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Brand       string `json:"brand,omitempty"`
	Provider    string `json:"provider,omitempty"`
	MappedModel string `json:"mapped_model,omitempty"`
	// ReasoningEffort is the row's configured (default) effort; create-time
	// overrides must come from SupportedEfforts.
	ReasoningEffort  string   `json:"reasoning_effort,omitempty"`
	SupportedEfforts []string `json:"supported_efforts,omitempty"`
}

type ConsoleSessionOption struct {
	SessionID   string `json:"session_id"`
	Engine      string `json:"engine"`
	Model       string `json:"model,omitempty"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at,omitempty"`
	ContextLeft *int   `json:"context_left,omitempty"`
	// Profile is the console.Profile name the row was started with ('' = none).
	Profile string `json:"profile,omitempty"`
}
