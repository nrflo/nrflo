package consoleui

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
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

// prettyToolContent splits content on the first " → " delimiter and, if
// the right-hand side is valid JSON, pretty-prints it with two-space
// indentation; otherwise, if it is valid XML, indents it via a
// token round-trip. Best-effort: non-JSON/non-XML or missing-delimiter
// content passes through unchanged.
func prettyToolContent(content string) string {
	left, right, ok := strings.Cut(content, " → ")
	if !ok {
		return content
	}
	trimmed := strings.TrimSpace(right)
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(trimmed), "", "  "); err == nil {
		return left + " → " + buf.String()
	}
	if pretty, ok := prettyXML(trimmed); ok {
		return left + " → " + pretty
	}
	return content
}

// prettyXML re-encodes s with two-space indentation via an XML token
// round-trip. Returns ok=false on any decode/encode error or if s contains
// no element (so plain CharData, which re-encoding would escape, e.g.
// ">" -> "&gt;", passes through unchanged).
func prettyXML(s string) (string, bool) {
	dec := xml.NewDecoder(strings.NewReader(s))
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	sawElement := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false
		}
		if _, isStart := tok.(xml.StartElement); isStart {
			sawElement = true
		}
		if err := enc.EncodeToken(tok); err != nil {
			return "", false
		}
	}
	if err := enc.Flush(); err != nil {
		return "", false
	}
	if !sawElement {
		return "", false
	}
	return buf.String(), true
}
