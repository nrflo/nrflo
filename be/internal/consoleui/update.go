package consoleui

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	wasRunning := m.status == "running"
	var commands []tea.Cmd
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case tea.KeyPressMsg:
		if cmd, handled := m.handleKey(msg); handled {
			return m, tea.Batch(cmd, m.tickOnRunning(wasRunning))
		}
	case spinner.TickMsg:
		// The tick chain lives only while a turn runs; it restarts on the
		// next idle→running transition via tickOnRunning.
		if m.status == "running" {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			m.refreshTranscript()
			return m, cmd
		}
		return m, nil
	case streamUpdate:
		m.applyStream(msg)
		commands = append(commands, waitForStream(m.events))
		if msg.Connected != nil && *msg.Connected {
			commands = append(commands, m.syncState())
			if !m.skillsFetched {
				m.skillsFetched = true
				commands = append(commands, m.loadSkills())
			}
			if !m.toolsFetched {
				m.toolsFetched = true
				commands = append(commands, m.loadTools())
			}
		}
		if needsHistory(msg.Events) {
			commands = append(commands, m.loadHistory())
		}
	case skillsMsg:
		if msg.err == nil {
			m.skills = msg.skills
		}
	case toolsMsg:
		if msg.err == nil {
			m.tools = msg.tools
		}
	case historyMsg:
		if msg.err != nil {
			m.lastErr = msg.err.Error()
		} else {
			m.applyHistory(msg.page, msg.prepend, msg.offset)
		}
	case syncMsg:
		if msg.err != nil {
			m.lastErr = msg.err.Error()
		} else {
			m.applySync(msg.detail, msg.page)
			m.lastErr = ""
		}
	case actionMsg:
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			if msg.action == "send" {
				m.status = "idle"
				if n := len(m.messages); n > 0 && m.messages[n-1].Category == "user_input" {
					m.messages = m.messages[:n-1]
					m.historyDirty = true
					m.refreshTranscript()
				}
				commands = append(commands, m.loadHistory())
			}
		} else {
			m.lastErr = ""
			if msg.action == "close" {
				return m, tea.Quit
			}
			if msg.action == "approval" && len(m.approvals) > 0 {
				m.approvals = m.approvals[1:]
			}
		}
	}

	commands = append(commands, m.tickOnRunning(wasRunning))

	_, keyMessage := message.(tea.KeyPressMsg)
	var cmd tea.Cmd
	if m.searchMode {
		m.search, cmd = m.search.Update(message)
		commands = append(commands, cmd)
		if !keyMessage {
			m.viewport, cmd = m.viewport.Update(message)
			commands = append(commands, cmd)
		}
	} else if m.copyMode {
		m.viewport, cmd = m.viewport.Update(message)
		commands = append(commands, cmd)
	} else {
		before := m.input.Value()
		m.input, cmd = m.input.Update(message)
		commands = append(commands, cmd)
		if m.input.Value() != before {
			m.skillIndex = 0
			m.skillsDismissed = false
			m.skillDetails = false
		}
		if m.ready {
			m.relayout()
		}
		if !keyMessage {
			m.viewport, cmd = m.viewport.Update(message)
			commands = append(commands, cmd)
		}
	}
	return m, tea.Batch(commands...)
}

// tickOnRunning starts the spinner tick chain when this message flipped the
// turn to running (send, stream event, or reconnect sync).
func (m *model) tickOnRunning(wasRunning bool) tea.Cmd {
	if !wasRunning && m.status == "running" {
		return m.spin.Tick
	}
	return nil
}

func (m *model) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	m.notice = ""
	key := msg.Keystroke()
	if m.searchMode {
		switch key {
		case "esc":
			m.searchMode = false
			m.search.Blur()
			m.input.Focus()
			return nil, true
		case "enter":
			m.applySearch()
			m.searchMode = false
			m.search.Blur()
			m.input.Focus()
			return nil, true
		}
		return nil, false
	}
	if m.copyMode {
		switch key {
		case "esc", "ctrl+g":
			m.copyMode = false
			m.input.Focus()
			return nil, true
		case "y":
			m.copyMode = false
			m.input.Focus()
			m.notice = "copied visible transcript"
			return tea.Raw(ansi.SetSystemClipboard(m.visibleTranscript())), true
		case "ctrl+p":
			if m.historyOffset > 0 {
				return m.loadOlder(), true
			}
		}
		return nil, false
	}
	if m.invoke.active {
		return m.handleInvokeKey(key)
	}
	if m.handleSuggestionKey(key) {
		return nil, true
	}
	if len(m.approvals) > 0 {
		switch key {
		case "y":
			approval := m.approvals[0]
			return action("approval", func() error { return m.client.Approve(m.ctx, approval.ID, "allow") }), true
		case "a":
			approval := m.approvals[0]
			return action("approval", func() error { return m.client.Approve(m.ctx, approval.ID, "allow_for_session") }), true
		case "n":
			approval := m.approvals[0]
			return action("approval", func() error { return m.client.Approve(m.ctx, approval.ID, "deny") }), true
		}
	}
	var cmd tea.Cmd
	switch key {
	case "pgup", "pgdown":
		m.viewport, cmd = m.viewport.Update(msg)
		return cmd, true
	case "ctrl+f":
		m.searchMode = true
		m.input.Blur()
		m.search.Focus()
		return nil, true
	case "ctrl+g":
		m.copyMode = true
		m.input.Blur()
		return nil, true
	case "f3":
		if m.searchStatus != "" {
			m.viewport.HighlightNext()
			return nil, true
		}
	case "shift+f3":
		if m.searchStatus != "" {
			m.viewport.HighlightPrevious()
			return nil, true
		}
	case "ctrl+c":
		if m.status == "running" {
			return action("interrupt", func() error { return m.client.Interrupt(m.ctx) }), true
		}
		return tea.Quit, true
	case "ctrl+d":
		return tea.Quit, true
	case "ctrl+x":
		return action("close", func() error { return m.client.Close(m.ctx) }), true
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return nil, true
		}
		m.input.Reset()
		m.skillIndex = 0
		m.skillsDismissed = false
		m.skillDetails = false
		m.appendOptimisticMessage(Message{Content: text, Category: "user_input"})
		m.status = "running"
		m.refreshTranscript()
		return action("send", func() error { return m.client.Send(m.ctx, text) }), true
	}
	return nil, false
}
