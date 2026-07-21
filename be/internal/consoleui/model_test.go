package consoleui

import (
	"bytes"
	"testing"
)

// TestClearScreenSeqLiteral pins the exact escape sequence: erase-screen
// (2J), erase-scrollback (3J), cursor-home (H), in that order.
func TestClearScreenSeqLiteral(t *testing.T) {
	want := "\x1b[2J\x1b[3J\x1b[H"
	if clearScreenSeq != want {
		t.Errorf("clearScreenSeq = %q, want %q", clearScreenSeq, want)
	}
}

// TestClearTerminalWritesSequence covers the best-effort write helper used
// by Run() to reset the terminal before starting the inline program.
func TestClearTerminalWritesSequence(t *testing.T) {
	var buf bytes.Buffer
	clearTerminal(&buf)
	if got, want := buf.String(), "\x1b[2J\x1b[3J\x1b[H"; got != want {
		t.Errorf("clearTerminal wrote %q, want %q", got, want)
	}
}

// TestClearTerminalNilSafeWriter ensures a writer that errors doesn't panic
// clearTerminal (ignored-error convention: _, _ = io.WriteString(...)).
func TestClearTerminalNilSafeWriter(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("clearTerminal panicked with erroring writer: %v", r)
		}
	}()
	clearTerminal(errWriter{})
}

// errWriter always fails, simulating a broken stdout.
type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) {
	return 0, errAlwaysFail
}

var errAlwaysFail = errWriterErr("write failed")

type errWriterErr string

func (e errWriterErr) Error() string { return string(e) }
