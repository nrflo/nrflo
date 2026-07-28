package consoleui

import (
	"testing"

	"charm.land/bubbles/v2/textarea"
)

// messagesOf builds n placeholder Messages labelled by absolute index, for
// building MessagePage fixtures whose Total tracks a growing DB.
func messagesOf(labels ...string) []Message {
	out := make([]Message, len(labels))
	for i, label := range labels {
		out[i] = Message{Category: "text", Content: label}
	}
	return out
}

// TestNewMessagesToPrint covers the pure print-once/dedupe splitter: the
// DB-absolute high-water mark (printedTotal) diffed against a reloaded
// page's [firstAbs, Total) window.
func TestNewMessagesToPrint(t *testing.T) {
	tests := []struct {
		name         string
		printedTotal int
		page         MessagePage
		wantContents []string
		wantNewTotal int
	}{
		{
			name:         "first load seeds all",
			printedTotal: 0,
			page:         MessagePage{Messages: messagesOf("a", "b", "c"), Total: 3},
			wantContents: []string{"a", "b", "c"},
			wantNewTotal: 3,
		},
		{
			name:         "skips already-printed rows",
			printedTotal: 3,
			page:         MessagePage{Messages: messagesOf("a", "b", "c"), Total: 3},
			wantContents: nil,
			wantNewTotal: 3,
		},
		{
			name:         "prints only new rows on incremental Total",
			printedTotal: 3,
			// firstAbs=3-4=-1 clamped to 0 by the model's own window (server
			// returns tail window >= printedTotal in practice); here page still
			// contains all rows a..d with Total advanced to 4.
			page:         MessagePage{Messages: messagesOf("a", "b", "c", "d"), Total: 4},
			wantContents: []string{"d"},
			wantNewTotal: 4,
		},
		{
			name:         "reconnect with identical page yields none",
			printedTotal: 5,
			page:         MessagePage{Messages: messagesOf("x", "y"), Total: 5},
			wantContents: nil,
			wantNewTotal: 5,
		},
		{
			name:         "window gap prints whole tail and advances",
			printedTotal: 2, // stale/gapped: firstAbs is far ahead of printedTotal
			page:         MessagePage{Messages: messagesOf("m", "n", "o"), Total: 100},
			wantContents: []string{"m", "n", "o"},
			wantNewTotal: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toPrint, newTotal := newMessagesToPrint(tt.printedTotal, tt.page)
			if newTotal != tt.wantNewTotal {
				t.Errorf("newTotal = %d, want %d", newTotal, tt.wantNewTotal)
			}
			if len(toPrint) != len(tt.wantContents) {
				t.Fatalf("toPrint = %+v, want contents %v", toPrint, tt.wantContents)
			}
			for i, want := range tt.wantContents {
				if toPrint[i].Content != want {
					t.Errorf("toPrint[%d].Content = %q, want %q", i, toPrint[i].Content, want)
				}
			}
		})
	}
}

// printTestModel builds a ready *model literal sufficient to exercise
// printNewMessages without a running terminal.
func printTestModel(printedTotal int, pendingUser string) *model {
	input := textarea.New()
	m := &model{printedTotal: printedTotal, pendingUser: pendingUser, input: input}
	m.width, m.height, m.ready = 80, 24, true
	return m
}

// TestPrintNewMessages_AdvancesAndClearsPendingUser verifies printNewMessages
// returns a print command, advances m.printedTotal to page.Total, and clears
// the optimistic pendingUser line once >=1 row printed.
func TestPrintNewMessages_AdvancesAndClearsPendingUser(t *testing.T) {
	m := printTestModel(0, "hello")
	page := MessagePage{Messages: []Message{{Category: "user_input", Content: "hello"}}, Total: 1}

	cmds := m.printNewMessages(page)
	if cmds == nil {
		t.Fatal("printNewMessages returned nil cmd, want a print sequence")
	}
	if m.printedTotal != 1 {
		t.Errorf("printedTotal = %d, want 1", m.printedTotal)
	}
	if m.pendingUser != "" {
		t.Errorf("pendingUser = %q, want cleared after printing", m.pendingUser)
	}
}

// TestPrintNewMessages_NoNewRowsLeavesPendingUserAlone verifies that when
// there is nothing new to print, printedTotal still lands on page.Total but
// pendingUser (an in-flight optimistic line not yet committed) is untouched.
func TestPrintNewMessages_NoNewRowsLeavesPendingUserAlone(t *testing.T) {
	m := printTestModel(2, "still typing echo")
	page := MessagePage{Messages: messagesOf("a", "b"), Total: 2}

	cmds := m.printNewMessages(page)
	if cmds != nil {
		t.Fatal("printNewMessages returned a cmd, want nil (nothing new)")
	}
	if m.printedTotal != 2 {
		t.Errorf("printedTotal = %d, want 2", m.printedTotal)
	}
	if m.pendingUser != "still typing echo" {
		t.Errorf("pendingUser = %q, want untouched", m.pendingUser)
	}
}

// TestContentWidth verifies the composer-padding-matching floor/formula.
func TestContentWidth(t *testing.T) {
	tests := []struct{ width, want int }{
		{100, 96},
		{10, 20}, // floors at 20 for narrow terminals
		{24, 20},
	}
	for _, tt := range tests {
		m := &model{width: tt.width}
		if got := m.contentWidth(); got != tt.want {
			t.Errorf("contentWidth() with width=%d = %d, want %d", tt.width, got, tt.want)
		}
	}
}
