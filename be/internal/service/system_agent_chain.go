package service

import (
	"fmt"

	"be/internal/model"
)

// AgentChainEntry is one resolved, validated step in an agent's model
// fallback chain. Index 0 (as returned by ResolveAgentChain) is the primary
// entry actually used to spawn; later entries are inert until the
// advance-on-failure follow-up consumes them.
type AgentChainEntry struct {
	Provider        string
	ExecutionMode   string
	ModelID         string
	ReasoningEffort string
}

// ResolveAgentChain returns def's ordered model fallback chain:
//   - a non-empty def.Model is an override — returns a single entry built
//     from the model row's provider, def's execution_mode, and def's
//     reasoning_effort override (or the model row's default effort);
//   - otherwise, def.Tier selects the tier_models chain, ordered by
//     position ASC (position 0 = primary); an empty/unset tier walks down to
//     the nearest lower populated tier (inheritance).
//
// Every entry's model is validated against IsValidModelForMode; an invalid
// entry fails the whole resolution.
func (s *SystemAgentDefinitionService) ResolveAgentChain(def *model.SystemAgentDefinition) ([]AgentChainEntry, error) {
	if def == nil {
		return nil, fmt.Errorf("resolve agent chain: nil definition")
	}

	if def.Model != "" {
		entry, err := s.resolveOverrideEntry(def)
		if err != nil {
			return nil, err
		}
		return []AgentChainEntry{entry}, nil
	}

	if def.Tier == nil {
		return nil, fmt.Errorf("resolve agent chain: %s has no model override and no tier", def.ID)
	}

	for tier := *def.Tier; tier >= 1; tier-- {
		entries, err := s.loadTierChain(tier)
		if err != nil {
			return nil, err
		}
		if len(entries) > 0 {
			return entries, nil
		}
	}
	return nil, fmt.Errorf("resolve agent chain: no tier_models rows for tier %d or below (agent %s)", *def.Tier, def.ID)
}

// resolveOverrideEntry builds the single-entry chain for a def with a
// per-agent model override.
func (s *SystemAgentDefinitionService) resolveOverrideEntry(def *model.SystemAgentDefinition) (AgentChainEntry, error) {
	valid, err := s.modelSvc.IsValidModelForMode(def.Model, registryMode(def.ExecutionMode))
	if err != nil {
		return AgentChainEntry{}, fmt.Errorf("resolve agent chain: validate override model %q: %w", def.Model, err)
	}
	if !valid {
		return AgentChainEntry{}, fmt.Errorf("resolve agent chain: invalid override model %q for mode %q", def.Model, def.ExecutionMode)
	}

	row, err := s.modelSvc.Get(def.Model)
	if err != nil {
		return AgentChainEntry{}, fmt.Errorf("resolve agent chain: load model row %q: %w", def.Model, err)
	}

	effort := row.DefaultEffort
	if def.ReasoningEffort != nil {
		effort = *def.ReasoningEffort
	}

	return AgentChainEntry{
		Provider:        row.Provider,
		ExecutionMode:   def.ExecutionMode,
		ModelID:         def.Model,
		ReasoningEffort: effort,
	}, nil
}

// loadTierChain reads tier_models rows for tier ordered by position ASC,
// validating each entry's model. Returns an empty slice (not an error) when
// the tier has no rows, so callers can walk down to the next lower tier.
func (s *SystemAgentDefinitionService) loadTierChain(tier int) ([]AgentChainEntry, error) {
	rows, err := s.pool.Query(
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
		valid, vErr := s.modelSvc.IsValidModelForMode(e.ModelID, registryMode(e.ExecutionMode))
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
