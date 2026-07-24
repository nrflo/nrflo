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
// m.pendingUser once the optimistic line has landed.
func (m *model) printNewMessages(page MessagePage) []tea.Cmd {
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
		cmds = append(cmds, tea.Println(rendered))
	}
	m.pendingUser = ""
	return cmds
}

// physicalRows counts the physical scrollback rows rendered occupies when
// printed to a terminal of the given width, mirroring bubbletea v2's
// cursedRenderer.insertAbove: one row per "\n"-delimited line, plus one extra
// row for every full multiple of width a line's display width consumes.
// tea.Println scrolls at the full terminal width, not the narrower content
// width rendered was wrapped to, so width here must be the terminal width.
func physicalRows(rendered string, width int) int {
	if width <= 0 {
		width = 1
	}
	lines := strings.Split(rendered, "\n")
	rows := len(lines)
	for _, line := range lines {
		rows += ansi.StringWidth(line) / width
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
