// Terminal capability query auto-answers for PTY-backed CLI TUIs: without
// them codex's TUI blocks on its init probes and bails.
package spawner

// respondToTerminalQueries scans a chunk of PTY output for terminal capability
// queries that a real terminal emulator would answer. Returns the concatenated
// canned replies (or nil if no queries were seen). Without this, codex's TUI
// init blocks waiting on these probes and bails after a few seconds. The
// replies advertise a minimal but valid xterm-like terminal:
//
//	\x1b[6n        DSR cursor position    → \x1b[24;80R   (row 24, col 80)
//	\x1b[c         DA primary             → \x1b[?1;2c    (VT100 + advanced video)
//	\x1b[>c        DA secondary           → \x1b[>0;0;0c  (no version info)
//	\x1b[?u        kitty keyboard query   → \x1b[?0u      (no kitty flags)
//	\x1b]10;?\x1b\\  OSC 10 fg color      → \x1b]10;rgb:c0c0/c0c0/c0c0\x1b\\
//	\x1b]11;?\x1b\\  OSC 11 bg color      → \x1b]11;rgb:0000/0000/0000\x1b\\
func respondToTerminalQueries(chunk []byte) []byte {
	var reply []byte
	for i := 0; i < len(chunk); i++ {
		// CSI sequences: ESC [ ... final-byte
		if i+1 < len(chunk) && chunk[i] == 0x1b && chunk[i+1] == '[' {
			// Find final byte (0x40-0x7e) and capture intermediate bytes.
			j := i + 2
			for j < len(chunk) && (chunk[j] < 0x40 || chunk[j] > 0x7e) {
				j++
			}
			if j >= len(chunk) {
				break
			}
			seq := chunk[i : j+1]
			switch string(seq) {
			case "\x1b[6n":
				reply = append(reply, []byte("\x1b[24;80R")...)
			case "\x1b[c", "\x1b[0c":
				reply = append(reply, []byte("\x1b[?1;2c")...)
			case "\x1b[>c", "\x1b[>0c":
				reply = append(reply, []byte("\x1b[>0;0;0c")...)
			case "\x1b[?u":
				reply = append(reply, []byte("\x1b[?0u")...)
			}
			i = j
			continue
		}
		// OSC sequences: ESC ] ... BEL or ST (ESC \)
		if i+1 < len(chunk) && chunk[i] == 0x1b && chunk[i+1] == ']' {
			// Find terminator.
			j := i + 2
			term := 0
			for j < len(chunk) {
				if chunk[j] == 0x07 {
					term = 1
					break
				}
				if chunk[j] == 0x1b && j+1 < len(chunk) && chunk[j+1] == '\\' {
					term = 2
					break
				}
				j++
			}
			if term == 0 {
				break
			}
			payload := string(chunk[i+2 : j])
			switch payload {
			case "10;?":
				reply = append(reply, []byte("\x1b]10;rgb:c0c0/c0c0/c0c0\x1b\\")...)
			case "11;?":
				reply = append(reply, []byte("\x1b]11;rgb:0000/0000/0000\x1b\\")...)
			}
			if term == 1 {
				i = j
			} else {
				i = j + 1
			}
		}
	}
	return reply
}
