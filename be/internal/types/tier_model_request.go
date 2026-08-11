package types

// TierChainEntry is one client-supplied entry in a tier's ordered fallback
// chain. Provider is derived server-side from the model row; position comes
// from the entry's index in the Entries slice.
type TierChainEntry struct {
	ExecutionMode   string `json:"execution_mode"`
	ModelID         string `json:"model_id"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Weight          int    `json:"weight,omitempty"`
}

// SetTierChainRequest replaces a tier's entire ordered chain.
type SetTierChainRequest struct {
	Entries []TierChainEntry `json:"entries"`
}
