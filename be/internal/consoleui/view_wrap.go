package consoleui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// wrapToWidth ANSI-aware word-wraps s to width, hard-wrapping unbroken
// tokens (e.g. long JSON strings) longer than width and preserving
// embedded newlines.
func wrapToWidth(s string, width int) string {
	return ansi.Wrap(s, max(1, width), "")
}

// expandTabs replaces tabs with four spaces. ansi.StringWidth counts "\t" as
// zero-width while terminals advance to the next tab stop, so any tab in
// printed or live-region content desyncs wrap/row accounting (both ours and
// bubbletea's insertAbove) and corrupts the screen.
func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

// fitWidth expands tabs, word-wraps to width, then clips any line still over
// width. ansi.Wrap leaves trailing-whitespace overflow (e.g. glamour's
// background-padding spaces) on the line, and trailing spaces are real cells
// that advance the terminal cursor — so wrap alone can't guarantee the
// line-width invariant printed rows require.
func fitWidth(s string, width int) string {
	width = max(1, width)
	lines := strings.Split(wrapToWidth(expandTabs(s), width), "\n")
	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "")
		}
	}
	return strings.Join(lines, "\n")
}
