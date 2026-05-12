// Temporary workaround for openai/codex#21639: codex hooks do not fire in
// PTY/TUI sessions in codex ≥ 0.129.0-alpha.15. captureTUIChunk and
// flushTUIBuffer capture the raw PTY byte stream, strip ANSI/control
// sequences, line-buffer, and emit non-empty lines via Spawner.TrackMessage.
//
// Cleanup checklist (when upstream ships a fix):
//  1. Flip (*CodexAdapter).CapturesTUIBytes() to return false.
//  2. After one release, delete this file.
//  3. Remove the tuiLineBuf field from processInfo.
//  4. Remove the captureTUI bool parameter from ferryPTYOutput.
//  5. Remove the CapturesTUIBytes() method from the CLIAdapter interface.
package spawner

import (
	"regexp"
	"strings"
)

const (
	tuiLineCapBytes = 8 * 1024      // 8 KB per-line cap
	tuiBufCapBytes  = 64 * 1024     // 64 KB total buffer cap before force-flush
)

// tuiAnsiRE matches ANSI/VT escape sequences (CSI, OSC, and 2-char escapes).
// Intentionally duplicated from be/internal/api/pty_input_capture.go — moving
// to a shared package adds churn for a temporary workaround.
var tuiAnsiRE = regexp.MustCompile(
	`\x1b(?:\[[0-9;?]*[A-Za-z]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[A-Za-z])`,
)

// stripANSI removes ANSI/VT escape sequences and replaces non-printable
// control bytes (except \t, \n, \r which are handled by the line splitter)
// with visible placeholders. Returns the cleaned string.
func stripANSI(data []byte) string {
	clean := tuiAnsiRE.ReplaceAll(data, nil)
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

// captureTUIChunk appends chunk to proc.tuiLineBuf (under messagesMutex),
// splits on newlines, and emits each completed non-empty line as a "text"
// agent_message via s.TrackMessage. Lines exceeding tuiLineCapBytes are
// truncated with "…". When the cumulative buffer exceeds tuiBufCapBytes
// without a newline (force-flush), the current partial line is emitted and
// the buffer is reset so memory stays bounded.
func captureTUIChunk(s *Spawner, proc *processInfo, chunk []byte) {
	stripped := stripANSI(chunk)
	if stripped == "" {
		return
	}

	var lines []string

	proc.messagesMutex.Lock()
	proc.tuiLineBuf = append(proc.tuiLineBuf, []byte(stripped)...)

	// Force-flush if buffer grew too large without a newline.
	if len(proc.tuiLineBuf) >= tuiBufCapBytes {
		line := capTUILine(string(proc.tuiLineBuf))
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
		proc.tuiLineBuf = nil
	} else {
		// Extract complete lines.
		for {
			idx := indexNewline(proc.tuiLineBuf)
			if idx < 0 {
				break
			}
			line := capTUILine(string(proc.tuiLineBuf[:idx]))
			proc.tuiLineBuf = proc.tuiLineBuf[idx+1:]
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
	}
	proc.messagesMutex.Unlock()

	// Emit outside mutex — TrackMessage acquires it internally.
	for _, line := range lines {
		s.TrackMessage(proc, line, "text")
	}
}

// flushTUIBuffer drains any partial line left in proc.tuiLineBuf and emits it
// as a "text" message. Called when the PTY session closes.
func flushTUIBuffer(s *Spawner, proc *processInfo) {
	var line string

	proc.messagesMutex.Lock()
	if len(proc.tuiLineBuf) > 0 {
		line = capTUILine(string(proc.tuiLineBuf))
		proc.tuiLineBuf = nil
	}
	proc.messagesMutex.Unlock()

	if strings.TrimSpace(line) != "" {
		s.TrackMessage(proc, line, "text")
	}
}

// indexNewline returns the index of the first \n or \r in b, or -1.
func indexNewline(b []byte) int {
	for i, c := range b {
		if c == '\n' || c == '\r' {
			return i
		}
	}
	return -1
}

// capTUILine truncates text to tuiLineCapBytes, appending "…" when over cap.
func capTUILine(text string) string {
	if len(text) <= tuiLineCapBytes {
		return text
	}
	return text[:tuiLineCapBytes] + "…"
}
