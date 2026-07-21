package consoleui

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// wrapToWidth ANSI-aware word-wraps s to width, hard-wrapping unbroken
// tokens (e.g. long JSON strings) longer than width and preserving
// embedded newlines.
func wrapToWidth(s string, width int) string {
	return ansi.Wrap(s, max(1, width), "")
}

// prettyToolContent splits content on the first " → " delimiter and, if
// the right-hand side is valid JSON, pretty-prints it with two-space
// indentation. Best-effort: non-JSON or missing-delimiter content passes
// through unchanged.
func prettyToolContent(content string) string {
	left, right, ok := strings.Cut(content, " → ")
	if !ok {
		return content
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(strings.TrimSpace(right)), "", "  "); err != nil {
		return content
	}
	return left + " → " + buf.String()
}
