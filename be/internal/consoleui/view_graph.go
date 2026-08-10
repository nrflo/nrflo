package consoleui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"be/internal/types"
)

// graphView renders the full-screen flow overlay: header, stats rollup, the
// session tree, and a key-help footer, scrolled to m.graph.scroll and clamped
// to the terminal height.
func (m *model) graphView() string {
	width, height := max(20, m.width), max(4, m.height)
	header := headerStyle.Render(" Session flow ") + mutedStyle.Render(truncate(
		m.detail.ProjectID+" · "+m.detail.Engine+"/"+m.detail.Model+graphUpdatedSuffix(m.graph), width-15))
	if m.graph.err != "" {
		header += "\n" + errorStyle.Render(" "+truncate(m.graph.err, width-2))
	}

	body := []string{}
	if m.graph.haveFlow {
		body = append(body, graphStatsLines(m.graph.stats, width)...)
		body = append(body, "")
		body = append(body, renderFlowTree(m.graph.flow, width, time.Now())...)
		if m.graph.flow.Truncated {
			body = append(body, mutedStyle.Render(" … graph truncated (depth/node cap)"))
		}
	} else if m.graph.err == "" {
		body = append(body, mutedStyle.Render(" loading…"))
	}

	footer := mutedStyle.Render(" ctrl+t/esc back · ↑/↓ scroll · r refresh")

	chromeRows := lipgloss.Height(header) + 1
	visible := max(1, height-chromeRows)
	maxScroll := max(0, len(body)-visible)
	m.graph.scroll = min(m.graph.scroll, maxScroll)
	window := body[m.graph.scroll:min(len(body), m.graph.scroll+visible)]
	if maxScroll > 0 {
		footer += mutedStyle.Render(" · " + strconv.Itoa(m.graph.scroll+1) + "-" +
			strconv.Itoa(m.graph.scroll+len(window)) + "/" + strconv.Itoa(len(body)))
	}

	rows := append([]string{header}, window...)
	for len(rows) < height-1 {
		rows = append(rows, "")
	}
	rows = append(rows, footer)
	return strings.Join(rows, "\n")
}

func graphUpdatedSuffix(g graphState) string {
	if g.fetchedAt.IsZero() {
		return ""
	}
	return " · updated " + g.fetchedAt.Local().Format("15:04:05")
}

// graphStatsLines renders the cost/token rollup and the top tool calls.
func graphStatsLines(stats types.SessionStatsResponse, width int) []string {
	lines := []string{mutedStyle.Render(truncate(fmt.Sprintf(
		" cost ~$%.2f (subtree $%.2f) · tokens %s (subtree %s)",
		stats.SelfCostUSD, stats.SubtreeCostUSD,
		compactCount(stats.SelfTokens), compactCount(stats.SubtreeTokens)), width-1))}
	if len(stats.ToolCalls) > 0 {
		entries := append([]types.ToolCallDistributionEntry(nil), stats.ToolCalls...)
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Success+entries[i].Error > entries[j].Success+entries[j].Error
		})
		parts := []string{}
		for i, e := range entries {
			if i >= 6 {
				parts = append(parts, "…")
				break
			}
			part := e.ToolName + " ×" + strconv.Itoa(e.Success+e.Error)
			if e.Error > 0 {
				part += " (" + strconv.Itoa(e.Error) + " err)"
			}
			parts = append(parts, part)
		}
		lines = append(lines, mutedStyle.Render(truncate(" tools: "+strings.Join(parts, " · "), width-1)))
	}
	return lines
}

// compactCount renders token counts as 1.2k/3.4M above 1000.
func compactCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return strconv.FormatInt(n, 10)
	}
}

// renderFlowTree renders the session flow as an ASCII tree rooted at the
// root session, one line per node, children ordered by edge appearance.
func renderFlowTree(flow types.SessionFlowResponse, width int, now time.Time) []string {
	nodesByID := map[string]types.SessionFlowNode{}
	for _, n := range flow.Nodes {
		nodesByID[n.SessionID] = n
	}
	children := map[string][]types.SessionFlowEdge{}
	for _, e := range flow.Edges {
		children[e.FromSessionID] = append(children[e.FromSessionID], e)
	}

	lines := []string{}
	var walk func(id, edgeKind, prefix, childPrefix string)
	seen := map[string]bool{}
	walk = func(id, edgeKind, prefix, childPrefix string) {
		node, ok := nodesByID[id]
		if !ok {
			node = types.SessionFlowNode{SessionID: id}
		}
		lines = append(lines, truncate(prefix+flowNodeLine(node, edgeKind, now), width-1))
		if seen[id] {
			return
		}
		seen[id] = true
		kids := children[id]
		for i, edge := range kids {
			connector, nextPrefix := "├─ ", childPrefix+"│  "
			if i == len(kids)-1 {
				connector, nextPrefix = "└─ ", childPrefix+"   "
			}
			walk(edge.ToSessionID, edge.Kind, childPrefix+connector, nextPrefix)
		}
	}
	walk(flow.RootSessionID, "", " ", " ")
	return lines
}

// flowNodeLine formats one node: status glyph, label, model, elapsed,
// context%, status text, and the edge kind that reached it.
func flowNodeLine(node types.SessionFlowNode, edgeKind string, now time.Time) string {
	label := node.AgentType
	if label == "" {
		label = node.Kind
	}
	if label == "" {
		label = shortID(node.SessionID)
	}
	parts := []string{flowStatusGlyph(node.Status) + " " + label}
	if node.ModelID != "" {
		parts = append(parts, node.ModelID)
	}
	if elapsed := flowElapsed(node, now); elapsed != "" {
		parts = append(parts, elapsed)
	}
	if node.ContextLeft != nil {
		parts = append(parts, "ctx "+strconv.Itoa(*node.ContextLeft)+"%")
	}
	status := node.Status
	if node.Result != "" && node.Result != "pass" {
		status += "/" + node.Result
	}
	if status != "" {
		parts = append(parts, status)
	}
	line := strings.Join(parts, " · ")
	if edgeKind != "" {
		line += " " + mutedStyle.Render("["+edgeKind+"]")
	}
	return line
}

// flowStatusGlyph maps a session status to a colored glyph.
func flowStatusGlyph(status string) string {
	switch status {
	case "running", "user_interactive":
		return lipgloss.NewStyle().Foreground(warn).Render("●")
	case "completed", "interactive_completed", "continued":
		return lipgloss.NewStyle().Foreground(good).Render("✓")
	case "failed", "timeout":
		return errorStyle.Render("✗")
	default:
		return mutedStyle.Render("○")
	}
}

// flowElapsed renders the node's wall time: ended-started when terminal,
// now-started while running; "" without a parseable start.
func flowElapsed(node types.SessionFlowNode, now time.Time) string {
	start, err := time.Parse(time.RFC3339Nano, node.StartedAt)
	if err != nil {
		return ""
	}
	end := now
	if t, err := time.Parse(time.RFC3339Nano, node.EndedAt); err == nil {
		end = t
	}
	d := end.Sub(start)
	if d < 0 {
		return ""
	}
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return strconv.Itoa(int(d.Seconds())) + "s"
	}
}

// shortID compacts a session uuid for display.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
