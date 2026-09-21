// Tests for bracketed-paste prompt delivery: mode detection off the TUI's own
// init burst (ferryPTYOutput) and the wrapping decision (writePromptOnce).
package spawner

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
)

// pasteBody is long enough to exercise the failure this guards: an unmarked
// bulk write into a bracketed-paste TUI is processed as keystrokes and all but
// the last ~100 bytes are dropped, so the agent gets a fragment starting
// mid-word while every delivery signal still reports success.
var pasteBody = strings.Repeat("instruction line\n", 250) + "Then call complete_step.\n"

func pasteProc(bracketed bool) *processInfo {
	p := &processInfo{sessionID: "sess-paste", agentType: "prepare", doneCh: make(chan struct{})}
	p.bracketedPaste = bracketed
	return p
}

// TestWritePromptOnce_WrapsBodyWhenTUIUsesBracketedPaste is the regression
// guard for the truncated-prompt failure: a 4KB body delivered unmarked to
// Claude's TUI reached the model as its final 78 bytes.
func TestWritePromptOnce_WrapsBodyWhenTUIUsesBracketedPaste(t *testing.T) {
	t.Parallel()

	s := New(Config{})
	sess := newMockSession()

	if !writePromptOnce(s, pasteProc(true), sess, pasteBody, "claude") {
		t.Fatal("writePromptOnce returned false on a healthy session")
	}

	sess.mu.Lock()
	got := string(sess.writtenBytes)
	sess.mu.Unlock()

	if !strings.HasPrefix(got, bracketedPasteSet2004) {
		t.Errorf("body not opened with a paste marker — the TUI will treat it as keystrokes and drop all but the tail")
	}
	if !strings.Contains(got, bracketedPasteEnd+"\r") {
		t.Errorf("submit CR must follow the paste terminator, not sit inside the paste (it would be pasted as text, never submitting)")
	}
	if !strings.Contains(got, pasteBody) {
		t.Errorf("body altered in transit; want it delivered verbatim inside the markers")
	}
}

// TestWritePromptOnce_NoWrapWhenModeOff pins the other half: a TUI that never
// enabled ?2004 would render the marker bytes as literal text, corrupting the
// prompt it was meant to protect.
func TestWritePromptOnce_NoWrapWhenModeOff(t *testing.T) {
	t.Parallel()

	s := New(Config{})
	sess := newMockSession()

	writePromptOnce(s, pasteProc(false), sess, "PROMPT", "claude")

	sess.mu.Lock()
	got := string(sess.writtenBytes)
	sess.mu.Unlock()

	if strings.Contains(got, "\x1b[200~") || strings.Contains(got, "\x1b[201~") {
		t.Errorf("wrote paste markers to a TUI that never requested the mode: %q", got)
	}
	if got != "PROMPT\r" {
		t.Errorf("payload = %q, want %q", got, "PROMPT\r")
	}
}

// TestStripPasteEnd_RemovesEmbeddedTerminator: a body containing the
// terminator would close the paste early and drop the remainder back onto the
// keystroke path — the exact truncation being fixed.
func TestStripPasteEnd_RemovesEmbeddedTerminator(t *testing.T) {
	t.Parallel()

	if got := stripPasteEnd("head" + bracketedPasteEnd + "tail"); got != "headtail" {
		t.Errorf("stripPasteEnd = %q, want %q", got, "headtail")
	}
	if got := stripPasteEnd("clean body"); got != "clean body" {
		t.Errorf("stripPasteEnd altered a clean body: %q", got)
	}
}

func TestScanBracketedPasteMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		carry     string
		chunk     string
		wantMode  bool
		wantFound bool
	}{
		{name: "set", chunk: "\x1b[?2004h\x1b[6n", wantMode: true, wantFound: true},
		{name: "reset", chunk: "\x1b[?2004l", wantMode: false, wantFound: true},
		{name: "last change wins", chunk: "\x1b[?2004h padding \x1b[?2004l", wantMode: false, wantFound: true},
		{name: "re-enabled after reset", chunk: "\x1b[?2004l padding \x1b[?2004h", wantMode: true, wantFound: true},
		{name: "no mode change", chunk: "\x1b[2J\x1b[H plain output", wantFound: false},
		{name: "split across reads", carry: "\x1b[?20", chunk: "04h", wantMode: true, wantFound: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mode, found := scanBracketedPasteMode([]byte(tt.carry), []byte(tt.chunk))
			if found != tt.wantFound || mode != tt.wantMode {
				t.Errorf("scanBracketedPasteMode = (%t, %t), want (%t, %t)", mode, found, tt.wantMode, tt.wantFound)
			}
		})
	}
}

// TestFerryPTYOutput_DetectsBracketedPasteFromInitBurst wires the two halves
// together: the mode is learned from the TUI's real init burst, before
// deliverPrompt's quiescence gate releases the write.
func TestFerryPTYOutput_DetectsBracketedPasteFromInitBurst(t *testing.T) {
	t.Parallel()

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s := New(Config{Clock: clk})
	proc := ferryProc("paste-detect-1", clk.Now())

	sess := newMockSession()
	// Claude's observed init burst: alt screen, bracketed paste on, clear.
	sess.readChunks = []string{"\x1b[?1049h\x1b[?2004h\x1b[2J"}
	sess.Kill() //nolint — always nil; pre-closes the done channel

	ferryPTYOutput(s, proc, sess, false, false)

	proc.messagesMutex.Lock()
	got := proc.bracketedPaste
	proc.messagesMutex.Unlock()

	if !got {
		t.Error("bracketed paste not detected from the init burst — prompts will be delivered unmarked and truncated")
	}
}

// TestFerryPTYOutput_NoModeSequenceLeavesPasteOff guards the default: absent an
// explicit ?2004h we must not invent paste markers.
func TestFerryPTYOutput_NoModeSequenceLeavesPasteOff(t *testing.T) {
	t.Parallel()

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s := New(Config{Clock: clk})
	proc := ferryProc("paste-detect-2", clk.Now())

	sess := newMockSession()
	sess.readChunks = []string{"plain output with no mode sequences"}
	sess.Kill() //nolint — always nil; pre-closes the done channel

	ferryPTYOutput(s, proc, sess, false, false)

	proc.messagesMutex.Lock()
	got := proc.bracketedPaste
	proc.messagesMutex.Unlock()

	if got {
		t.Error("bracketed paste enabled without the TUI ever requesting it")
	}
}

// TestClaudeEngine_WriteTurnText_WrapsWhenPasteModeOn covers the console
// engine's copy of the same PTY discipline: a long console turn (a pasted
// spec, a steered instruction) truncates identically without the markers.
func TestClaudeEngine_WriteTurnText_WrapsWhenPasteModeOn(t *testing.T) {
	t.Parallel()

	e := &claudeEngine{bracketedPaste: true}
	sess := newMockSession()

	if err := e.writeTurnText(sess, pasteBody); err != nil {
		t.Fatalf("writeTurnText: %v", err)
	}

	sess.mu.Lock()
	got := string(sess.writtenBytes)
	sess.mu.Unlock()

	if !strings.HasPrefix(got, bracketedPasteSet2004) || !strings.HasSuffix(got, bracketedPasteEnd) {
		t.Error("console turn text not wrapped in paste markers — long turns will reach the model truncated")
	}
	if !strings.Contains(got, pasteBody) {
		t.Error("console turn text altered in transit")
	}
}

// TestClaudeEngine_WriteTurnText_NoWrapWhenPasteModeOff keeps the markers off
// the wire for a TUI that never requested the mode.
func TestClaudeEngine_WriteTurnText_NoWrapWhenPasteModeOff(t *testing.T) {
	t.Parallel()

	e := &claudeEngine{}
	sess := newMockSession()

	if err := e.writeTurnText(sess, "hello"); err != nil {
		t.Fatalf("writeTurnText: %v", err)
	}

	sess.mu.Lock()
	got := string(sess.writtenBytes)
	sess.mu.Unlock()

	if got != "hello" {
		t.Errorf("payload = %q, want %q", got, "hello")
	}
}

// TestClaudeEngine_WriteTurnText_SlashBypassesPasteMode guards the native
// command-palette path: Claude 2.1.247 consumes bracketed-pasted slash
// commands without submitting them, so short command turns stay raw even
// while the TUI advertises bracketed paste for ordinary prompt bodies.
func TestClaudeEngine_WriteTurnText_SlashBypassesPasteMode(t *testing.T) {
	t.Parallel()

	e := &claudeEngine{bracketedPaste: true}
	sess := newMockSession()

	if err := e.writeTurnText(sess, "/address-comments"); err != nil {
		t.Fatalf("writeTurnText: %v", err)
	}

	sess.mu.Lock()
	got := string(sess.writtenBytes)
	sess.mu.Unlock()
	if got != "/address-comments" {
		t.Errorf("slash payload = %q, want raw command without paste markers", got)
	}
}

func TestSanitizePromptBody(t *testing.T) {
	cases := map[string]string{
		"We\u0080\u0094\u0099re": "Were",
		"a\x00b\x1b[31mc\x7f":    "ab[31mc",
		"line1\nline2\tx\r":      "line1\nline2\tx",
		"héllo — “quoted” 日本語 ✓": "héllo — “quoted” 日本語 ✓",
	}
	for in, want := range cases {
		if got := sanitizePromptBody(in); got != want {
			t.Errorf("sanitizePromptBody(%q) = %q, want %q", in, got, want)
		}
	}
}
