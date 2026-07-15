package consoleui

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
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
		if needsHistory(msg.Events) {
			commands = append(commands, m.loadHistory())
		}
	case historyMsg:
		if msg.err != nil {
			m.lastErr = msg.err.Error()
		} else {
			m.messages = msg.messages
			m.deltas = make(map[string]string)
			m.deltaOrder = nil
			m.historyDirty = true
			m.refreshTranscript()
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
			}
		} else {
			m.lastErr = ""
			if msg.action == "approval" && len(m.approvals) > 0 {
				m.approvals = m.approvals[1:]
			}
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(message)
	commands = append(commands, cmd)
	m.input, cmd = m.input.Update(message)
	commands = append(commands, cmd)
	return m, tea.Batch(commands...)
}

func (m *model) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	key := msg.Keystroke()
	if len(m.approvals) > 0 {
		switch key {
		case "y":
			approval := m.approvals[0]
			return action("approval", func() error { return m.client.Approve(m.ctx, approval.ID, "allow") }), true
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
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return nil, true
		}
		m.input.Reset()
		m.messages = append(m.messages, Message{Content: text, Category: "user_input"})
		m.historyDirty = true
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
			if _, exists := m.deltas[id]; !exists {
				m.deltaOrder = append(m.deltaOrder, id)
			}
			m.deltas[id] += eventString(event, "text")
		case "console_chat.thinking":
			id := eventString(event, "item_id")
			if id != m.thinkingID {
				m.thinkingID = id
				m.thinking = ""
			}
			m.thinking += eventString(event, "text")
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
