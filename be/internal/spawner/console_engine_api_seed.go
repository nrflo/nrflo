package spawner

import "be/internal/db"

// seededTurnText composes the PROVIDER-visible text for the api console
// engine's first turn: the rotation-carried refinery digest
// (EngineSpec.SeededContext) prepended to the user's text. The api engine has
// no UserPromptSubmit hook (unlike claude), so this is its only channel for
// carrying the digest forward across a rotation. Callers must persist/emit
// the original, unprefixed text separately — this composed text is passed to
// the provider only, never persisted as the user's message.
func seededTurnText(seededContext, text string) string {
	return seededContext + "\n\n" + text
}

// watcherBudget returns profileBudget when set, else the derived per-model
// default budget (context_budget_fraction * maxContext, or the
// context_budget_default absolute override) — the api console engine's
// context-watcher budget.
func watcherBudget(pool *db.Pool, profileBudget, maxContext int) int {
	if profileBudget > 0 {
		return profileBudget
	}
	return deriveContextBudgetDefault(pool, maxContext)
}
