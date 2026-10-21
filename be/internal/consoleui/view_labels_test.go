package consoleui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestToolLabelPreservesPayloadColor(t *testing.T) {
	for _, category := range []string{"tool", "tool_use", "tool_result", "subagent"} {
		got := renderMessage(Message{Category: category, Content: "[Bash] pwd"}, 80)
		if ansi.Strip(got) != "[Bash] pwd" ||
			!strings.Contains(got, toolLabelStyle("Bash").Render("[Bash]")) ||
			!strings.HasSuffix(got, toolTextStyle.Render(" pwd")) {
			t.Errorf("%s lost its label or payload color: %q", category, got)
		}
	}
	got := renderMessage(Message{Category: "system_turn", Content: "[nrflo] Done"}, 80)
	if !strings.HasSuffix(got, toolTextStyle.Render(" · Done")) {
		t.Errorf("callback lost its payload color: %q", got)
	}
}
