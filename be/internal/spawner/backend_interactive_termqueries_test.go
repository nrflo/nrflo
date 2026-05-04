package spawner

import "testing"

func TestRespondToTerminalQueries_DSRCursorPosition(t *testing.T) {
	t.Parallel()
	got := respondToTerminalQueries([]byte("\x1b[6n"))
	if string(got) != "\x1b[24;80R" {
		t.Errorf("DSR reply = %q, want %q", got, "\x1b[24;80R")
	}
}

func TestRespondToTerminalQueries_DAPrimary(t *testing.T) {
	t.Parallel()
	got := respondToTerminalQueries([]byte("\x1b[c"))
	if string(got) != "\x1b[?1;2c" {
		t.Errorf("DA primary reply = %q, want %q", got, "\x1b[?1;2c")
	}
}

func TestRespondToTerminalQueries_KittyKeyboard(t *testing.T) {
	t.Parallel()
	got := respondToTerminalQueries([]byte("\x1b[?u"))
	if string(got) != "\x1b[?0u" {
		t.Errorf("kitty reply = %q, want %q", got, "\x1b[?0u")
	}
}

func TestRespondToTerminalQueries_OSCForegroundColor(t *testing.T) {
	t.Parallel()
	got := respondToTerminalQueries([]byte("\x1b]10;?\x1b\\"))
	want := "\x1b]10;rgb:c0c0/c0c0/c0c0\x1b\\"
	if string(got) != want {
		t.Errorf("OSC 10 reply = %q, want %q", got, want)
	}
}

// Replicates the exact init burst codex sends in chunks 1-4 of our captured run:
// bracketed paste set, kitty push, focus events set, DSR, kitty query, DA query.
// Of those, DSR / kitty-query / DA are the ones a terminal must answer.
func TestRespondToTerminalQueries_CodexInitBurst(t *testing.T) {
	t.Parallel()
	burst := []byte("\x1b[?2004h\x1b[>7u\x1b[?1004h\x1b[6n\x1b[?u\x1b[c")
	got := respondToTerminalQueries(burst)
	want := "\x1b[24;80R\x1b[?0u\x1b[?1;2c"
	if string(got) != want {
		t.Errorf("codex init reply = %q, want %q", got, want)
	}
}

func TestRespondToTerminalQueries_NoQueriesNoReply(t *testing.T) {
	t.Parallel()
	// SET sequences (lowercase l = reset, h = set) and cursor moves are not
	// queries — must produce no reply.
	noQuery := []byte("\x1b[?2004h\x1b[?2004l\x1b[H\x1b[2J\x1b[?25l\x1b[?25h")
	got := respondToTerminalQueries(noQuery)
	if len(got) != 0 {
		t.Errorf("non-query stream produced reply: %q", got)
	}
}

func TestRespondToTerminalQueries_EmptyAndPartial(t *testing.T) {
	t.Parallel()
	if got := respondToTerminalQueries(nil); len(got) != 0 {
		t.Errorf("nil chunk produced reply: %q", got)
	}
	// Truncated CSI (no final byte) — must not panic and must produce no reply.
	if got := respondToTerminalQueries([]byte("\x1b[6")); len(got) != 0 {
		t.Errorf("truncated CSI produced reply: %q", got)
	}
	// Truncated OSC (no terminator) — must not panic and must produce no reply.
	if got := respondToTerminalQueries([]byte("\x1b]10;?")); len(got) != 0 {
		t.Errorf("truncated OSC produced reply: %q", got)
	}
}
