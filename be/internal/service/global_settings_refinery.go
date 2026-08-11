package service

import "strconv"

// RefineryFoldStartContextPctKey gates the autonomous refinery fold on
// agent_sessions.context_left: fold iff context_left <= this threshold.
const RefineryFoldStartContextPctKey = "refinery_fold_start_context_pct"

// DefaultRefineryFoldStartContextPct is used when the setting is unset,
// unparseable, or out of [0,100]. It sits well above the relaunch threshold
// (context_left <= 25) for two reasons: the fold debounce needs room to
// fire before a session is killed, and the FIRST fold of a session covers
// everything burned up to this point. Gating late makes that first fold large
// and slow — the expensive case the local fold model is worst at — so the
// threshold buys small incremental folds rather than one big one at the end.
const DefaultRefineryFoldStartContextPct = 60

// RefineryConsoleFoldStartContextPctKey gates console-chat folds the same
// way: fold iff the chat session's context_left <= this threshold.
const RefineryConsoleFoldStartContextPctKey = "refinery_console_fold_start_context_pct"

// DefaultRefineryConsoleFoldStartContextPct (fold once >=25% of context is
// used) is higher than the autonomous default because a console chat's
// digest seeds model-switch siblings and engine restarts that can happen at
// any depth — but a barely-used chat has nothing worth paying a fold for.
const DefaultRefineryConsoleFoldStartContextPct = 75

// GetRefineryFoldStartContextPct returns the global autonomous fold-start
// context threshold, falling back to the default for an unset, unparseable,
// or out-of-[0,100] stored value.
func (s *GlobalSettingsService) GetRefineryFoldStartContextPct() (int, error) {
	return s.getPctConfig(RefineryFoldStartContextPctKey, DefaultRefineryFoldStartContextPct)
}

// GetRefineryConsoleFoldStartContextPct mirrors it for console-chat folds.
func (s *GlobalSettingsService) GetRefineryConsoleFoldStartContextPct() (int, error) {
	return s.getPctConfig(RefineryConsoleFoldStartContextPctKey, DefaultRefineryConsoleFoldStartContextPct)
}

func (s *GlobalSettingsService) getPctConfig(key string, def int) (int, error) {
	val, err := s.pool.GetConfig(key)
	if err != nil {
		return 0, err
	}
	if val == "" {
		return def, nil
	}
	parsed, err := strconv.Atoi(val)
	if err != nil || parsed < 0 || parsed > 100 {
		return def, nil
	}
	return parsed, nil
}
