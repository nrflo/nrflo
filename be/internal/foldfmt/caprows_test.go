package foldfmt

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestCapRows_PassThroughUnderCap verifies every row at or under perRow is
// returned unmodified — no truncation marker appended.
func TestCapRows_PassThroughUnderCap(t *testing.T) {
	t.Parallel()
	lines := []string{"short one", "short two", strings.Repeat("a", 10)}
	got := CapRows(lines, 10)
	for i, line := range lines {
		if got[i] != line {
			t.Errorf("CapRows(%q, 10)[%d] = %q, want unmodified %q", lines, i, got[i], line)
		}
	}
}

// TestCapRows_TruncatesOverCapWithMarker verifies a row over perRow is
// head-capped to perRow bytes plus a truncation marker appended.
func TestCapRows_TruncatesOverCapWithMarker(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("b", 50)
	got := CapRows([]string{long}, 10)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if !strings.HasPrefix(got[0], strings.Repeat("b", 10)) {
		t.Errorf("CapRows over-cap = %q, want it to start with the first 10 bytes", got[0])
	}
	if !strings.HasSuffix(got[0], "…[truncated]") {
		t.Errorf("CapRows over-cap = %q, want it to end with the truncation marker", got[0])
	}
}

// TestCapRows_UTF8SafeTruncation verifies a row whose cap point falls mid
// multi-byte rune backs off to the rune boundary (via CapBytes) rather than
// splitting it, and still appends the truncation marker.
func TestCapRows_UTF8SafeTruncation(t *testing.T) {
	t.Parallel()
	// 'a' (1 byte) + 'é' (2 bytes) = 3 bytes; capping at 2 would split 'é'.
	s := "aé" + strings.Repeat("z", 20)
	got := CapRows([]string{s}, 2)
	if !utf8.ValidString(got[0]) {
		t.Errorf("CapRows(%q, 2) = %q is not valid UTF-8", s, got[0])
	}
	if !strings.HasPrefix(got[0], "a") || strings.Contains(got[0], "é") {
		t.Errorf("CapRows(%q, 2) = %q, want it to back off past the split rune boundary", s, got[0])
	}
	if !strings.HasSuffix(got[0], "…[truncated]") {
		t.Errorf("CapRows over-cap = %q, want the truncation marker", got[0])
	}
}

// TestCapRows_EmptyInput verifies a nil/empty slice returns an empty
// (non-nil-panicking) slice.
func TestCapRows_EmptyInput(t *testing.T) {
	t.Parallel()
	got := CapRows(nil, 100)
	if len(got) != 0 {
		t.Errorf("CapRows(nil, 100) = %v, want empty", got)
	}
}
