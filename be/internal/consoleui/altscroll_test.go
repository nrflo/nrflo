package consoleui

import "testing"

// TestAltScrollSequences pins the exact DECSET 1007 escape bytes used to
// enable/disable alternate-scroll mode. bubbletea v2 and x/ansi expose no
// constant for mode 1007 (see altscroll.go), so these raw sequences are the
// sole source of truth for the wire bytes sent via tea.Raw / os.Stdout.
func TestAltScrollSequences(t *testing.T) {
	if altScrollEnable != "\x1b[?1007h" {
		t.Errorf("altScrollEnable = %q, want %q", altScrollEnable, "\x1b[?1007h")
	}
	if altScrollDisable != "\x1b[?1007l" {
		t.Errorf("altScrollDisable = %q, want %q", altScrollDisable, "\x1b[?1007l")
	}
}
