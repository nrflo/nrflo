package consoleui

// handleSuggestionKey owns navigation/selection of the "/" skill dropdown
// while it is open, short-circuiting the composer/viewport updates for the
// keys it consumes (mirrors handleKey's (cmd, handled) contract).
func (m *model) handleSuggestionKey(key string) bool {
	if !m.suggestionsOpen() {
		return false
	}
	matches := m.suggestionMatches()
	switch key {
	case "up", "ctrl+p":
		m.skillIndex = (m.skillIndex - 1 + len(matches)) % len(matches)
		return true
	case "down", "ctrl+n":
		m.skillIndex = (m.skillIndex + 1) % len(matches)
		return true
	case "tab", "enter":
		if m.skillIndex < 0 || m.skillIndex >= len(matches) {
			m.skillIndex = 0
		}
		m.completeSkill(matches[m.skillIndex].Name)
		return true
	case "ctrl+o":
		m.skillDetails = !m.skillDetails
		return true
	case "esc":
		if m.skillDetails {
			m.skillDetails = false
		} else {
			m.skillsDismissed = true
		}
		return true
	}
	return false
}

// completeSkill fills the composer with "/name " (the trailing space closes
// the suggestion box on the next keystroke) and resets dropdown state.
func (m *model) completeSkill(name string) {
	m.input.SetValue("/" + name + " ")
	m.skillIndex = 0
	m.skillsDismissed = false
	m.skillDetails = false
}
