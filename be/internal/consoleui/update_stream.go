package consoleui

import (
	"context"
	"errors"
)

func (m *model) applyStream(update streamUpdate) {
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
		case "console_chat.session_approvals":
			// Always the full list (never a delta) — see console/chat_events.go.
			m.detail.SessionApprovals = eventStrings(event, "tools")
		case "console_chat.error":
			m.lastErr = eventString(event, "text")
		case "agent.context_updated":
			value := eventInt(event, "context_left")
			m.detail.ContextLeft = &value
		case "session.cost_updated":
			// Ignore updates for models with no seeded pricing — the BE emits
			// cost_estimate=0/pricing_known=false there, which is unknown, not free.
			if eventBool(event, "pricing_known") {
				value := eventFloat(event, "cost_estimate")
				m.detail.CostEstimate = &value
			}
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
