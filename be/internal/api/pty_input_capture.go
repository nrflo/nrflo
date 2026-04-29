package api

import (
	"bytes"
	"regexp"
	"strings"
)

const maxInputEntryBytes = 4096

// ansiRE matches ANSI/VT escape sequences (CSI, OSC, and 2-char escapes).
var ansiRE = regexp.MustCompile(
	`\x1b(?:\[[0-9;?]*[A-Za-z]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[A-Za-z])`,
)

// sanitizeInput strips ANSI escape sequences and replaces control bytes with
// human-readable placeholders. \t, \r, \n are preserved for the caller to handle.
func sanitizeInput(data []byte) string {
	clean := ansiRE.ReplaceAll(data, nil)
	var b strings.Builder
	b.Grow(len(clean))
	for i := 0; i < len(clean); i++ {
		c := clean[i]
		switch c {
		case 0x03:
			b.WriteString("^C")
		case 0x04:
			b.WriteString("^D")
		case 0x07:
			b.WriteString("␇")
		case 0x7f:
			b.WriteString("^?")
		case '\t', '\n', '\r':
			b.WriteByte(c)
		default:
			if c < 0x20 {
				b.WriteByte('^')
				b.WriteByte(c + 0x40)
			} else {
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}

// splitLines returns completed lines (split on \r, \n, or \r\n / \n\r as one
// delimiter) and the remaining partial bytes. Each returned line is a copy.
func splitLines(buf []byte) (lines [][]byte, remainder []byte) {
	remainder = buf
	for {
		idx := bytes.IndexAny(remainder, "\r\n")
		if idx < 0 {
			return
		}
		line := make([]byte, idx)
		copy(line, remainder[:idx])
		sep := remainder[idx]
		remainder = remainder[idx+1:]
		// Consume a paired \r\n or \n\r as a single line ending.
		if len(remainder) > 0 {
			next := remainder[0]
			if (sep == '\r' && next == '\n') || (sep == '\n' && next == '\r') {
				remainder = remainder[1:]
			}
		}
		lines = append(lines, line)
	}
}

// capEntry truncates text to maxInputEntryBytes, appending "…" when over cap.
func capEntry(text string) string {
	if len(text) <= maxInputEntryBytes {
		return text
	}
	return text[:maxInputEntryBytes] + "…"
}

// shouldRecord returns true if text contains non-whitespace content worth storing.
func shouldRecord(text string) bool {
	return strings.TrimSpace(text) != ""
}
