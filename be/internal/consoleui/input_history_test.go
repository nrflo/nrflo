package consoleui

import (
	"fmt"
	"testing"
)

// TestNewHistory covers seeding from a message page: only user_input rows
// are kept, order is preserved oldest→newest, consecutive dupes are
// deduped, and the result caps at historyLimit with the oldest dropped.
func TestNewHistory(t *testing.T) {
	t.Run("filters non-user_input categories", func(t *testing.T) {
		msgs := []Message{
			{Category: "user_input", Content: "a"},
			{Category: "text", Content: "ignored"},
			{Category: "tool", Content: "ignored"},
			{Category: "thinking", Content: "ignored"},
			{Category: "user_input", Content: "b"},
		}
		h := newHistory(msgs)
		want := []string{"a", "b"}
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

	t.Run("dedupes consecutive user_input rows", func(t *testing.T) {
		msgs := []Message{
			{Category: "user_input", Content: "a"},
			{Category: "user_input", Content: "a"},
			{Category: "user_input", Content: "b"},
		}
		h := newHistory(msgs)
		want := []string{"a", "b"}
		if len(h.entries) != len(want) || h.entries[0] != want[0] || h.entries[1] != want[1] {
			t.Errorf("entries = %v, want %v", h.entries, want)
		}
	})

	t.Run("caps at historyLimit dropping oldest", func(t *testing.T) {
		msgs := make([]Message, 0, historyLimit+10)
		for i := 0; i < historyLimit+10; i++ {
			msgs = append(msgs, Message{Category: "user_input", Content: label(i)})
		}
		h := newHistory(msgs)
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
}

func label(i int) string {
	return fmt.Sprintf("msg-%d", i)
}

// TestAppendEntry covers the pure copy-on-write append/dedupe/cap logic.
func TestAppendEntry(t *testing.T) {
	t.Run("appends new entry", func(t *testing.T) {
		got := appendEntry([]string{"a"}, "b")
		want := []string{"a", "b"}
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("appendEntry = %v, want %v", got, want)
		}
	})

	t.Run("consecutive duplicate is a no-op", func(t *testing.T) {
		entries := []string{"a", "b"}
		got := appendEntry(entries, "b")
		if len(got) != 2 {
			t.Fatalf("appendEntry = %v, want unchanged len 2", got)
		}
		if got[1] != "b" {
			t.Errorf("appendEntry = %v, want last entry %q", got, "b")
		}
	})

	t.Run("non-consecutive duplicate is appended", func(t *testing.T) {
		got := appendEntry([]string{"a", "b"}, "a")
		want := []string{"a", "b", "a"}
		if len(got) != len(want) {
			t.Fatalf("appendEntry = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("appendEntry[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("drops oldest beyond cap", func(t *testing.T) {
		entries := make([]string, historyLimit)
		for i := range entries {
			entries[i] = label(i)
		}
		got := appendEntry(entries, "new")
		if len(got) != historyLimit {
			t.Fatalf("len(appendEntry) = %d, want %d", len(got), historyLimit)
		}
		if got[0] != label(1) {
			t.Errorf("got[0] = %q, want %q (oldest dropped)", got[0], label(1))
		}
		if got[len(got)-1] != "new" {
			t.Errorf("got[last] = %q, want %q", got[len(got)-1], "new")
		}
	})

	t.Run("does not mutate input slice", func(t *testing.T) {
		entries := []string{"a", "b"}
		orig := append([]string(nil), entries...)
		_ = appendEntry(entries, "c")
		for i := range orig {
			if entries[i] != orig[i] {
				t.Errorf("input slice mutated: entries = %v, want %v", entries, orig)
			}
		}
		if len(entries) != 2 {
			t.Errorf("input slice length changed: %v", entries)
		}
	})
}

// TestRecord covers appending on send: index resets to the draft slot and
// draft is cleared even when the message is a consecutive duplicate.
func TestRecord(t *testing.T) {
	t.Run("appends and resets to draft slot", func(t *testing.T) {
		h := inputHistory{entries: []string{"a"}, index: 0, draft: "browsing text"}
		h = h.record("b")
		want := []string{"a", "b"}
		if len(h.entries) != 2 || h.entries[0] != want[0] || h.entries[1] != want[1] {
			t.Errorf("entries = %v, want %v", h.entries, want)
		}
		if h.index != len(h.entries) {
			t.Errorf("index = %d, want %d", h.index, len(h.entries))
		}
		if h.draft != "" {
			t.Errorf("draft = %q, want cleared", h.draft)
		}
	})

	t.Run("consecutive duplicate still resets index and clears draft", func(t *testing.T) {
		h := inputHistory{entries: []string{"a", "b"}, index: 0, draft: "browsing text"}
		h = h.record("b")
		if len(h.entries) != 2 {
			t.Fatalf("entries = %v, want unchanged len 2 (dedup)", h.entries)
		}
		if h.index != len(h.entries) {
			t.Errorf("index = %d, want %d", h.index, len(h.entries))
		}
		if h.draft != "" {
			t.Errorf("draft = %q, want cleared", h.draft)
		}
	})
}

// TestHistoryPrev covers Up-key recall stepping.
func TestHistoryPrev(t *testing.T) {
	t.Run("from draft saves current and returns newest", func(t *testing.T) {
		h := inputHistory{entries: []string{"a", "b", "c"}, index: 3}
		h, val, ok := historyPrev(h, "in progress")
		if !ok {
			t.Fatalf("historyPrev handled = false, want true")
		}
		if val != "c" {
			t.Errorf("val = %q, want %q", val, "c")
		}
		if h.draft != "in progress" {
			t.Errorf("draft = %q, want %q", h.draft, "in progress")
		}
		if h.index != 2 {
			t.Errorf("index = %d, want 2", h.index)
		}
	})

	t.Run("steps to older on repeat", func(t *testing.T) {
		h := inputHistory{entries: []string{"a", "b", "c"}, index: 2, draft: "in progress"}
		h, val, ok := historyPrev(h, "unused")
		if !ok || val != "b" || h.index != 1 {
			t.Errorf("historyPrev = (idx=%d,val=%q,ok=%v), want (1,%q,true)", h.index, val, ok, "b")
		}
	})

	t.Run("Up at oldest stays put and remains handled", func(t *testing.T) {
		h := inputHistory{entries: []string{"a", "b", "c"}, index: 0, draft: "d"}
		h, val, ok := historyPrev(h, "unused")
		if !ok {
			t.Fatalf("historyPrev handled = false, want true")
		}
		if val != "a" || h.index != 0 {
			t.Errorf("historyPrev = (idx=%d,val=%q), want (0,%q)", h.index, val, "a")
		}
	})

	t.Run("empty history returns unhandled", func(t *testing.T) {
		h := inputHistory{}
		_, val, ok := historyPrev(h, "text")
		if ok {
			t.Errorf("historyPrev handled = true, want false for empty history")
		}
		if val != "" {
			t.Errorf("val = %q, want empty", val)
		}
	})

	t.Run("empty composer Up returns newest with draft empty", func(t *testing.T) {
		h := inputHistory{entries: []string{"a", "b"}, index: 2}
		h, val, ok := historyPrev(h, "")
		if !ok || val != "b" {
			t.Fatalf("historyPrev = (val=%q,ok=%v), want (%q,true)", val, ok, "b")
		}
		if h.draft != "" {
			t.Errorf("draft = %q, want empty", h.draft)
		}
	})
}

// TestHistoryNext covers Down-key recall stepping, including restoring the
// saved draft once stepped past the newest entry.
func TestHistoryNext(t *testing.T) {
	t.Run("steps newer from a browsed index", func(t *testing.T) {
		h := inputHistory{entries: []string{"a", "b", "c"}, index: 0, draft: "d"}
		h, val, ok := historyNext(h)
		if !ok || val != "b" || h.index != 1 {
			t.Errorf("historyNext = (idx=%d,val=%q,ok=%v), want (1,%q,true)", h.index, val, ok, "b")
		}
	})

	t.Run("stepping past newest returns saved draft", func(t *testing.T) {
		h := inputHistory{entries: []string{"a", "b", "c"}, index: 2, draft: "saved draft"}
		h, val, ok := historyNext(h)
		if !ok {
			t.Fatalf("historyNext handled = false, want true")
		}
		if val != "saved draft" {
			t.Errorf("val = %q, want %q", val, "saved draft")
		}
		if h.index != len(h.entries) {
			t.Errorf("index = %d, want %d (draft slot)", h.index, len(h.entries))
		}
	})

	t.Run("at draft slot returns unhandled", func(t *testing.T) {
		h := inputHistory{entries: []string{"a", "b"}, index: 2}
		_, _, ok := historyNext(h)
		if ok {
			t.Errorf("historyNext handled = true, want false at draft slot")
		}
	})
}

// TestIndicator covers the footer label derivation.
func TestIndicator(t *testing.T) {
	t.Run("browsing shows 1-based position", func(t *testing.T) {
		h := inputHistory{entries: []string{"a", "b", "c"}, index: 1}
		label, show := h.indicator()
		if !show {
			t.Fatalf("show = false, want true while browsing")
		}
		if label != "History 2/3" {
			t.Errorf("label = %q, want %q", label, "History 2/3")
		}
	})

	t.Run("at draft slot hides indicator", func(t *testing.T) {
		h := inputHistory{entries: []string{"a", "b"}, index: 2}
		_, show := h.indicator()
		if show {
			t.Errorf("show = true, want false at draft slot")
		}
	})

	t.Run("empty history hides indicator", func(t *testing.T) {
		h := inputHistory{}
		label, show := h.indicator()
		if show || label != "" {
			t.Errorf("indicator() = (%q,%v), want (\"\",false)", label, show)
		}
	})
}
