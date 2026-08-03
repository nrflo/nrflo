package consoleui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// A row with a persisted created_at renders with a local HH:MM:SS prefix on
// its first line, continuation lines indented to align, and every line still
// within width.
func TestRenderMessage_TimestampPrefix(t *testing.T) {
	const width = 40
	createdAt := time.Date(2026, 8, 3, 9, 15, 42, 0, time.UTC)
	msg := Message{
		Content:   strings.Repeat("wrap me across several lines please ", 4),
		Category:  "user_input",
		CreatedAt: createdAt.Format(time.RFC3339Nano),
	}
	got := ansi.Strip(renderMessage(msg, width))
	want := createdAt.Local().Format("15:04:05") + " "
	lines := strings.Split(got, "\n")
	if !strings.HasPrefix(lines[0], want) {
		t.Errorf("first line = %q, want prefix %q (local time)", lines[0], want)
	}
	if len(lines) < 2 {
		t.Fatalf("content did not wrap: %q", got)
	}
	for i, line := range lines[1:] {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, strings.Repeat(" ", timePrefixWidth)) {
			t.Errorf("continuation line %d = %q, want %d-space indent", i+1, line, timePrefixWidth)
		}
	}
	for i, line := range strings.Split(renderMessage(msg, width), "\n") {
		if w := ansi.StringWidth(line); w > width {
			t.Errorf("line %d width = %d, want <= %d", i, w, width)
		}
	}
}

// Rows without a parseable created_at render exactly as before — no prefix,
// full width.
func TestRenderMessage_NoTimestamp_Unchanged(t *testing.T) {
	msg := Message{Content: "hello", Category: "user_input"}
	if got, want := ansi.Strip(renderMessage(msg, 40)), "hello"; got != want {
		t.Errorf("renderMessage(no created_at) = %q, want %q", got, want)
	}
	msg.CreatedAt = "not-a-time"
	if got, want := ansi.Strip(renderMessage(msg, 40)), "hello"; got != want {
		t.Errorf("renderMessage(bad created_at) = %q, want %q", got, want)
	}
}

// A terminal too narrow for the timestamp column drops the prefix rather
// than squeezing the body below a readable width.
func TestRenderMessage_NarrowWidth_DropsPrefix(t *testing.T) {
	msg := Message{
		Content:   "hi",
		Category:  "user_input",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if got, want := ansi.Strip(renderMessage(msg, 20)), "hi"; got != want {
		t.Errorf("renderMessage(narrow) = %q, want %q (no prefix)", got, want)
	}
}

// system_notice renders "" with or without a timestamp — the skip contract
// printNewMessages relies on.
func TestRenderMessage_SystemNotice_StaysEmpty(t *testing.T) {
	msg := Message{
		Content:   "ignored",
		Category:  "system_notice",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if got := renderMessage(msg, 80); got != "" {
		t.Errorf("renderMessage(system_notice) = %q, want empty", got)
	}
}
