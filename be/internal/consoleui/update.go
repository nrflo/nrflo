package consoleui

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	wasBusy := m.busy()
	var commands []tea.Cmd
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		if !m.historyPrinted {
			m.historyPrinted = true
			commands = append(commands, m.printNewMessages(m.initialPage))
		}
	case tea.KeyPressMsg:
		if cmd, handled := m.handleKey(msg); handled {
			return m, tea.Batch(cmd, m.tickOnBusy(wasBusy))
		}
	case spinner.TickMsg:
		// The tick chain lives only while a turn runs or delegated workers
		// are active; it restarts on the next idle→busy transition via
		// tickOnBusy.
		if m.busy() {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil
	case streamUpdate:
		m.applyStream(msg)
		commands = append(commands, waitForStream(m.events))
		if msg.Connected != nil && *msg.Connected {
			commands = append(commands, m.syncState(), m.loadBgCount(), m.loadDelegateCount())
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
		if bgRelevant(msg.Events) {
			commands = append(commands, m.loadBgCount())
			if m.graph.open {
				commands = append(commands, m.loadGraph())
			}
		}
		if delegateRelevant(msg.Events) {
			commands = append(commands, m.loadDelegateCount())
		}
	case graphMsg:
		m.applyGraph(msg)
	case skillsMsg:
		if msg.err == nil {
			m.skills = msg.skills
		}
	case toolsMsg:
		if msg.err == nil {
			m.tools = msg.tools
		}
	case bgCountMsg:
		if msg.err == nil {
			m.bgRunning = msg.count
		}
	case delegateCountMsg:
		if msg.err == nil {
			m.delegating = msg.count
		}
	case historyMsg:
		if msg.err != nil {
			m.lastErr = msg.err.Error()
		} else if !m.graph.open {
			// Printing is suppressed while the alt-screen graph overlay is
			// open (tea.Println targets the inline region); closing the
			// overlay reloads the tail and the printedTotal high-water mark
			// prints only what was missed.
			commands = append(commands, m.printNewMessages(msg.page))
		}
	case syncMsg:
		if msg.err != nil {
			m.lastErr = msg.err.Error()
		} else {
			if !m.graph.open {
				commands = append(commands, m.printNewMessages(msg.page))
			}
			m.applySync(msg.detail)
			m.lastErr = ""
		}
	case actionMsg:
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			if msg.action == "answer" {
				// Let the user retry the card instead of leaving it stuck "sent".
				m.qa.sent = false
			}
		} else {
			m.lastErr = ""
			if msg.action == "close" {
				return m, tea.Quit
			}
			if msg.action == "approval" && len(m.approvals) > 0 {
				m.approvals = m.approvals[1:]
				m.syncQuestion()
			}
		}
	case sendResultMsg:
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = "idle"
			m.pendingUser = ""
			commands = append(commands, m.loadHistory())
		} else {
			m.lastErr = ""
			if msg.queued {
				m.queuedCount++
				m.notice = "queued — delivers when the current turn ends"
			}
		}
	}

	commands = append(commands, m.tickOnBusy(wasBusy))

	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(message)
	commands = append(commands, cmd)
	if m.input.Value() != before {
		m.skillIndex = 0
		m.skillsDismissed = false
		m.skillDetails = false
	}
	return m, tea.Batch(commands...)
}

// busy reports whether the footer spinner should animate: a turn is running
// or delegated workers are still active.
func (m *model) busy() bool {
	return m.status == "running" || m.delegating > 0
}

// tickOnBusy starts the spinner tick chain when this message flipped the
// model to busy (send, stream event, reconnect sync, or delegate count).
func (m *model) tickOnBusy(wasBusy bool) tea.Cmd {
	if !wasBusy && m.busy() {
		return m.spin.Tick
	}
	return nil
}

func (m *model) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	m.notice = ""
	key := msg.Keystroke()
	if m.graph.open {
		return m.handleGraphKey(key)
	}
	if m.invoke.active {
		return m.handleInvokeKey(key)
	}
	if m.handleSuggestionKey(key) {
		return nil, true
	}
	if m.handleHistoryKey(key) {
		return nil, true
	}
	if m.questionActive() {
		if cmd, handled := m.handleQuestionKey(key); handled {
			return cmd, true
		}
	} else if len(m.approvals) > 0 {
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
	switch key {
	case "ctrl+c":
		if m.status == "running" {
			return action("interrupt", func() error { return m.client.Interrupt(m.ctx) }), true
		}
		return tea.Quit, true
	case "ctrl+d":
		return tea.Quit, true
	case "ctrl+x":
		return action("close", func() error { return m.client.Close(m.ctx) }), true
	case "ctrl+y":
		toggled := !m.detail.Yolo
		return action("yolo", func() error { return m.client.SetYolo(m.ctx, toggled) }), true
	case "ctrl+t":
		m.graph.open = true
		m.graph.scroll = 0
		return m.loadGraph(), true
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return nil, true
		}
		m.input.Reset()
		m.skillIndex = 0
		m.skillsDismissed = false
		m.skillDetails = false
		m.pendingUser = text
		m.history = m.history.record(text)
		m.status = "running"
		return func() tea.Msg {
			queued, err := m.client.Send(m.ctx, text)
			return sendResultMsg{queued: queued, err: err}
		}, true
	}
	return nil, false
}
