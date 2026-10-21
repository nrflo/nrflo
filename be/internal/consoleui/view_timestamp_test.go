package consoleui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderMessageUsesFullWidthWithoutTimestamp(t *testing.T) {
	for _, category := range []string{"user_input", "assistant", "thinking", "tool_result"} {
		msg := Message{Category: category, Content: "[Read] " + strings.Repeat("x", 32), CreatedAt: "2026-08-03T09:15:42Z"}
		got := renderMessage(msg, 40)
		msg.CreatedAt = ""
		if want := renderMessage(msg, 40); got != want {
			t.Errorf("%s timestamp changed output: %q", category, got)
		}
		for _, line := range strings.Split(got, "\n") {
			if ansi.StringWidth(line) > 40 {
				t.Errorf("line exceeds width: %q", line)
			}
		}
	}
}
