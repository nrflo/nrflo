package consoleui

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var commands []tea.Cmd
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case tea.KeyPressMsg:
		if cmd, handled := m.handleKey(msg); handled {
			return m, cmd
		}
	case streamUpdate:
		m.applyStream(msg)
		commands = append(commands, waitForStream(m.events))
		if msg.Connected != nil && *msg.Connected {
			commands = append(commands, m.syncState())
		}
		if needsHistory(msg.Events) {
			commands = append(commands, m.loadHistory())
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
		m.input, cmd = m.input.Update(message)
		commands = append(commands, cmd)
		if !keyMessage {
			m.viewport, cmd = m.viewport.Update(message)
			commands = append(commands, cmd)
		}
	}
	return m, tea.Batch(commands...)
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
	switch key {
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
		m.appendOptimisticMessage(Message{Content: text, Category: "user_input"})
		m.status = "running"
		m.refreshTranscript()
		return action("send", func() error { return m.client.Send(m.ctx, text) }), true
	}
	return nil, false
}

func (m *model) applyStream(update streamUpdate) {
	approvalCount := len(m.approvals)
	if update.Connected != nil {
		m.connected = *update.Connected
		if m.connected {
			m.lastErr = ""
		}
	}
	if update.Err != nil && !errors.Is(update.Err, context.Canceled) {
		m.lastErr = update.Err.Error()
	}
	for _, event := range update.Events {
		if event.SessionID != "" && event.SessionID != m.detail.SessionID {
			continue
		}
		switch event.Type {
		case "console_chat.delta":
			id := eventString(event, "item_id")
			m.appendDelta(id, eventString(event, "text"))
		case "console_chat.thinking":
			id := eventString(event, "item_id")
			if id != m.thinkingID {
				m.thinkingID = id
				m.thinking = ""
			}
			m.thinking = trimDeltaTail(m.thinking + eventString(event, "text"))
		case "console_chat.turn":
			m.status = eventString(event, "state")
			if m.status == "idle" {
				m.thinking = ""
				m.thinkingID = ""
			}
		case "console_chat.approval_request":
			m.approvals = append(m.approvals, Approval{
				ID: eventString(event, "approval_id"), Kind: eventString(event, "kind"),
				Command: eventString(event, "command"), Cwd: eventString(event, "cwd"),
				Reason: eventString(event, "reason"),
			})
		case "console_chat.approval_resolved":
			m.removeApproval(eventString(event, "approval_id"))
		case "console_chat.error":
			m.lastErr = eventString(event, "text")
		case "agent.context_updated":
			value := eventInt(event, "context_left")
			m.detail.ContextLeft = &value
		}
	}
	if len(update.Events) > 0 {
		if approvalCount != len(m.approvals) && m.ready {
			m.resize(m.width, m.height)
		} else {
			m.refreshTranscript()
		}
	}
}

func needsHistory(events []Event) bool {
	for _, event := range events {
		if event.Type == "messages.updated" {
			return true
		}
	}
	return false
}

func (m *model) removeApproval(id string) {
	for i := range m.approvals {
		if m.approvals[i].ID == id {
			m.approvals = append(m.approvals[:i], m.approvals[i+1:]...)
			return
		}
	}
}
