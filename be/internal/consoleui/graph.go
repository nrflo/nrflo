package consoleui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"be/internal/types"
)

// graphState is the ctrl+t flow-graph overlay: a full-screen alt-screen view
// of the session's delegation/consult/sub-workflow tree (GET /sessions/{sid}/
// flow + /stats). While open, transcript printing is suppressed (pages are
// dropped; closing reloads the tail and the printedTotal high-water mark
// prints only what was missed) and keys never reach the composer.
type graphState struct {
	open      bool
	haveFlow  bool
	flow      types.SessionFlowResponse
	stats     types.SessionStatsResponse
	err       string
	scroll    int
	fetchedAt time.Time
}

// graphMsg carries one flow+stats fetch result. A fetch error keeps any
// previously rendered graph and surfaces the error in the overlay header.
type graphMsg struct {
	flow  types.SessionFlowResponse
	stats types.SessionStatsResponse
	err   error
}

// loadGraph fetches the flow graph and stats in one command; stats errors are
// non-fatal (the tree renders without the rollup).
func (m *model) loadGraph() tea.Cmd {
	return func() tea.Msg {
		flow, err := m.client.Flow(m.ctx)
		if err != nil {
			return graphMsg{err: err}
		}
		stats, statsErr := m.client.SessionStats(m.ctx)
		_ = statsErr
		return graphMsg{flow: flow, stats: stats}
	}
}

// handleGraphKey owns every keypress while the overlay is open — nothing
// falls through to the composer.
func (m *model) handleGraphKey(key string) (tea.Cmd, bool) {
	switch key {
	case "ctrl+t", "esc", "q":
		m.graph.open = false
		// Reload the tail so rows that arrived while the overlay was open
		// print now (newMessagesToPrint's high-water mark skips the rest).
		return m.loadHistory(), true
	case "r":
		return m.loadGraph(), true
	case "up", "k":
		m.graph.scroll = max(0, m.graph.scroll-1)
		return nil, true
	case "down", "j":
		m.graph.scroll++
		return nil, true
	case "pgup":
		m.graph.scroll = max(0, m.graph.scroll-max(1, m.height-6))
		return nil, true
	case "pgdown":
		m.graph.scroll += max(1, m.height-6)
		return nil, true
	case "ctrl+c":
		return tea.Quit, true
	}
	return nil, true
}

func (m *model) applyGraph(msg graphMsg) {
	if msg.err != nil {
		m.graph.err = msg.err.Error()
		return
	}
	m.graph.err = ""
	m.graph.flow, m.graph.stats = msg.flow, msg.stats
	m.graph.haveFlow = true
	m.graph.fetchedAt = time.Now()
}
