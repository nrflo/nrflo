package consoleui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"be/internal/types"
)

// ConsoleSkill mirrors the server payload (types.go:68 Catalog alias pattern).
type ConsoleSkill = types.ConsoleSkill

type skillsMsg struct {
	skills []ConsoleSkill
	err    error
}

// maxSuggestionRows caps how many matching skills the "/" dropdown renders
// at once, mirroring the web dropdown's scroll cutoff.
const maxSuggestionRows = 8

// filterSkills ports ui/src/components/console/ChatComposerSuggestions.tsx
// filterSkills 1:1: empty query returns all skills; otherwise a
// case-insensitive prefix match wins, falling back to a substring match.
func filterSkills(skills []ConsoleSkill, query string) []ConsoleSkill {
	q := strings.ToLower(query)
	if q == "" {
		return skills
	}
	prefix := make([]ConsoleSkill, 0, len(skills))
	for _, skill := range skills {
		if strings.HasPrefix(strings.ToLower(skill.Name), q) {
			prefix = append(prefix, skill)
		}
	}
	if len(prefix) > 0 {
		return prefix
	}
	substring := make([]ConsoleSkill, 0, len(skills))
	for _, skill := range skills {
		if strings.Contains(strings.ToLower(skill.Name), q) {
			substring = append(substring, skill)
		}
	}
	return substring
}

// slashQuery mirrors ChatComposer.tsx: the draft must start with '/', stay
// on a single line, and have no space yet after the slash.
func slashQuery(value string) (string, bool) {
	if !strings.HasPrefix(value, "/") {
		return "", false
	}
	if strings.Contains(value, "\n") {
		return "", false
	}
	rest := value[1:]
	if strings.Contains(rest, " ") {
		return "", false
	}
	return rest, true
}

// suggestionMatches returns the skills matching the composer's current "/"
// query, or nil when the composer isn't in slash-query position.
func (m *model) suggestionMatches() []ConsoleSkill {
	query, ok := slashQuery(m.input.Value())
	if !ok {
		return nil
	}
	return filterSkills(m.skills, query)
}

// suggestionsOpen reports whether the "/" dropdown should render: a valid
// slash query with at least one match, not dismissed by the user.
func (m *model) suggestionsOpen() bool {
	if m.skillsDismissed {
		return false
	}
	return len(m.suggestionMatches()) > 0
}

func (m *model) loadSkills() tea.Cmd {
	return func() tea.Msg {
		skills, err := m.client.Skills(m.ctx)
		return skillsMsg{skills: skills, err: err}
	}
}
