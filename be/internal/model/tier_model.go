package model

// TierModel is one ordered entry in a tier's fallback chain (tier_models
// table). Position 0 is the chain's primary entry. Weight > 0 marks the
// entry as a weighted-rotation candidate; all-zero weights = strict ordinal.
type TierModel struct {
	Tier            int    `json:"tier"`
	Position        int    `json:"position"`
	Provider        string `json:"provider"`
	ExecutionMode   string `json:"execution_mode"`
	ModelID         string `json:"model_id"`
	ReasoningEffort string `json:"reasoning_effort"`
	Weight          int    `json:"weight"`
}
