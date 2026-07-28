package consoleui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

var (
	accent      = lipgloss.Color("63")
	muted       = lipgloss.Color("244")
	good        = lipgloss.Color("42")
	warn        = lipgloss.Color("214")
	bad         = lipgloss.Color("196")
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	mutedStyle  = lipgloss.NewStyle().Foreground(muted)
	// Role is conveyed by color alone — no "you"/"assistant" header rows:
	// user input light blue, assistant near-white (distinct from the dim-gray
	// tool/thinking rows).
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("254"))
	errorStyle     = lipgloss.NewStyle().Foreground(bad)
	approvalBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(warn).Padding(0, 1)
	composerBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
)

// assistantGlamour is DarkStyleConfig with the document color lifted to match
// assistantStyle, so glamour body text doesn't read as the same gray as tool
// rows.
var assistantGlamour = func() glamouransi.StyleConfig {
	cfg := styles.DarkStyleConfig
	color := "254"
	cfg.Document.Color = &color
	return cfg
}()

func (m *model) View() tea.View {
	if !m.ready {
		view := tea.NewView("Starting nrflo console…")
		view.AltScreen = true
		return view
	}
	chromeSections := []string{}
	if len(m.approvals) > 0 {
		chromeSections = append(chromeSections, m.approvalView())
	}
	if m.suggestionsOpen() {
		chromeSections = append(chromeSections, m.suggestionView())
	}
	if m.invoke.active {
		chromeSections = append(chromeSections, m.invokeView())
	}
	chromeSections = append(chromeSections, composerBox.Width(max(1, m.width-2)).Render(m.input.View()), m.statusBar(), m.footer())
	chrome := clampChrome(chromeSections, m.height)
	budget := m.height - lipgloss.Height(chrome)
	sections := []string{}
	if live := m.liveRegionView(budget); live != "" {
		sections = append(sections, live)
	}
	sections = append(sections, chrome)
	frame := lipgloss.JoinVertical(lipgloss.Left, sections...)
	if h := lipgloss.Height(frame); h > m.height {
		lines := strings.Split(frame, "\n")
		frame = strings.Join(lines[h-m.height:], "\n")
	}
	// The frame is never padded: insertAbove can only insert printed rows into
	// the free rows above the frame, so a padded full-height frame desyncs the
	// renderer one row per insert. Run() parks the cursor on the terminal's
	// bottom row before starting the program, so the inline region — and with
	// it the chrome — starts bottom-anchored and every insert re-anchors it.
	view := tea.NewView(frame)
	view.AltScreen = false
	view.WindowTitle = "nrflo console"
	return view
}

// liveRegionCap bounds the live region to a short tail regardless of terminal
// height. tea.Println inserts scroll the on-screen frame up before the
// post-print repaint flushes (the renderer flushes on a frame ticker, inserts
// write immediately), so any live-region row can be pushed into native
// scrollback permanently; keeping the region small keeps headroom above the
// frame larger than a print chunk (see maxPrintRows) so that never happens.
const liveRegionCap = 12

// liveRegionView renders the bounded managed region: the optimistic pending
// user line, in-flight deltas, thinking, and the working spinner, wrapped to
// content width and tail-clipped to liveRegionCap rows (printed rows live in
// the terminal's native scrollback, not here).
//
// The section's height ratchets via m.liveBand and shrinks only at print time
// (printNewMessages releases the band alongside an insert of at least as many
// rows): the inline renderer top-anchors a frame shrink, so any shrink not
// immediately refilled by an insert would lift the chrome off the terminal
// bottom. Shorter content pads with leading blank lines up to the band.
func (m *model) liveRegionView(maxLines int) string {
	maxLines = min(maxLines, liveRegionCap)
	if maxLines < 1 {
		return ""
	}
	parts := make([]string, 0, len(m.deltas)+2)
	if m.pendingUser != "" {
		parts = append(parts, userStyle.Render(fitWidth(m.pendingUser, m.contentWidth())))
	}
	for _, id := range m.deltaOrder {
		if text := m.deltas[id]; text != "" {
			parts = append(parts, assistantStyle.Render(fitWidth(text, m.contentWidth())))
		}
	}
	if m.thinking != "" {
		parts = append(parts, mutedStyle.Italic(true).Render(fitWidth("thinking · "+m.thinking, m.contentWidth())))
	}
	if m.status == "running" {
		line := " working…" + workingSuffix(m.tool, time.Since(m.tool.Since))
		parts = append(parts, m.spin.View()+mutedStyle.Render(truncate(line, max(20, m.contentWidth()-2))))
	}
	content := strings.Join(parts, "\n\n")
	lines := []string{}
	if content != "" {
		lines = strings.Split(content, "\n")
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
	}
	m.liveBand = min(maxLines, max(m.liveBand, len(lines)))
	if m.liveBand == 0 {
		return ""
	}
	for len(lines) < m.liveBand {
		lines = append([]string{""}, lines...)
	}
	return strings.Join(lines, "\n")
}

func (m *model) footer() string {
	help := "enter send · ctrl+d detach · ctrl+x close"
	if m.status == "running" {
		help = "working… · ctrl+c interrupt · ctrl+d detach · ctrl+x close"
	}
	if m.lastErr != "" {
		return errorStyle.Render(" " + truncate(m.lastErr, max(20, m.width-2)))
	}
	if label, show := m.history.indicator(); show {
		return mutedStyle.Render(" " + label)
	}
	if m.notice != "" {
		return mutedStyle.Render(" " + m.notice)
	}
	return mutedStyle.Render(" " + help)
}

func (m *model) approvalView() string {
	a := m.approvals[0]
	detail := a.Command
	if detail == "" {
		detail = a.Reason
	}
	text := lipgloss.NewStyle().Bold(true).Foreground(warn).Render("Approval required") +
		"  " + truncate(detail, max(20, m.width-28)) + "  " +
		lipgloss.NewStyle().Bold(true).Render("[y] allow  [a] always  [n] deny")
	return approvalBox.Width(max(1, m.width-2)).Render(text)
}

func (m *model) resize(width, height int) {
	m.width, m.height, m.ready = width, height, true
	m.input.SetWidth(max(10, width-6))
}

// clampChrome guarantees the chrome block never exceeds maxHeight rows.
// Optional sections (approvals/suggestions/invoke, prepended before the
// mandatory composer/status/footer tail) are dropped front-to-back first;
// if the mandatory tail alone still exceeds maxHeight, the block is
// tail-truncated so the bottom-most rows (footer, then status, then the
// tail of the composer) stay visible.
func clampChrome(sections []string, maxHeight int) string {
	if maxHeight < 1 {
		maxHeight = 1
	}
	chrome := lipgloss.JoinVertical(lipgloss.Left, sections...)
	for len(sections) > 1 && lipgloss.Height(chrome) > maxHeight {
		sections = sections[1:]
		chrome = lipgloss.JoinVertical(lipgloss.Left, sections...)
	}
	if h := lipgloss.Height(chrome); h > maxHeight {
		lines := strings.Split(chrome, "\n")
		chrome = strings.Join(lines[h-maxHeight:], "\n")
	}
	return chrome
}

// renderMessage renders a transcript row for printing. Every branch expands
// tabs and hard-wraps to width: printed lines must never exceed the terminal
// width or contain tabs, or bubbletea's insertAbove row math desyncs
// (ansi.StringWidth counts "\t" as 0 while the terminal advances to the next
// tab stop) and ghost rows of the live frame leak into native scrollback.
func renderMessage(message Message, width int) string {
	switch message.Category {
	case "user_input":
		return userStyle.Render(fitWidth(message.Content, width))
	case "tool", "tool_use", "tool_result":
		return mutedStyle.Render(fitWidth("tool · "+prettyToolContent(message.Content), width))
	case "thinking":
		return mutedStyle.Italic(true).Render(fitWidth("thinking · "+message.Content, width))
	default:
		renderer, err := glamour.NewTermRenderer(glamour.WithStyles(assistantGlamour), glamour.WithWordWrap(width))
		if err == nil {
			if rendered, renderErr := renderer.Render(message.Content); renderErr == nil {
				return fitWidth(strings.TrimSpace(rendered), width)
			}
		}
		return assistantStyle.Render(fitWidth(message.Content, width))
	}
}

func truncate(value string, width int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if lipgloss.Width(value) <= width {
		return value
	}
	return lipgloss.NewStyle().MaxWidth(max(1, width-1)).Render(value) + "…"
}
