package consoleui

import (
	"strings"
	"time"

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
	rows := 0
	for _, message := range toPrint {
		rendered := renderMessage(message, width)
		if rendered == "" {
			// tea.Println with an empty body is a renderer no-op — skip it
			// rather than emit a command that does nothing.
			continue
		}
		for _, chunk := range splitChunks(rendered, m.maxPrintRows()) {
			if chunk == "" {
				// A blank transcript line split into its own chunk: a space
				// keeps the row (insertAbove skips an empty body entirely).
				chunk = " "
			}
			rows += strings.Count(chunk, "\n") + 1
			cmds = append(cmds, tea.Println(chunk))
		}
	}
	m.pendingUser = ""
	if len(cmds) == 0 {
		return nil
	}
	// Release the frame band's padding (up to the printed row count): the
	// resulting frame shrink is refilled exactly by these inserts. The pause
	// lets the frame ticker flush the shrunken frame FIRST — an insert running
	// against the taller on-screen frame would land fine, but the shrink flush
	// after it would float the chrome until the next print.
	if release := min(rows, m.frameBand-m.frameNatural); release > 0 {
		m.frameBand -= release
		cmds = append([]tea.Cmd{printReleasePause}, cmds...)
	}
	return tea.Sequence(cmds...)
}

// printPauseMsg is the no-op message printReleasePause resolves to; Update
// ignores it.
type printPauseMsg struct{}

// printReleasePause delays the Println sequence long enough for the frame
// ticker (60fps) to flush the band-released frame before the inserts run.
var printReleasePause tea.Cmd = func() tea.Msg {
	time.Sleep(50 * time.Millisecond)
	return printPauseMsg{}
}

// chromeAllowance is the worst-case chrome height (composer up to 8 rows +
// border, status bar, footer) reserved when sizing print chunks.
const chromeAllowance = 12

// maxPrintRows bounds a single tea.Println's physical rows. insertAbove can
// only insert into the free rows above the on-screen frame — the excess
// scrolls frame rows into native scrollback permanently — so a chunk must fit
// in the headroom above the live region + chrome.
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

// contentWidth returns the content width printed rows and the live region
// wrap to: the full terminal width minus one column, so an exactly-full line
// can never trip the terminal's deferred-wrap ambiguity.
func (m *model) contentWidth() int {
	return max(20, m.width-1)
}
