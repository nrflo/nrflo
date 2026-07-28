package consoleui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// runningTool is the live in-flight tool line, fed by
// console_chat.tool_started/tool_finished and cleared on turn idle/error.
type runningTool struct {
	Detail string
	Since  time.Time
}

// workingSuffix renders the spinner-line suffix for an in-flight tool, e.g.
// " · [Bash] make test · 42s". Empty when no tool is running.
func workingSuffix(tool runningTool, elapsed time.Duration) string {
	if tool.Detail == "" {
		return ""
	}
	return " · " + tool.Detail + " · " + formatElapsed(elapsed)
}

// formatElapsed renders a compact duration: 42s, 3m10s, 1h05m.
func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int(d.Seconds())
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
	default:
		return fmt.Sprintf("%dh%02dm", seconds/3600, (seconds%3600)/60)
	}
}

// bgCountMsg carries the refreshed count of active workflow instances for the
// chat's project.
type bgCountMsg struct {
	count int
	err   error
}

// loadBgCount fetches the project's active workflow-instance count — called
// on WS connect and again whenever a workflow-lifecycle event arrives, never
// on a timer (WebSocket-only realtime).
func (m *model) loadBgCount() tea.Cmd {
	projectID := m.detail.ProjectID
	return func() tea.Msg {
		count, err := m.client.ActiveWorkflowCount(m.ctx, projectID)
		return bgCountMsg{count: count, err: err}
	}
}

// bgRelevant reports whether any event can change the active-workflow count.
func bgRelevant(events []Event) bool {
	for _, e := range events {
		switch e.Type {
		case "workflow.updated", "orchestration.started", "orchestration.completed",
			"delegate.started", "delegate.completed", "delegate.failed":
			return true
		}
	}
	return false
}
