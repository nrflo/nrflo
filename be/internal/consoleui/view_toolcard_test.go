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

// TestToolCard_LongDelegateBrief_CapsBodyWithForcedEllipsis verifies item 1:
// a large delegate brief is capped at toolCardBodyLines wrapped body lines,
// the cut is always marked via forceEllipsis, and no line exceeds width.
func TestToolCard_LongDelegateBrief_CapsBodyWithForcedEllipsis(t *testing.T) {
	const width = 80
	brief := strings.Repeat("this is a long delegate brief sentence. ", 60) // ~2.4KB
	content := delegateRow("Mcp__nrflo__delegate", brief)

	card := toolCard(content, width)
	lines := strings.Split(card, "\n")

	if len(lines) > 1+toolCardBodyLines {
		t.Fatalf("toolCard produced %d lines, want <= %d (1 head + %d body)", len(lines), 1+toolCardBodyLines, toolCardBodyLines)
	}
	last := lines[len(lines)-1]
	if !strings.HasSuffix(last, "…") {
		t.Errorf("toolCard last line = %q, want it to end with the forced ellipsis marker", last)
	}
	for i, line := range lines {
		if lw := ansi.StringWidth(line); lw > width {
			t.Errorf("toolCard line %d width = %d, want <= %d (line %q)", i, lw, width, line)
		}
		if strings.Contains(line, "\t") {
			t.Errorf("toolCard line %d contains a literal tab: %q", i, line)
		}
	}
}

// TestToolCard_DelegateHeadParams verifies the head line carries
// tier=/fanout=<N>/first-sentence-of-brief, and that the CLI/hook
// title-cased name and the api-mode lowercase name normalize to the same
// params (item 3's name-normalization requirement).
func TestToolCard_DelegateHeadParams(t *testing.T) {
	const brief = "First sentence. Rest of the brief that should not appear."
	cliContent := delegateRow("Mcp__nrflo__delegate", brief)
	apiContent := delegateRow("delegate", brief)

	cliHead := strings.SplitN(toolCard(cliContent, 200), "\n", 2)[0]
	apiHead := strings.SplitN(toolCard(apiContent, 200), "\n", 2)[0]

	for _, want := range []string{toolRowPrefix, "[Mcp__nrflo__delegate]", "tier=executor", "fanout=3", "First sentence."} {
		if !strings.Contains(cliHead, want) {
			t.Errorf("cli head line = %q, want it to contain %q", cliHead, want)
		}
	}
	if strings.Contains(cliHead, "Rest of the brief") {
		t.Errorf("cli head line = %q, want only the first sentence of the brief", cliHead)
	}

	cliParams := toolHeadParams("Mcp__nrflo__delegate", strings.TrimPrefix(cliContent, "[Mcp__nrflo__delegate] "))
	apiParams := toolHeadParams("delegate", strings.TrimPrefix(apiContent, "[delegate] "))
	if cliParams != apiParams {
		t.Errorf("toolHeadParams CLI-name = %q, api-name = %q, want identical params after normalization", cliParams, apiParams)
	}
	if !strings.Contains(apiHead, "[delegate]") || !strings.Contains(apiHead, "tier=executor") {
		t.Errorf("api head line = %q, want the [delegate] name and same params", apiHead)
	}
}

// TestRenderMessage_ToolAndSubagent_SharePrefixAndSkipGlamour verifies item
// 3: tool/tool_use/tool_result/subagent all emit renderMessage output through
// the same toolCard path, sharing one literal prefix, and subagent no longer
// falls through to the glamour default branch (which would strip/transform
// markdown-like content instead of passing it through verbatim).
func TestRenderMessage_ToolAndSubagent_SharePrefixAndSkipGlamour(t *testing.T) {
	const width = 80
	content := "[Task] **bold** general-purpose: investigate the thing"

	categories := []string{"tool", "tool_use", "tool_result", "subagent"}
	var first string
	for i, cat := range categories {
		rendered := renderMessage(Message{Category: cat, Content: content}, width)
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
	// Glamour would render markdown emphasis (strip/transform "**bold**" into
	// an ANSI bold sequence); toolCard passes it through as literal text.
	if !strings.Contains(first, "**bold**") {
		t.Errorf("renderMessage(subagent) = %q, want the literal markdown preserved (not glamour-rendered)", first)
	}
}

// TestToolCard_Fallbacks verifies non-JSON payloads, the bracket-less
// error-row shape, and empty content all degrade gracefully: no panic, a
// bare-name/firstLine head, and no forced-ellipsis marker since nothing was
// truncated.
func TestToolCard_Fallbacks(t *testing.T) {
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
			card := toolCard(tt.content, width)
			if strings.Contains(card, "…") {
				t.Errorf("toolCard(%q) = %q, want no forced-ellipsis marker for untruncated content", tt.content, card)
			}
			for i, line := range strings.Split(card, "\n") {
				if lw := ansi.StringWidth(line); lw > width {
					t.Errorf("toolCard(%q) line %d width = %d, want <= %d", tt.content, i, lw, width)
				}
				if strings.Contains(line, "\t") {
					t.Errorf("toolCard(%q) line %d contains a literal tab", tt.content, i)
				}
			}
		})
	}

	if got := toolCard("", width); got != toolRowPrefix {
		t.Errorf("toolCard(empty) = %q, want bare prefix %q", got, toolRowPrefix)
	}
}

// TestToolCard_ShortPayload_StaysUncappedNoMarker verifies the marker only
// ever means truncation: a short two-line tool result renders both lines
// verbatim with no forced-ellipsis.
func TestToolCard_ShortPayload_StaysUncappedNoMarker(t *testing.T) {
	const width = 80
	content := "[Read] file.go → line one\nline two"
	card := toolCard(content, width)

	if strings.Contains(card, "…") {
		t.Errorf("toolCard(%q) = %q, want no ellipsis marker for a short payload", content, card)
	}
	if !strings.Contains(card, "line one") || !strings.Contains(card, "line two") {
		t.Errorf("toolCard(%q) = %q, want both body lines preserved verbatim", content, card)
	}
}
