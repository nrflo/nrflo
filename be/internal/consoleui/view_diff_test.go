package consoleui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestEditDiffFromStructuredInput(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"input": map[string]string{
		"file_path": "main.go", "old_string": "package main\nvar x = 1\n", "new_string": "package main\nvar x = 2\n",
	}})
	got := renderMessage(Message{Category: "tool", Content: "[Edit] main.go", Payload: string(payload)}, 100)
	plain := ansi.Strip(got)
	for _, want := range []string{"[Edit] main.go", "2 - var x = 1", "2 + var x = 2"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q: %q", want, plain)
		}
	}
	for _, color := range []string{"48;5;22", "48;5;52"} {
		if !strings.Contains(got, color) {
			t.Errorf("diff lacks background %s: %q", color, got)
		}
	}
}

func TestPersistedDiffAndDeletedText(t *testing.T) {
	for _, input := range []map[string]string{
		{"file_path": "a.txt", "diff": "@@ -10,1 +10,1 @@\n-old\n+new"},
		{"file_path": "a.txt", "old_string": "old\n", "new_string": ""},
	} {
		payload, _ := json.Marshal(map[string]any{"input": input})
		got := ansi.Strip(renderMessage(Message{Category: "tool", Content: "[Edit] a.txt", Payload: string(payload)}, 40))
		if !strings.Contains(got, "- old") {
			t.Errorf("deletion not rendered: %q", got)
		}
	}
}

func TestDiffFallbackAndWidth(t *testing.T) {
	content := "[Apply_patch] *** Begin Patch\n*** Update File: a.go\n@@\n-\told\n+\t" + strings.Repeat("界", 100) + "\n*** End Patch"
	for _, width := range []int{1, 12, 40} {
		got := renderMessage(Message{Category: "tool", Content: content}, width)
		for _, line := range strings.Split(got, "\n") {
			if ansi.StringWidth(line) > width || strings.Contains(line, "\t") {
				t.Errorf("invalid diff line at width %d: %q", width, line)
			}
		}
	}
}
