package usagelimits

import "regexp"

// Precompiled patterns for ANSI escape code stripping.
// Ported from scripts/usage-limits.sh strip_ansi().
var (
	// Pass 1: Replace cursor-right moves (ESC[NC) with spaces
	cursorRightRe = regexp.MustCompile("\x1b\\[([0-9]+)C")

	// Pass 2: SGR codes (ESC[...m), OSC sequences (ESC]...BEL),
	// charset designators (ESC(N / ESC)N), SI (\x0f), CR (\r)
	sgrRe     = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")
	oscRe     = regexp.MustCompile("\x1b\\][^\x07]*\x07")
	charsetRe = regexp.MustCompile("\x1b[()][0-9A-B]")

	// Pass 3: Device mode sequences (ESC[?...h/l etc)
	deviceModeRe = regexp.MustCompile("\x1b\\[\\?[0-9;]*[a-zA-Z]")
)

// stripANSI removes ANSI escape sequences from terminal output.
func stripANSI(input []byte) string {
	// Pass 1: cursor-right → spaces
	result := cursorRightRe.ReplaceAllFunc(input, func(match []byte) []byte {
		sub := cursorRightRe.FindSubmatch(match)
		if len(sub) < 2 {
			return []byte(" ")
		}
		n := 0
		for _, b := range sub[1] {
			n = n*10 + int(b-'0')
		}
		if n <= 0 {
			n = 1
		}
		if n > 200 {
			n = 200
		}
		spaces := make([]byte, n)
		for i := range spaces {
			spaces[i] = ' '
		}
		return spaces
	})

	// Pass 2: strip SGR, OSC, charset, SI, CR
	result = sgrRe.ReplaceAll(result, nil)
	result = oscRe.ReplaceAll(result, nil)
	result = charsetRe.ReplaceAll(result, nil)
	result = removeBytes(result, '\x0f')
	result = removeBytes(result, '\r')

	// Pass 3: device mode
	result = deviceModeRe.ReplaceAll(result, nil)

	return string(result)
}

// removeBytes removes all occurrences of a single byte.
func removeBytes(data []byte, old byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		if b != old {
			out = append(out, b)
		}
	}
	return out
}
