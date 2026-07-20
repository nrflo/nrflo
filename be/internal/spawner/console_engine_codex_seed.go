package spawner

// codexFirstTurnText composes the provider-visible text for a codex console
// turn. On the first turn (firstTurnSent==false) it prepends the system
// prompt and, when present, the rotation-carried refinery digest
// (EngineSpec.SeededContext) — codex has no UserPromptSubmit hook, so this is
// its only channel for both. Every later turn returns text unchanged. Callers
// must only flip firstTurnSent to true after the turn/start call this text
// feeds actually succeeds, so a failed first turn still gets the prefix on
// retry.
func codexFirstTurnText(text, systemPrompt, seededContext string, firstTurnSent bool) string {
	if firstTurnSent {
		return text
	}
	prefix := systemPrompt
	if seededContext != "" {
		if prefix != "" {
			prefix += "\n\n"
		}
		prefix += seededContext
	}
	if prefix == "" {
		return text
	}
	return prefix + "\n\n" + text
}
