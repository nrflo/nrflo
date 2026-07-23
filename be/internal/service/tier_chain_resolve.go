package service

import (
	"fmt"

	"be/internal/db"
)

// AgentChainEntry is one resolved, validated step in an agent's model
// fallback chain. Index 0 (as returned by resolveChain) is the primary
// entry actually used to spawn; later entries are inert until the
// advance-on-failure follow-up consumes them.
type AgentChainEntry struct {
	Provider        string `json:"provider"`
	ExecutionMode   string `json:"execution_mode"`
	ModelID         string `json:"model_id"`
	ReasoningEffort string `json:"reasoning_effort"`
	Tier            int    `json:"tier"`
}

// TierSpec is the def-type-agnostic input to resolveChain: the fields any
// tiered definition (system or workflow) carries, regardless of which table
// it lives in.
type TierSpec struct {
	ID              string
	Model           string
	ExecutionMode   string
	ReasoningEffort *string
	Tier            *int
}

// resolveChain returns spec's ordered model fallback chain:
//   - a non-empty spec.Model is an override — returns a single entry built
//     from the model row's provider, spec's execution_mode, and spec's
//     reasoning_effort override (or the model row's default effort);
//   - otherwise, spec.Tier selects the tier_models chain, ordered by
//     position ASC (position 0 = primary); an empty/unset tier walks down to
//     the nearest lower populated tier (inheritance).
//
// Every entry's model is validated against IsValidModelForMode; an invalid
// entry fails the whole resolution. A stored tier_models execution_mode of
// "" (inherit) is substituted with spec.ExecutionMode in the returned entry
// — a no-op for a def whose own mode already matches, and what lets one
// global tier serve both api-mode system agents and cli-mode workflow
// agents.
func resolveChain(pool *db.Pool, modelSvc *ModelService, spec TierSpec) ([]AgentChainEntry, error) {
	if spec.Model != "" {
		entry, err := resolveOverrideEntry(modelSvc, spec)
		if err != nil {
			return nil, err
		}
		return []AgentChainEntry{entry}, nil
	}

	if spec.Tier == nil {
		return nil, fmt.Errorf("resolve agent chain: %s has no model override and no tier", spec.ID)
	}

	for tier := *spec.Tier; tier >= 1; tier-- {
		entries, err := loadTierChain(pool, modelSvc, tier, spec)
		if err != nil {
			return nil, err
		}
		if len(entries) > 0 {
			return entries, nil
		}
	}
	return nil, fmt.Errorf("resolve agent chain: no tier_models rows for tier %d or below (agent %s)", *spec.Tier, spec.ID)
}

// resolveOverrideEntry builds the single-entry chain for a spec with a
// per-agent model override.
func resolveOverrideEntry(modelSvc *ModelService, spec TierSpec) (AgentChainEntry, error) {
	valid, err := modelSvc.IsValidModelForMode(spec.Model, registryMode(spec.ExecutionMode))
	if err != nil {
		return AgentChainEntry{}, fmt.Errorf("resolve agent chain: validate override model %q: %w", spec.Model, err)
	}
	if !valid {
		return AgentChainEntry{}, fmt.Errorf("resolve agent chain: invalid override model %q for mode %q", spec.Model, spec.ExecutionMode)
	}

	row, err := modelSvc.Get(spec.Model)
	if err != nil {
		return AgentChainEntry{}, fmt.Errorf("resolve agent chain: load model row %q: %w", spec.Model, err)
	}

	effort := row.DefaultEffort
	if spec.ReasoningEffort != nil {
		effort = *spec.ReasoningEffort
	}

	tier := 0
	if spec.Tier != nil {
		tier = *spec.Tier
	}

	return AgentChainEntry{
		Provider:        row.Provider,
		ExecutionMode:   spec.ExecutionMode,
		ModelID:         spec.Model,
		ReasoningEffort: effort,
		Tier:            tier,
	}, nil
}

// loadTierChain reads tier_models rows for tier ordered by position ASC,
// validating each entry's model. Returns an empty slice (not an error) when
// the tier has no rows, so callers can walk down to the next lower tier.
func loadTierChain(pool *db.Pool, modelSvc *ModelService, tier int, spec TierSpec) ([]AgentChainEntry, error) {
	rows, err := pool.Query(
		`SELECT provider, execution_mode, model_id, reasoning_effort
		 FROM tier_models WHERE tier = ? ORDER BY position ASC`, tier)
	if err != nil {
		return nil, fmt.Errorf("resolve agent chain: query tier_models tier=%d: %w", tier, err)
	}
	defer rows.Close()

	entries := []AgentChainEntry{}
	for rows.Next() {
		var e AgentChainEntry
		if err := rows.Scan(&e.Provider, &e.ExecutionMode, &e.ModelID, &e.ReasoningEffort); err != nil {
			return nil, fmt.Errorf("resolve agent chain: scan tier_models tier=%d: %w", tier, err)
		}
		if e.ExecutionMode == "" {
			e.ExecutionMode = spec.ExecutionMode
		}
		e.Tier = tier
		valid, vErr := modelSvc.IsValidModelForMode(e.ModelID, registryMode(e.ExecutionMode))
		if vErr != nil {
			return nil, fmt.Errorf("resolve agent chain: validate tier=%d model %q: %w", tier, e.ModelID, vErr)
		}
		if !valid {
			return nil, fmt.Errorf("resolve agent chain: invalid model %q for mode %q at tier=%d", e.ModelID, e.ExecutionMode, tier)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("resolve agent chain: iterate tier_models tier=%d: %w", tier, err)
	}
	return entries, nil
}
