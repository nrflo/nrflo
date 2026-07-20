package spawner

// UserTurn is the ConsoleEngine.SendUserTurn payload: Text is always the
// raw "/name args" (or plain) text the user typed and is what gets
// persisted as the user_input row on every engine. Skill is non-nil when
// the console package matched Text's leading "/name" against a discovered
// project skill — pass-through-vs-expand is then an engine-internal
// decision (Rule 6): the claude engine ignores Skill and types Text as-is
// through the PTY (the skill's own SKILL.md drives its native slash-command
// handling), while the codex/api engines expand Skill into the
// provider-visible turn text via expandSkillTurn, mirroring
// codexFirstTurnText/seededTurnText's provider-text-vs-persisted-text split.
type UserTurn struct {
	Text  string
	Skill *SkillMatch
}

// SkillMatch is a project skill resolved against a user's leading "/name"
// text, carried on UserTurn for the engine to interpret. Args is the
// remainder of the typed text after "/name" (trimmed), empty when the user
// typed no arguments.
type SkillMatch struct {
	Name string
	Path string
	Body string
	Args string
}

// expandSkillTurn composes the provider-visible text for a matched skill
// turn: the skill's body, followed by a clearly-delimited arguments section
// when the user supplied any. Shared by the codex and api engines — the
// claude engine never calls this, since it passes turn.Text through raw.
func expandSkillTurn(m *SkillMatch) string {
	if m.Args == "" {
		return m.Body
	}
	return m.Body + "\n\n---\nArguments: " + m.Args
}
