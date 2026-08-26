// Bracketed-paste (DECSET ?2004) mode tracking for PTY-backed CLI TUIs.
// Detection lives here; the write-side wrapping is in prompt_delivery_retry.go.
package spawner

import "bytes"

// bracketedPasteSet / bracketedPasteReset are the DECSET/DECRST sequences a
// TUI emits to turn bracketed paste mode on and off.
const (
	bracketedPasteSet   = "\x1b[?2004h"
	bracketedPasteReset = "\x1b[?2004l"
)

// scanBracketedPasteMode reports the last bracketed-paste mode change in
// carry+chunk, and whether one was present at all. Both sequences are scanned
// so a TUI that toggles the mode off is honored rather than latched.
func scanBracketedPasteMode(carry, chunk []byte) (mode bool, found bool) {
	buf := chunk
	if len(carry) > 0 {
		buf = append(append(make([]byte, 0, len(carry)+len(chunk)), carry...), chunk...)
	}
	on := bytes.LastIndex(buf, []byte(bracketedPasteSet))
	off := bytes.LastIndex(buf, []byte(bracketedPasteReset))
	if on < 0 && off < 0 {
		return false, false
	}
	return on > off, true
}

// carryTail returns the trailing bytes to prepend to the next chunk so a mode
// sequence split across two reads is still matched. One byte short of the
// sequence length is enough — a full match inside the tail was already seen.
func carryTail(chunk []byte) []byte {
	n := len(bracketedPasteSet) - 1
	if len(chunk) < n {
		n = len(chunk)
	}
	return append(make([]byte, 0, n), chunk[len(chunk)-n:]...)
}
