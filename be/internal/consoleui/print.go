package consoleui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
)

// printedEntry retains a printed message's source content and the physical
// scrollback rows it contributed at print time, for resize re-measurement.
type printedEntry struct {
	message Message
	rows    int
}

// newMessagesToPrint is the pure print-once/dedupe splitter. printedTotal is
// the DB-absolute high-water mark of rows already printed to scrollback;
// page is the latest tail window with page.Total as its absolute count.
// firstAbs is the absolute index of page.Messages[0]; already-printed rows
// are skipped via startInPage, and a window gap (printedTotal below
// firstAbs, e.g. after a reconnect that missed a large burst) degrades to
// printing the whole tail rather than fabricating a gap. Returns the rows to
// print and the new printedTotal, which only ever advances (max of the
// current watermark and page.Total) so an out-of-order, staler page
// resolving after a newer one can never regress the watermark and cause
// re-printed rows.
func newMessagesToPrint(printedTotal int, page MessagePage) (toPrint []Message, newTotal int) {
	firstAbs := page.Total - len(page.Messages)
	startInPage := printedTotal - firstAbs
	if startInPage < 0 {
		startInPage = 0
	}
	if startInPage > len(page.Messages) {
		startInPage = len(page.Messages)
	}
	newTotal = printedTotal
	if page.Total > newTotal {
		newTotal = page.Total
	}
	return page.Messages[startInPage:], newTotal
}

// printNewMessages renders any messages newly visible in page (per
// newMessagesToPrint) into tea.Println commands so they land in the
// terminal's native scrollback, advances m.printedTotal, and clears
// m.pendingUser once the optimistic line has landed. The Printlns are
// returned as one tea.Sequence: tea.Batch runs commands concurrently, which
// would land chunks (and messages) in scrollback in random order.
func (m *model) printNewMessages(page MessagePage) tea.Cmd {
	toPrint, newTotal := newMessagesToPrint(m.printedTotal, page)
	m.printedTotal = newTotal
	if len(toPrint) == 0 {
		return nil
	}
	width := m.contentWidth()
	cmds := make([]tea.Cmd, 0, len(toPrint))
	for _, message := range toPrint {
		rendered := renderMessage(message, width)
		rows := physicalRows(rendered, m.width)
		m.printedLines += rows
		m.appendPrintedTail(message, rows)
		for _, chunk := range splitChunks(rendered, m.maxPrintRows()) {
			cmds = append(cmds, tea.Println(chunk))
		}
	}
	m.pendingUser = ""
	return tea.Sequence(cmds...)
}

// chromeAllowance is the worst-case chrome height (composer up to 8 rows +
// border, status bar, footer) reserved when sizing print chunks.
const chromeAllowance = 12

// maxPrintRows bounds a single tea.Println's physical rows. insertAbove
// scrolls the screen up by the printed row count against the frame currently
// on screen (the shrunken repaint only flushes on the next frame tick), so a
// chunk must fit in the headroom above the live region + chrome or frame rows
// leak into native scrollback permanently.
func (m *model) maxPrintRows() int {
	return max(1, m.height-liveRegionCap-chromeAllowance)
}

// splitChunks splits rendered into "\n"-delimited groups of at most maxRows
// lines each. renderMessage hard-wraps every line to below terminal width, so
// lines and physical rows are one-to-one.
func splitChunks(rendered string, maxRows int) []string {
	lines := strings.Split(rendered, "\n")
	chunks := make([]string, 0, (len(lines)+maxRows-1)/maxRows)
	for start := 0; start < len(lines); start += maxRows {
		end := min(start+maxRows, len(lines))
		chunks = append(chunks, strings.Join(lines[start:end], "\n"))
	}
	return chunks
}

// physicalRows counts the physical scrollback rows rendered occupies when
// printed to a terminal of the given width, mirroring bubbletea v2's
// cursedRenderer.insertAbove exactly: one row per "\n"-delimited line, plus
// lineWidth/width extra rows only when a line's display width exceeds width
// (insertAbove's `lineWidth > w` guard — an exactly-full line adds nothing).
// tea.Println scrolls at the full terminal width, not the narrower content
// width rendered was wrapped to, so width here must be the terminal width.
func physicalRows(rendered string, width int) int {
	if width <= 0 {
		width = 1
	}
	lines := strings.Split(rendered, "\n")
	rows := len(lines)
	for _, line := range lines {
		if lineWidth := ansi.StringWidth(line); lineWidth > width {
			rows += lineWidth / width
		}
	}
	return rows
}

// appendPrintedTail retains message in the bounded on-screen tail buffer used
// to recompute m.printedLines on resize, evicting the oldest entries once
// the buffer's cumulative physical height covers at least one terminal
// height. This is purely a resize re-measure aid, not a scroll/copy surface:
// completed transcript rows are never held in a viewport.
func (m *model) appendPrintedTail(message Message, rows int) {
	m.printedTail = append(m.printedTail, printedEntry{message: message, rows: rows})
	m.printedTailRows += rows
	for len(m.printedTail) > 1 && m.printedTailRows-m.printedTail[0].rows >= m.maxHeightSeen {
		m.printedTailRows -= m.printedTail[0].rows
		m.printedTail = m.printedTail[1:]
	}
}

// contentWidth returns the content width printed rows and the live region
// wrap to, mirroring the composerBox horizontal padding.
func (m *model) contentWidth() int {
	return max(20, m.width-4)
}
