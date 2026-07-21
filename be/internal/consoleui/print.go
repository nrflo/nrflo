package consoleui

import (
	tea "charm.land/bubbletea/v2"
)

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
		cmds = append(cmds, tea.Println(renderMessage(message, width)))
	}
	m.pendingUser = ""
	return cmds
}

// contentWidth returns the content width printed rows and the live region
// wrap to, mirroring the composerBox horizontal padding.
func (m *model) contentWidth() int {
	return max(20, m.width-4)
}
