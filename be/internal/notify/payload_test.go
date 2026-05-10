package notify

import (
	"strings"
	"testing"

	"be/internal/ws"
)

func TestRenderSlack_AllWatchedEventTypes(t *testing.T) {
	tests := []struct {
		eventType string
		data      map[string]interface{}
		wantSub   string
	}{
		{
			ws.EventOrchestrationCompleted,
			map[string]interface{}{"ticket_id": "TICK-1", "workflow": "feature"},
			"feature",
		},
		{
			ws.EventOrchestrationFailed,
			map[string]interface{}{"ticket_id": "TICK-2", "workflow": "bugfix", "reason": "timeout"},
			"bugfix",
		},
		{
			ws.EventAgentCompleted,
			map[string]interface{}{"agent_type": "implementor", "workflow": "feature"},
			"implementor",
		},
		{
			ws.EventAgentContextSaving,
			map[string]interface{}{"agent_type": "qa-verifier", "workflow": "feature"},
			"qa-verifier",
		},
		{
			ws.EventAgentStallRestart,
			map[string]interface{}{"agent_type": "doc-updater", "workflow": "docs"},
			"doc-updater",
		},
	}

	for _, tc := range tests {
		t.Run(tc.eventType, func(t *testing.T) {
			result := renderSlack(tc.eventType, tc.data)
			if !strings.Contains(result, "*nrflo*") {
				t.Errorf("renderSlack(%q): missing '*nrflo*', got %q", tc.eventType, result)
			}
			if !strings.Contains(result, tc.eventType) {
				t.Errorf("renderSlack(%q): missing event type in header, got %q", tc.eventType, result)
			}
			if !strings.Contains(result, tc.wantSub) {
				t.Errorf("renderSlack(%q): missing %q in %q", tc.eventType, tc.wantSub, result)
			}
		})
	}
}

func TestRenderTelegram_AllWatchedEventTypes(t *testing.T) {
	tests := []struct {
		eventType string
		data      map[string]interface{}
		wantSub   string
	}{
		{
			ws.EventOrchestrationCompleted,
			map[string]interface{}{"ticket_id": "TICK-1", "workflow": "feature"},
			"feature",
		},
		{
			ws.EventOrchestrationFailed,
			map[string]interface{}{"workflow": "bugfix"},
			"bugfix",
		},
		{
			ws.EventAgentCompleted,
			map[string]interface{}{"agent_type": "implementor", "workflow": "feature"},
			"implementor",
		},
		{
			ws.EventAgentContextSaving,
			map[string]interface{}{"agent_type": "qa", "workflow": "feature"},
			"qa",
		},
		{
			ws.EventAgentStallRestart,
			map[string]interface{}{"agent_type": "setup", "workflow": "refactor"},
			"setup",
		},
	}

	for _, tc := range tests {
		t.Run(tc.eventType, func(t *testing.T) {
			result := renderTelegram(tc.eventType, tc.data)
			if !strings.Contains(result, "*nrflo*") {
				t.Errorf("renderTelegram(%q): missing '*nrflo*', got %q", tc.eventType, result)
			}
			if !strings.Contains(result, tc.wantSub) {
				t.Errorf("renderTelegram(%q): missing %q in %q", tc.eventType, tc.wantSub, result)
			}
		})
	}
}

func TestEscapeTelegramV2_AllSpecialChars(t *testing.T) {
	special := []rune{'_', '[', ']', '(', ')', '~', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!'}
	for _, ch := range special {
		input := string(ch)
		result := escapeTelegramV2(input)
		escaped := `\` + string(ch)
		if result != escaped {
			t.Errorf("escapeTelegramV2(%q) = %q, want %q", input, result, escaped)
		}
	}
}

func TestEscapeTelegramV2_Passthrough_NoSpecialChars(t *testing.T) {
	input := "Hello World abc123"
	result := escapeTelegramV2(input)
	if result != input {
		t.Errorf("escapeTelegramV2(%q) = %q, want unchanged", input, result)
	}
}

func TestRenderTelegram_SpecialCharsEscaped(t *testing.T) {
	result := renderTelegram(ws.EventOrchestrationCompleted, map[string]interface{}{
		"ticket_id": "TICK_1",
		"workflow":  "my-workflow",
	})
	// Underscore in ticket_id link text should be escaped as \_
	if !strings.Contains(result, `\_`) {
		t.Errorf("underscore not escaped in: %q", result)
	}
	// Hyphen in workflow name should be escaped as \-
	if !strings.Contains(result, `\-`) {
		t.Errorf("hyphen not escaped in: %q", result)
	}
}

func TestRenderSlack_UnknownEventType(t *testing.T) {
	result := renderSlack("unknown.event", map[string]interface{}{})
	if !strings.Contains(result, "*nrflo*") {
		t.Errorf("missing '*nrflo*' for unknown event: %q", result)
	}
	if !strings.Contains(result, "unknown.event") {
		t.Errorf("missing event type in header: %q", result)
	}
}

func TestRenderSlack_WithReason(t *testing.T) {
	result := renderSlack(ws.EventOrchestrationFailed, map[string]interface{}{
		"workflow": "feature",
		"reason":   "agent timeout",
	})
	if !strings.Contains(result, "agent timeout") {
		t.Errorf("reason missing from: %q", result)
	}
}

func TestRenderSlack_TicketLink(t *testing.T) {
	result := renderSlack(ws.EventOrchestrationCompleted, map[string]interface{}{
		"ticket_id": "TICK-1",
		"workflow":  "feature",
	})
	wantLink := "<" + NotificationBaseURL + "/tickets/TICK-1|TICK-1>"
	if !strings.Contains(result, wantLink) {
		t.Errorf("expected ticket link %q in: %q", wantLink, result)
	}
}

func TestRenderTelegram_TicketLink(t *testing.T) {
	result := renderTelegram(ws.EventOrchestrationCompleted, map[string]interface{}{
		"ticket_id": "TICK-1",
		"workflow":  "feature",
	})
	wantLink := "[TICK\\-1](" + NotificationBaseURL + "/tickets/TICK-1)"
	if !strings.Contains(result, wantLink) {
		t.Errorf("expected ticket link %q in: %q", wantLink, result)
	}
}

func TestRenderSlack_ProjectScopedLink(t *testing.T) {
	result := renderSlack(ws.EventOrchestrationCompleted, map[string]interface{}{
		"workflow":    "deploy",
		"instance_id": "wfi-abc",
	})
	wantLink := "<" + NotificationBaseURL + "/project-workflows?instance_id=wfi-abc|project>"
	if !strings.Contains(result, wantLink) {
		t.Errorf("expected project link %q in: %q", wantLink, result)
	}
}

func TestRenderSlack_NoLinkWhenNoTicketAndNoInstance(t *testing.T) {
	result := renderSlack(ws.EventOrchestrationCompleted, map[string]interface{}{
		"workflow": "deploy",
	})
	if strings.Contains(result, NotificationBaseURL) {
		t.Errorf("expected no link when no scope: %q", result)
	}
}

func TestTruncateRunes_TruncatesLongString(t *testing.T) {
	s := strings.Repeat("a", 2000)
	result := truncateRunes(s, 1500)
	runes := []rune(result)
	if len(runes) != 1501 {
		t.Errorf("expected 1501 runes (1500 + ellipsis), got %d", len(runes))
	}
	if !strings.HasSuffix(result, "…") {
		t.Errorf("expected result to end with '…', got %q", result[len(result)-10:])
	}
}

func TestTruncateRunes_PassthroughWhenShort(t *testing.T) {
	s := "short string"
	result := truncateRunes(s, 1500)
	if result != s {
		t.Errorf("expected passthrough for short string, got %q", result)
	}
}

func TestRenderSummaryBlock_LinesPrefixedWithChevron(t *testing.T) {
	identity := func(s string) string { return s }
	result := renderSummaryBlock("line one\nline two", identity)
	for _, line := range strings.Split(result, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "> ") {
			t.Errorf("line %q does not start with '> '", line)
		}
	}
}

func TestRenderSummaryBlock_MultiLinePreservesNewlines(t *testing.T) {
	identity := func(s string) string { return s }
	result := renderSummaryBlock("first\nsecond\nthird", identity)
	if !strings.Contains(result, "\n") {
		t.Errorf("expected multiline output, got single line: %q", result)
	}
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	if len(lines) < 3 {
		t.Errorf("expected at least 3 lines in output, got %d: %q", len(lines), result)
	}
}

func TestRenderSlack_SummaryBlockOnOrchestrationCompleted(t *testing.T) {
	data := map[string]interface{}{
		"workflow_final_result": "my result",
	}
	result := renderSlack(ws.EventOrchestrationCompleted, data)
	if !strings.Contains(result, "> my result") {
		t.Errorf("expected '> my result' in output, got %q", result)
	}
}

func TestRenderSlack_NoSummaryOnOrchestrationFailed(t *testing.T) {
	data := map[string]interface{}{
		"workflow_final_result": "hi",
	}
	result := renderSlack(ws.EventOrchestrationFailed, data)
	if strings.Contains(result, "> hi") {
		t.Errorf("expected no summary block for orchestration.failed, but got '> hi' in %q", result)
	}
}

func TestRenderSlack_NoSummaryWhenFieldAbsent(t *testing.T) {
	result := renderSlack(ws.EventOrchestrationCompleted, map[string]interface{}{})
	if strings.Contains(result, "> ") {
		t.Errorf("expected no summary block when workflow_final_result absent, got %q", result)
	}
}

func TestRenderTelegram_SummaryEscapesDots(t *testing.T) {
	data := map[string]interface{}{
		"workflow_final_result": "v1.0",
	}
	result := renderTelegram(ws.EventOrchestrationCompleted, data)
	if !strings.Contains(result, `\.`) {
		t.Errorf("expected dot to be escaped as '\\.' in MarkdownV2, got %q", result)
	}
}

func TestRenderSlack_SummaryTruncated(t *testing.T) {
	summary := strings.Repeat("x", 2000)
	data := map[string]interface{}{
		"workflow_final_result": summary,
	}
	result := renderSlack(ws.EventOrchestrationCompleted, data)
	if len([]rune(result)) >= 2100 {
		t.Errorf("expected total output under 2100 runes, got %d", len([]rune(result)))
	}
	if !strings.Contains(result, "…") {
		t.Errorf("expected truncation ellipsis '…' in output, got %q", result[:100])
	}
}
