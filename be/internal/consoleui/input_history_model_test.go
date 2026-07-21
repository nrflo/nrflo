package consoleui

import (
	"strings"
	"testing"
)

// historyTestModel builds a headless *model with a real textarea and seeded
// history, sufficient to exercise handleHistoryKey without a terminal.
func historyTestModel(t *testing.T, entries []string) *model {
	t.Helper()
	input := newTestComposer(t)
	m := &model{input: input, history: inputHistory{entries: entries, index: len(entries)}}
	m.width, m.height, m.ready = 80, 24, true
	return m
}

// TestHandleHistoryKey covers the model-level key handler: precedence
// guards (suggestions/invoke), boundary detection, and footer wiring.
func TestHandleHistoryKey(t *testing.T) {
	t.Run("Up on single-line composer recalls and consumes", func(t *testing.T) {
		m := historyTestModel(t, []string{"first", "second"})
		if handled := m.handleHistoryKey("up"); !handled {
			t.Fatalf("handleHistoryKey(up) = false, want true")
		}
		if got := m.input.Value(); got != "second" {
			t.Errorf("input.Value() = %q, want %q", got, "second")
		}
		if !strings.Contains(m.footer(), "History") {
			t.Errorf("footer() = %q, want it to contain %q", m.footer(), "History")
		}
	})

	t.Run("Up returns false when suggestions dropdown is open", func(t *testing.T) {
		m := historyTestModel(t, []string{"first", "second"})
		m.skills = []ConsoleSkill{{Name: "invoke", Description: "d"}}
		m.input.SetValue("/inv")
		if !m.suggestionsOpen() {
			t.Fatalf("test setup: suggestionsOpen() = false, want true")
		}
		if handled := m.handleHistoryKey("up"); handled {
			t.Errorf("handleHistoryKey(up) = true, want false while suggestions open")
		}
	})

	t.Run("Up returns false when invoke flow is active", func(t *testing.T) {
		m := historyTestModel(t, []string{"first", "second"})
		m.invoke = startInvoke("mytool", nil)
		if handled := m.handleHistoryKey("up"); handled {
			t.Errorf("handleHistoryKey(up) = true, want false while invoke active")
		}
	})

	t.Run("interior cursor move: Up and Down both fall through", func(t *testing.T) {
		m := historyTestModel(t, []string{"first", "second"})
		m.input.SetValue("a\nb\nc")
		m.input.MoveToEnd()
		m.input.CursorUp() // now on line 1 (middle), not first or last line

		if got := m.input.Line(); got == 0 || got == m.input.LineCount()-1 {
			t.Fatalf("test setup: Line() = %d, LineCount() = %d, want interior line", got, m.input.LineCount())
		}
		if handled := m.handleHistoryKey("up"); handled {
			t.Errorf("handleHistoryKey(up) = true, want false at interior line")
		}
		if handled := m.handleHistoryKey("down"); handled {
			t.Errorf("handleHistoryKey(down) = true, want false at interior line")
		}
	})

	t.Run("Down on last line steps newer and restores draft past newest", func(t *testing.T) {
		m := historyTestModel(t, []string{"first", "second"})
		m.history.index = 1 // browsing "second" (newest)
		m.history.draft = "my draft"
		if handled := m.handleHistoryKey("down"); !handled {
			t.Fatalf("handleHistoryKey(down) = false, want true")
		}
		if got := m.input.Value(); got != "my draft" {
			t.Errorf("input.Value() = %q, want %q", got, "my draft")
		}
		if strings.Contains(m.footer(), "History") {
			t.Errorf("footer() = %q, want no History indicator at draft slot", m.footer())
		}
	})

	t.Run("footer indicator clears after record()", func(t *testing.T) {
		m := historyTestModel(t, []string{"first"})
		m.handleHistoryKey("up")
		if !strings.Contains(m.footer(), "History") {
			t.Fatalf("test setup: footer() = %q, want History indicator while browsing", m.footer())
		}
		m.history = m.history.record("second")
		if strings.Contains(m.footer(), "History") {
			t.Errorf("footer() = %q, want no History indicator after record()", m.footer())
		}
	})
}
