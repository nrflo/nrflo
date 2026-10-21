package consoleui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderToolBlocks(t *testing.T) {
	for _, category := range []string{"tool", "tool_use", "tool_result", "subagent"} {
		got := ansi.Strip(renderMessage(Message{Category: category, Content: "[Read] file.go → line one\nline two"}, 80))
		if got != "[Read] file.go → line one\n  line two" {
			t.Errorf("%s = %q", category, got)
		}
	}
	if got := renderMessage(Message{Category: "tool"}, 80); got != "" {
		t.Errorf("empty tool = %q", got)
	}
	got := ansi.Strip(renderMessage(Message{Category: "tool", Content: "[Bash] pwd\n/tmp"}, 80))
	if got != "[Bash] pwd\n  /tmp" {
		t.Errorf("command output lost its line break: %q", got)
	}
}

func TestRenderCommandUsesStructuredInputAndShowsOutput(t *testing.T) {
	command := "grep -n \"hello world\" \"$HOME/file.go\" | head -5"
	payload, _ := json.Marshal(map[string]any{"input": map[string]string{"command": command}})
	got := renderMessage(Message{Category: "tool", Content: "[Bash] " + command + " (exit 0)\nfound\nsecond line", Payload: string(payload)}, 120)
	plain := ansi.Strip(got)
	if plain != "[Bash] "+command+" (exit 0)\n  found\n  second line" {
		t.Errorf("command block = %q", plain)
	}
	if !strings.Contains(got, "38;5;114") {
		t.Errorf("quoted arguments lack syntax color: %q", got)
	}
}

func TestRenderToolPreviewBounds(t *testing.T) {
	message := Message{Category: "tool", Content: "[Task] " + strings.Repeat("long payload\n", 100)}
	got := ansi.Strip(renderMessage(message, 40))
	if rows := len(strings.Split(got, "\n")); rows > toolPreviewRows+1 {
		t.Errorf("preview rows = %d", rows)
	}
	if !strings.Contains(got, "more lines") {
		t.Errorf("missing omission marker: %q", got)
	}
}

func TestRenderToolWidthsAndMalformedPayloads(t *testing.T) {
	for _, width := range []int{1, 2, 8, 40, 80} {
		for _, payload := range []string{"", "broken json", `{\"input\":null}`} {
			msg := Message{Category: "tool", Content: "[Bash] echo \"界🙂\"\t" + strings.Repeat("a", 100) + "\noutput", Payload: payload}
			got := renderMessage(msg, width)
			if strings.Contains(got, "\t") {
				t.Error("literal tab in tool block")
			}
			for _, line := range strings.Split(got, "\n") {
				if ansi.StringWidth(line) > width {
					t.Errorf("width %d exceeded by %q", width, line)
				}
			}
		}
	}
}
