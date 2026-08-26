package consoleui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// delegateRow builds a "[Name] <compact JSON>" row for delegate's
// tools_builtin/delegate.go arg shape (tier/brief/fanout).
func delegateRow(name, brief string) string {
	return `[` + name + `] {"tier":"executor","brief":"` + brief + `","fanout":["a","b","c"]}`
}

// TestRenderTool_LongDelegateBrief_CapsBodyWithForcedEllipsis verifies the
// unified renderer still caps a large delegate payload at toolBodyLines
// wrapped body lines (plus the head line), marks any cut with forceEllipsis,
// and keeps every line within width and tab-free.
func TestRenderTool_LongDelegateBrief_CapsBodyWithForcedEllipsis(t *testing.T) {
	const width = 80
	brief := strings.Repeat("this is a long delegate brief sentence. ", 60) // ~2.4KB
	content := delegateRow("Mcp__nrflo__delegate", brief)

	rendered := renderMessage(Message{Category: "tool", Content: content}, width)
	lines := strings.Split(ansi.Strip(rendered), "\n")

	if len(lines) > 1+toolBodyLines {
		t.Fatalf("tool row rendered %d lines, want <= %d (1 head + %d body)", len(lines), 1+toolBodyLines, toolBodyLines)
	}
	last := lines[len(lines)-1]
	if !strings.HasSuffix(last, "…") {
		t.Errorf("tool row last line = %q, want it to end with the forced ellipsis marker", last)
	}
	for i, line := range lines {
		if lw := ansi.StringWidth(line); lw > width {
			t.Errorf("tool row line %d width = %d, want <= %d (line %q)", i, lw, width, line)
		}
		if strings.Contains(line, "\t") {
			t.Errorf("tool row line %d contains a literal tab: %q", i, line)
		}
	}
}

// TestRenderMessage_ToolAndSubagent_SharePrefixAndSkipGlamour verifies all
// four tool-family categories emit through the one shared pipeline with the
// same literal prefix, and that markdown-like content passes through verbatim
// (the glamour default branch is gone).
func TestRenderMessage_ToolAndSubagent_SharePrefixAndSkipGlamour(t *testing.T) {
	const width = 80
	content := "[Task] **bold** general-purpose: investigate the thing"

	categories := []string{"tool", "tool_use", "tool_result", "subagent"}
	var first string
	for i, cat := range categories {
		rendered := ansi.Strip(renderMessage(Message{Category: cat, Content: content}, width))
		if !strings.Contains(rendered, toolRowPrefix) {
			t.Errorf("renderMessage(%s) = %q, want it to contain the unified prefix %q", cat, rendered, toolRowPrefix)
		}
		if i == 0 {
			first = rendered
			continue
		}
		if rendered != first {
			t.Errorf("renderMessage(%s) = %q, want identical output to renderMessage(%s) = %q for the same content", cat, rendered, categories[0], first)
		}
	}
	if !strings.Contains(first, "**bold**") {
		t.Errorf("renderMessage(subagent) = %q, want the literal markdown preserved (not markdown-rendered)", first)
	}
}

// TestRenderTool_Fallbacks verifies non-JSON payloads, the bracket-less
// error-row shape, and empty content all degrade gracefully: no panic, a
// bare-name/firstLine head, and no forced-ellipsis marker since nothing was
// truncated.
func TestRenderTool_Fallbacks(t *testing.T) {
	const width = 80
	tests := []struct {
		name    string
		content string
	}{
		{"non-JSON curated invoke", "[Bash] ls -la"},
		{"bracket-less error row", "Delegate: boom"},
		{"empty content", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := ansi.Strip(renderMessage(Message{Category: "tool", Content: tt.content}, width))
			if strings.Contains(card, "…") {
				t.Errorf("renderMessage(tool, %q) = %q, want no forced-ellipsis marker for untruncated content", tt.content, card)
			}
			for i, line := range strings.Split(card, "\n") {
				if lw := ansi.StringWidth(line); lw > width {
					t.Errorf("renderMessage(tool, %q) line %d width = %d, want <= %d", tt.content, i, lw, width)
				}
				if strings.Contains(line, "\t") {
					t.Errorf("renderMessage(tool, %q) line %d contains a literal tab", tt.content, i)
				}
			}
		})
	}

	if got := ansi.Strip(renderMessage(Message{Category: "tool", Content: ""}, width)); got != toolRowPrefix {
		t.Errorf("renderMessage(tool, empty) = %q, want bare prefix %q", got, toolRowPrefix)
	}
}

// TestRenderTool_ShortPayload_StaysUncappedNoMarker verifies the marker only
// ever means truncation: a short two-line tool result renders both lines
// verbatim with no forced-ellipsis.
func TestRenderTool_ShortPayload_StaysUncappedNoMarker(t *testing.T) {
	const width = 80
	content := "[Read] file.go → line one\nline two"
	card := ansi.Strip(renderMessage(Message{Category: "tool", Content: content}, width))

	if strings.Contains(card, "…") {
		t.Errorf("renderMessage(tool, %q) = %q, want no ellipsis marker for a short payload", content, card)
	}
	if !strings.Contains(card, "line one") || !strings.Contains(card, "line two") {
		t.Errorf("renderMessage(tool, %q) = %q, want both body lines preserved verbatim", content, card)
	}
}
