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

// maxSuggestionRows caps how many matching rows the "/" / "/invoke " dropdown
// renders at once, mirroring the web dropdown's scroll cutoff.
const maxSuggestionRows = 8

// suggestionItem is the row shape the windowed suggestion box renders,
// generalized over skills (the "/" directive) and tools (the "/invoke "
// directive).
type suggestionItem struct {
	Name        string
	Description string
}

func toSuggestionItems[T any](items []T, name, desc func(T) string) []suggestionItem {
	out := make([]suggestionItem, len(items))
	for i, item := range items {
		out[i] = suggestionItem{Name: name(item), Description: desc(item)}
	}
	return out
}

// filterByName ports ui/src/components/console/ChatComposerSuggestions.tsx
// filterSkills 1:1, generalized over any named item: empty query returns all
// items; otherwise a case-insensitive prefix match wins, falling back to a
// substring match.
func filterByName[T any](items []T, query string, name func(T) string) []T {
	q := strings.ToLower(query)
	if q == "" {
		return items
	}
	prefix := make([]T, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(name(item)), q) {
			prefix = append(prefix, item)
		}
	}
	if len(prefix) > 0 {
		return prefix
	}
	substring := make([]T, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(name(item)), q) {
			substring = append(substring, item)
		}
	}
	return substring
}

func filterSkills(skills []ConsoleSkill, query string) []ConsoleSkill {
	return filterByName(skills, query, func(s ConsoleSkill) string { return s.Name })
}

func filterTools(tools []ConsoleTool, query string) []ConsoleTool {
	return filterByName(tools, query, func(t ConsoleTool) string { return t.Name })
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

// suggestionKindType distinguishes which catalogue the suggestion box is
// currently sourcing rows from.
type suggestionKindType int

const (
	suggestionKindNone suggestionKindType = iota
	suggestionKindSkills
	suggestionKindTools
)

// suggestionKind reports which directive (if any) the composer's current
// value is in: tools while a "/invoke " query is active, skills while a "/"
// query is active, none while an invoke flow is already in progress (the
// directive text has been cleared by beginInvoke by then, but this also
// guards against races).
func (m *model) suggestionKind() suggestionKindType {
	if m.invoke.active {
		return suggestionKindNone
	}
	if _, ok := invokeQuery(m.input.Value()); ok {
		return suggestionKindTools
	}
	if _, ok := slashQuery(m.input.Value()); ok {
		return suggestionKindSkills
	}
	return suggestionKindNone
}

// suggestionMatches returns the rows matching the composer's current
// directive query, dispatching on suggestionKind; nil when neither directive
// is active.
func (m *model) suggestionMatches() []suggestionItem {
	switch m.suggestionKind() {
	case suggestionKindTools:
		query, _ := invokeQuery(m.input.Value())
		return toSuggestionItems(filterTools(m.tools, query),
			func(t ConsoleTool) string { return t.Name },
			func(t ConsoleTool) string { return t.Description })
	case suggestionKindSkills:
		query, _ := slashQuery(m.input.Value())
		return toSuggestionItems(filterSkills(m.skills, query),
			func(s ConsoleSkill) string { return s.Name },
			func(s ConsoleSkill) string { return s.Description })
	default:
		return nil
	}
}

// suggestionsOpen reports whether the "/" / "/invoke " dropdown should
// render: a valid directive query with at least one match, not dismissed by
// the user, and no invoke flow already in progress.
func (m *model) suggestionsOpen() bool {
	if m.invoke.active {
		return false
	}
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
