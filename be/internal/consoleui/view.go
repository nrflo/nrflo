package consoleui

import (
	"strconv"
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
	if m.graph.open {
		// Full-screen flow overlay on the alt buffer: the inline frame and
		// native scrollback underneath stay untouched and restore on close.
		view := tea.NewView(m.graphView())
		view.AltScreen = true
		view.WindowTitle = "nrflo console"
		return view
	}
	chromeSections := []string{}
	if m.questionActive() {
		chromeSections = append(chromeSections, m.questionView())
	} else if len(m.approvals) > 0 {
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
	// The renderer top-anchors a frame SHRINK (chrome would float up with
	// blank rows below), so the frame's height ratchets via m.frameBand:
	// any shrink — live region clearing, the composer losing a row, an
	// approval box closing — is padded back with blank top rows, and only
	// printNewMessages releases band rows, paired with an insert that
	// refills exactly the vacated rows. The band is capped so inserts always
	// keep at least maxPrintRows free rows above the frame: insertAbove can
	// only insert into free rows above the on-screen frame, and a
	// full-height frame desyncs the renderer one row per insert. Run() parks
	// the cursor on the terminal's bottom row before starting the program,
	// so the inline region starts bottom-anchored.
	m.frameNatural = lipgloss.Height(frame)
	m.frameBand = max(m.frameBand, m.frameNatural)
	m.frameBand = min(m.frameBand, max(m.frameNatural, m.height-m.maxPrintRows()))
	if pad := m.frameBand - m.frameNatural; pad > 0 {
		// Padding rows carry a space: a fully empty top row is skipped by the
		// renderer's diff, which then never blanks the rows a shrink vacated.
		frame = strings.Repeat(" \n", pad) + frame
	}
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
// user line and in-flight deltas/thinking, wrapped to content width and
// tail-clipped to liveRegionCap rows (printed rows live in the terminal's
// native scrollback, not here; the working indicator lives in the footer).
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
	content := strings.Join(parts, "\n\n")
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

// footer renders the bottom help line; while a turn runs it carries the
// animated working indicator plus the in-flight tool detail and elapsed time
// (the live region shows no spinner line — this is the single working
// indicator).
func (m *model) footer() string {
	if m.lastErr != "" {
		return errorStyle.Render(" " + truncate(m.lastErr, max(20, m.width-2)))
	}
	if label, show := m.history.indicator(); show {
		return mutedStyle.Render(" " + label)
	}
	if m.notice != "" {
		return mutedStyle.Render(" " + m.notice)
	}
	if m.status == "running" {
		line := " working…" + workingSuffix(m.tool, time.Since(m.tool.Since))
		if m.queuedCount > 0 {
			line += " · queued:" + strconv.Itoa(m.queuedCount)
		}
		line += " · ctrl+c interrupt · ctrl+t graph"
		return m.spin.View() + mutedStyle.Render(truncate(line, max(20, m.width-3)))
	}
	return mutedStyle.Render(" " + "ctrl+t graph")
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

// timePrefixWidth is the printed width of "HH:MM:SS " — the muted local-time
// prefix on every transcript row.
const timePrefixWidth = 9

// timePrefix formats a message's persisted created_at as a local "HH:MM:SS "
// prefix; "" (no prefix) when the timestamp is absent or unparseable.
func timePrefix(createdAt string) string {
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ""
	}
	return t.Local().Format("15:04:05") + " "
}

// renderMessage renders a transcript row for printing: the body (rendered at
// width minus the timestamp column) prefixed on its first line with the muted
// local created_at time, continuation lines indented to align. Rows without a
// parseable timestamp — and terminals too narrow for the column — render the
// body alone at full width. Every line stays tab-free and within width:
// bubbletea's insertAbove row math desyncs otherwise (ansi.StringWidth counts
// "\t" as 0 while the terminal advances to the next tab stop) and ghost rows
// of the live frame leak into native scrollback.
func renderMessage(message Message, width int) string {
	prefix := timePrefix(message.CreatedAt)
	if prefix == "" || width-timePrefixWidth < 20 {
		return renderMessageBody(message, width)
	}
	body := renderMessageBody(message, width-timePrefixWidth)
	if body == "" {
		return ""
	}
	pad := strings.Repeat(" ", timePrefixWidth)
	lines := strings.Split(body, "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = mutedStyle.Render(prefix) + lines[i]
		} else if lines[i] != "" {
			lines[i] = pad + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func renderMessageBody(message Message, width int) string {
	switch message.Category {
	case "user_input":
		return userStyle.Render(fitWidth(message.Content, width))
	case "tool", "tool_use", "tool_result", "subagent":
		return mutedStyle.Render(toolCard(message.Content, width))
	case "thinking":
		return mutedStyle.Italic(true).Render(fitWidth("thinking · "+message.Content, width))
	case "system_notice":
		return ""
	case "task_notification":
		return mutedStyle.Render(fitWidth(collapseTaskNotification(message.Content), width))
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
