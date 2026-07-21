package consoleui

import "testing"

// TestNewHistoryFromContents covers seeding from a flat, oldest->newest list
// of raw contents (Client.History's response shape): global keep-last dedup
// (not merely consecutive), cap dropping the oldest beyond historyLimit, and
// the result starting at the draft slot. Split out of input_history_test.go
// to stay under the 300-line file cap.
func TestNewHistoryFromContents(t *testing.T) {
	t.Run("global keep-last dedup: non-consecutive duplicate keeps the later occurrence", func(t *testing.T) {
		h := newHistoryFromContents([]string{"a", "b", "a"})
		want := []string{"b", "a"}
		if len(h.entries) != len(want) {
			t.Fatalf("entries = %v, want %v", h.entries, want)
		}
		for i, w := range want {
			if h.entries[i] != w {
				t.Errorf("entries[%d] = %q, want %q", i, h.entries[i], w)
			}
		}
		if h.index != len(h.entries) {
			t.Errorf("index = %d, want draft slot %d", h.index, len(h.entries))
		}
	})

	t.Run("preserves oldest to newest order with no duplicates", func(t *testing.T) {
		h := newHistoryFromContents([]string{"x", "y", "z"})
		want := []string{"x", "y", "z"}
		if len(h.entries) != len(want) {
			t.Fatalf("entries = %v, want %v", h.entries, want)
		}
		for i, w := range want {
			if h.entries[i] != w {
				t.Errorf("entries[%d] = %q, want %q", i, h.entries[i], w)
			}
		}
	})

	t.Run("caps at historyLimit dropping oldest", func(t *testing.T) {
		contents := make([]string, 0, historyLimit+10)
		for i := 0; i < historyLimit+10; i++ {
			contents = append(contents, label(i))
		}
		h := newHistoryFromContents(contents)
		if len(h.entries) != historyLimit {
			t.Fatalf("len(entries) = %d, want %d", len(h.entries), historyLimit)
		}
		if h.entries[0] != label(10) {
			t.Errorf("entries[0] = %q, want %q (oldest 10 dropped)", h.entries[0], label(10))
		}
		if h.entries[len(h.entries)-1] != label(historyLimit+9) {
			t.Errorf("entries[last] = %q, want %q", h.entries[len(h.entries)-1], label(historyLimit+9))
		}
		if h.index != len(h.entries) {
			t.Errorf("index = %d, want draft slot %d", h.index, len(h.entries))
		}
	})

	t.Run("empty input yields empty history at draft slot", func(t *testing.T) {
		h := newHistoryFromContents(nil)
		if len(h.entries) != 0 {
			t.Errorf("entries = %v, want empty", h.entries)
		}
		if h.index != 0 {
			t.Errorf("index = %d, want 0", h.index)
		}
	})
}
