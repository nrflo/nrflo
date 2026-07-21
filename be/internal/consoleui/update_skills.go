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
		name := matches[m.skillIndex].Name
		switch {
		case m.suggestionKind() == suggestionKindTools:
			m.beginInvoke(name)
		case name == invokeDirectiveName:
			m.enterInvokeDirective()
		default:
			m.completeSkill(name)
		}
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

// enterInvokeDirective completes the reserved invoke directive row to
// "/invoke " (mirrors completeSkill's dropdown-state reset) without calling
// beginInvoke: the tool suggestion box opens naturally once invokeQuery
// matches the new composer value.
func (m *model) enterInvokeDirective() {
	m.input.SetValue("/invoke ")
	m.skillIndex = 0
	m.skillsDismissed = false
	m.skillDetails = false
}
