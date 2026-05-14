package spawner

import (
	"fmt"
	"strings"
	"testing"
)

// TestClassifyOpencodePart verifies that classifyOpencodePart returns the full
// content without byte-cap truncation. Each case uses text > 1024 bytes (the
// removed old cap) to catch any surviving truncation.
func TestClassifyOpencodePart(t *testing.T) {
	longText := strings.Repeat("a", 1200)     // old cap was 1024 B
	longReason := strings.Repeat("b", 1200)   // old cap was 1024 B
	longCommand := strings.Repeat("c", 1200)  // old cap was 1024 B

	cases := []struct {
		name         string
		raw          string
		wantContent  string
		wantCategory string
		wantOK       bool
	}{
		{
			name:         "text_type_full_content",
			raw:          fmt.Sprintf(`{"type":"text","text":%q}`, longText),
			wantContent:  longText,
			wantCategory: "text",
			wantOK:       true,
		},
		{
			name:         "reasoning_type_maps_to_thinking",
			raw:          fmt.Sprintf(`{"type":"reasoning","text":%q}`, longReason),
			wantContent:  longReason,
			wantCategory: "thinking",
			wantOK:       true,
		},
		{
			name: "tool_type_full_command",
			raw: fmt.Sprintf(
				`{"type":"tool","tool":"bash","state":{"input":{"command":%q}}}`,
				longCommand,
			),
			wantContent:  "bash: " + longCommand,
			wantCategory: "tool",
			wantOK:       true,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			content, category, payload, ok := classifyOpencodePart(c.raw)

			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if content != c.wantContent {
				t.Errorf("content length %d, want %d (truncated=%v)",
					len(content), len(c.wantContent), len(content) < len(c.wantContent))
			}
			if strings.HasSuffix(content, "…") || strings.HasSuffix(content, "...") {
				t.Errorf("content must not end with ellipsis, got suffix %q", content[len(content)-4:])
			}
			if category != c.wantCategory {
				t.Errorf("category = %q, want %q", category, c.wantCategory)
			}
			// payload must be the verbatim raw JSON for non-skipped parts.
			if payload != c.raw {
				snippet := payload
				if len(snippet) > 40 {
					snippet = snippet[:40]
				}
				t.Errorf("payload = %q, want raw JSON (len %d)", snippet, len(c.raw))
			}
		})
	}
}

// TestClassifyOpencodePart_SkippedTypes verifies that step-start, step-finish,
// unknown, and empty-text parts return ok=false so the caller skips them.
func TestClassifyOpencodePart_SkippedTypes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"step_start", `{"type":"step-start"}`},
		{"step_finish", `{"type":"step-finish"}`},
		{"unknown_type", `{"type":"blob"}`},
		{"text_empty_string", `{"type":"text","text":""}`},
		{"reasoning_empty_string", `{"type":"reasoning","text":""}`},
		{"invalid_json", `not-json`},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			_, _, _, ok := classifyOpencodePart(c.raw)
			if ok {
				t.Errorf("%s: expected ok=false (part should be skipped)", c.name)
			}
		})
	}
}

