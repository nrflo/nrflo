package service

import (
	"strconv"

	"be/internal/model"
)

// Per-model-tier refinery fold-start keys: each holds a fold-start
// context-left pct for sessions whose model classifies into that cost tier
// (PlanModelTierClass). 0 disables folding for the tier entirely. Precedence
// per session: explicit tier key > explicit per-model key
// (refinery_fold_start_pct_model_<id>) > built-in cheap-off default >
// generic autonomous/console key.
const (
	RefineryFoldStartPctPremiumKey     = "refinery_fold_start_pct_premium"
	RefineryFoldStartPctMidKey         = "refinery_fold_start_pct_mid"
	RefineryFoldStartPctCheapKey       = "refinery_fold_start_pct_cheap"
	RefineryFoldStartPctModelKeyPrefix = "refinery_fold_start_pct_model_"
)

// refineryFoldTierKeys maps a model cost tier to its fold-start config key.
var refineryFoldTierKeys = map[ModelTier]string{
	ModelTierPremium: RefineryFoldStartPctPremiumKey,
	ModelTierMid:     RefineryFoldStartPctMidKey,
	ModelTierCheap:   RefineryFoldStartPctCheapKey,
}

// GetRefineryFoldStartPctForModel resolves the effective fold-start
// context-left pct for a session running modelID (0 = folding disabled).
// console selects the console-chat generic fallback instead of the
// autonomous one. Cheap-tier models default to 0 — they run simple tasks
// and a fold spends more tokens than a lost handoff is worth — unless an
// explicit tier or per-model key re-enables them.
func (s *GlobalSettingsService) GetRefineryFoldStartPctForModel(modelID string, console bool) (int, error) {
	tier := s.modelTier(modelID)

	if v, ok, err := s.pctIfSet(refineryFoldTierKeys[tier]); err != nil {
		return 0, err
	} else if ok {
		return v, nil
	}
	if modelID != "" {
		if v, ok, err := s.pctIfSet(RefineryFoldStartPctModelKeyPrefix + modelID); err != nil {
			return 0, err
		} else if ok {
			return v, nil
		}
	}
	if tier == ModelTierCheap {
		return 0, nil
	}
	if console {
		return s.GetRefineryConsoleFoldStartContextPct()
	}
	return s.GetRefineryFoldStartContextPct()
}

// modelTier classifies modelID through the single PlanModelTierClass
// classifier; an id missing from the registry (raw CLI model names) is
// classified by name via the same fallback path (a transient row with no
// seeded pricing).
func (s *GlobalSettingsService) modelTier(modelID string) ModelTier {
	if modelID == "" {
		return ModelTierMid
	}
	row, err := NewModelService(s.pool, s.clock).Get(modelID)
	if err != nil || row == nil {
		row = &model.Model{ID: modelID}
	}
	return PlanModelTierClass(row)
}

// pctIfSet reads key and reports whether it holds a valid [0,100] integer —
// unlike getPctConfig, an unset/invalid value falls THROUGH (ok=false)
// instead of resolving to a default.
func (s *GlobalSettingsService) pctIfSet(key string) (int, bool, error) {
	val, err := s.pool.GetConfig(key)
	if err != nil {
		return 0, false, err
	}
	if val == "" {
		return 0, false, nil
	}
	parsed, parseErr := strconv.Atoi(val)
	if parseErr != nil || parsed < 0 || parsed > 100 {
		return 0, false, nil
	}
	return parsed, true, nil
}
