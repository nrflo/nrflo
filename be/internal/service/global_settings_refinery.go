package service

import "strconv"

// RefineryFoldStartContextPctKey gates the autonomous refinery fold on
// agent_sessions.context_left: fold iff context_left <= this threshold.
const RefineryFoldStartContextPctKey = "refinery_fold_start_context_pct"

// DefaultRefineryFoldStartContextPct is used when the setting is unset,
// unparseable, or out of [0,100].
const DefaultRefineryFoldStartContextPct = 40

// GetRefineryFoldStartContextPct returns the global fold-start context
// threshold, falling back to DefaultRefineryFoldStartContextPct for an
// unset, unparseable, or out-of-[0,100] stored value.
func (s *GlobalSettingsService) GetRefineryFoldStartContextPct() (int, error) {
	val, err := s.pool.GetConfig(RefineryFoldStartContextPctKey)
	if err != nil {
		return 0, err
	}
	if val == "" {
		return DefaultRefineryFoldStartContextPct, nil
	}
	parsed, err := strconv.Atoi(val)
	if err != nil || parsed < 0 || parsed > 100 {
		return DefaultRefineryFoldStartContextPct, nil
	}
	return parsed, nil
}
