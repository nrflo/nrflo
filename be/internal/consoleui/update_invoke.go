package consoleui

import (
	tea "charm.land/bubbletea/v2"
)

// loadTools fetches the chat's own invokable tool catalogue, mirroring
// loadSkills.
func (m *model) loadTools() tea.Cmd {
	return func() tea.Msg {
		tools, err := m.client.Tools(m.ctx)
		return toolsMsg{tools: tools, err: err}
	}
}

func (m *model) toolByName(name string) (ConsoleTool, bool) {
	for _, tool := range m.tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return ConsoleTool{}, false
}

// beginInvoke starts the /invoke flow for the selected tool: looks it up by
// name, builds the pure invokeState, clears the "/invoke ..." directive text
// from the composer, and prefills the composer with the first field's
// default (or leaves it empty when the flow jumps straight to confirm).
func (m *model) beginInvoke(name string) {
	tool, ok := m.toolByName(name)
	if !ok {
		return
	}
	m.invoke = startInvoke(tool.Name, toolArgFields(tool.InputSchema))
	m.skillIndex = 0
	m.skillsDismissed = false
	m.skillDetails = false
	m.prefillInvokeComposer()
}

// prefillInvokeComposer loads the current arg field's default into the
// composer during the args phase, or clears it once in confirm phase.
func (m *model) prefillInvokeComposer() {
	if m.invoke.phase == invokePhaseArgs && m.invoke.index < len(m.invoke.fields) {
		m.input.SetValue(m.invoke.fields[m.invoke.index].Default)
	} else {
		m.input.Reset()
	}
}

// handleInvokeKey owns the /invoke flow's keys while it's active, mirroring
// handleKey's (cmd, handled) contract. It must be checked before the
// suggestion box and approvals so confirm's "y" wins over an approval "y".
func (m *model) handleInvokeKey(key string) (tea.Cmd, bool) {
	switch m.invoke.phase {
	case invokePhaseArgs:
		switch key {
		case "enter":
			m.invoke = acceptArg(m.invoke, m.input.Value())
			m.prefillInvokeComposer()
			return nil, true
		case "esc":
			m.invoke = cancelInvoke()
			m.input.Reset()
			return nil, true
		}
	case invokePhaseConfirm:
		switch key {
		case "y":
			tool, args, inform := m.invoke.tool, buildInvokeArguments(m.invoke.fields, m.invoke.values), m.invoke.inform
			m.invoke = cancelInvoke()
			m.input.Reset()
			return action("invoke", func() error {
				_, err := m.client.Invoke(m.ctx, tool, args, inform)
				return err
			}), true
		case "i":
			m.invoke = toggleInform(m.invoke)
			return nil, true
		case "esc":
			m.invoke = cancelInvoke()
			m.input.Reset()
			return nil, true
		}
	}
	return nil, false
}
