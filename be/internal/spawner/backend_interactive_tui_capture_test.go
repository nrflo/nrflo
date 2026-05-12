// Tests for the openai/codex#21639 TUI-byte-capture workaround.
// When upstream ships a fix, these tests should be deleted alongside the
// production file (backend_interactive_tui_capture.go).
package spawner

import (
	"strings"
	"testing"

	"be/internal/repo"
)

// tuiEntries returns a snapshot of the pending message entries and the current
// tuiLineBuf length, both read under proc.messagesMutex.
func tuiEntries(proc *processInfo) (entries []repo.MessageEntry, bufLen int) {
	proc.messagesMutex.Lock()
	defer proc.messagesMutex.Unlock()
	entries = make([]repo.MessageEntry, len(proc.pendingMessages))
	copy(entries, proc.pendingMessages)
	bufLen = len(proc.tuiLineBuf)
	return
}

// TestCaptureTUI_StripsAnsi verifies that ANSI escape sequences are removed
// and the cleaned line is emitted as a "text" message.
func TestCaptureTUI_StripsAnsi(t *testing.T) {
	t.Parallel()
	s := noPoolSpawner()
	proc := minProc("tui-ansi-1")

	captureTUIChunk(s, proc, []byte("hello \x1b[31mworld\x1b[0m\n"))

	entries, _ := tuiEntries(proc)
	if len(entries) != 1 {
		t.Fatalf("captureTUIChunk: got %d messages, want 1", len(entries))
	}
	if entries[0].Content != "hello world" {
		t.Errorf("Content = %q, want %q", entries[0].Content, "hello world")
	}
	if entries[0].Category != "text" {
		t.Errorf("Category = %q, want %q", entries[0].Category, "text")
	}
}

// TestCaptureTUI_LineBuffersAcrossChunks verifies partial lines are buffered
// across captureTUIChunk calls and flushed when newlines arrive.
func TestCaptureTUI_LineBuffersAcrossChunks(t *testing.T) {
	t.Parallel()
	s := noPoolSpawner()
	proc := minProc("tui-buf-1")

	captureTUIChunk(s, proc, []byte("abc"))
	captureTUIChunk(s, proc, []byte("def\nghi\n"))

	entries, _ := tuiEntries(proc)
	if len(entries) != 2 {
		t.Fatalf("got %d messages, want 2", len(entries))
	}
	if entries[0].Content != "abcdef" {
		t.Errorf("entries[0].Content = %q, want %q", entries[0].Content, "abcdef")
	}
	if entries[1].Content != "ghi" {
		t.Errorf("entries[1].Content = %q, want %q", entries[1].Content, "ghi")
	}
}

// TestCaptureTUI_SkipsEmptyAfterStrip verifies that chunks consisting entirely
// of ANSI sequences produce no messages (empty after stripping).
func TestCaptureTUI_SkipsEmptyAfterStrip(t *testing.T) {
	t.Parallel()
	s := noPoolSpawner()
	proc := minProc("tui-empty-1")

	// CSI erase-line + cursor-up + newline: all non-printable after strip.
	captureTUIChunk(s, proc, []byte("\x1b[2K\x1b[1A\n"))

	entries, _ := tuiEntries(proc)
	if len(entries) != 0 {
		t.Errorf("got %d messages, want 0; first content = %q", len(entries), entries[0].Content)
	}
}

// TestCaptureTUI_TruncatesLongLine verifies a line exceeding 8 KB is capped
// with a trailing "…" marker.
func TestCaptureTUI_TruncatesLongLine(t *testing.T) {
	t.Parallel()
	s := noPoolSpawner()
	proc := minProc("tui-trunc-1")

	captureTUIChunk(s, proc, []byte(strings.Repeat("x", 10240)+"\n"))

	entries, _ := tuiEntries(proc)
	if len(entries) != 1 {
		t.Fatalf("got %d messages, want 1", len(entries))
	}
	got := entries[0].Content
	if !strings.HasSuffix(got, "…") {
		t.Errorf("Content does not end with truncation marker; got suffix %q", got[max(0, len(got)-10):])
	}
	want := tuiLineCapBytes + len("…")
	if len(got) != want {
		t.Errorf("len(Content) = %d, want %d (8 KB + len(…))", len(got), want)
	}
}

// TestCaptureTUI_BufferOverflow verifies that 70 KB of data without a newline
// triggers a force-flush with one truncated message and resets tuiLineBuf.
func TestCaptureTUI_BufferOverflow(t *testing.T) {
	t.Parallel()
	s := noPoolSpawner()
	proc := minProc("tui-overflow-1")

	// 70 KB > tuiBufCapBytes (64 KB) — force-flush must fire.
	captureTUIChunk(s, proc, []byte(strings.Repeat("b", 70*1024)))

	entries, bufLen := tuiEntries(proc)
	if len(entries) != 1 {
		t.Fatalf("got %d messages after overflow, want 1", len(entries))
	}
	if bufLen != 0 {
		t.Errorf("tuiLineBuf not reset after overflow: len=%d, want 0", bufLen)
	}
}

// TestFlushTUIBuffer_DrainsPartialLine verifies that flushTUIBuffer emits a
// partial (unterminated) line and clears the buffer.
func TestFlushTUIBuffer_DrainsPartialLine(t *testing.T) {
	t.Parallel()
	s := noPoolSpawner()
	proc := minProc("tui-flush-1")

	captureTUIChunk(s, proc, []byte("unterminated"))

	entries, _ := tuiEntries(proc)
	if len(entries) != 0 {
		t.Fatalf("expected 0 messages before flush, got %d", len(entries))
	}

	flushTUIBuffer(s, proc)

	entries, bufLen := tuiEntries(proc)
	if len(entries) != 1 {
		t.Fatalf("got %d messages after flush, want 1", len(entries))
	}
	if entries[0].Content != "unterminated" {
		t.Errorf("Content = %q, want %q", entries[0].Content, "unterminated")
	}
	if bufLen != 0 {
		t.Errorf("tuiLineBuf not cleared after flush: len=%d, want 0", bufLen)
	}
}

