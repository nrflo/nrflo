package consoleui

import (
	"strings"
)

const (
	historyPageSize   = 200
	historyWindowSize = 1000
	maxDeltaItems     = 64
	maxDeltaBytes     = 128 * 1024
)

// applyDetail seeds live-region-only state (turn status, pending approvals,
// in-flight deltas/thinking) from a freshly-loaded ChatDetail. It never
// touches printed scrollback.
func (m *model) applyDetail(detail ChatDetail) {
	m.detail = detail
	m.status = detail.Turn
	m.approvals = append([]Approval{}, detail.PendingApprovals...)
	m.deltas = make(map[string]string, len(detail.LiveItems))
	m.deltaOrder = m.deltaOrder[:0]
	start := max(0, len(detail.LiveItems)-maxDeltaItems)
	for _, item := range detail.LiveItems[start:] {
		m.deltaOrder = append(m.deltaOrder, item.ID)
		m.deltas[item.ID] = trimDeltaTail(item.Text)
	}
	m.thinking, m.thinkingID = "", ""
	if detail.Thinking != nil {
		m.thinking, m.thinkingID = trimDeltaTail(detail.Thinking.Text), detail.Thinking.ID
	}
}

// applySync re-seeds live state from a WS-reconnect sync. The caller (the
// syncMsg branch in Update) is responsible for printing any new rows in
// page via printNewMessages.
func (m *model) applySync(detail ChatDetail) {
	m.applyDetail(detail)
}

func (m *model) appendDelta(id, text string) {
	if id == "" {
		id = "assistant"
	}
	if _, exists := m.deltas[id]; !exists {
		m.deltaOrder = append(m.deltaOrder, id)
		if len(m.deltaOrder) > maxDeltaItems {
			delete(m.deltas, m.deltaOrder[0])
			m.deltaOrder = m.deltaOrder[1:]
		}
	}
	m.deltas[id] = trimDeltaTail(m.deltas[id] + text)
}

func trimDeltaTail(value string) string {
	if len(value) <= maxDeltaBytes {
		return value
	}
	return strings.ToValidUTF8(value[len(value)-maxDeltaBytes:], "")
}
