package consoleui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

func (m *model) recordTurnTiming(state string, now time.Time) {
	if state == "running" && m.turnStarted.IsZero() {
		m.turnStarted = now
	}
	if state == "idle" && !m.turnStarted.IsZero() {
		m.turnSummaries = append(m.turnSummaries, "Worked for "+formatElapsed(now.Sub(m.turnStarted))+" · "+now.Local().Format("15:04"))
		m.turnStarted = time.Time{}
	}
}

// Fetch final persisted messages before printing the turn's timing line.
func (m *model) loadTurnHistory() tea.Cmd {
	summaries := m.turnSummaries
	m.turnSummaries = nil
	return func() tea.Msg {
		page, err := m.client.TailMessages(m.ctx, historyWindowSize)
		return historyMsg{page: page, err: err, summaries: summaries}
	}
}
