package spawner

import (
	"strings"
	"testing"
)

// TestHandleTextMessage_LongText_NoTruncation verifies that text messages longer
// than 500 chars are stored and logged in full (no " ... [" truncation markers).
func TestHandleTextMessage_LongText_NoTruncation(t *testing.T) {
	t.Parallel()
	s := noPoolSpawner()
	proc := minProc("sess-notrunc-1")
	buf := captureLog(t)

	longText := strings.Repeat("x", 1000)

	// Feed a Claude assistant event with text length 1000
	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": longText,
				},
			},
		},
	})

	// Verify TrackMessage received the full text
	msgs := pendingMessages(proc)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 pending message, got %d", len(msgs))
	}
	if msgs[0] != longText {
		t.Errorf("TrackMessage received truncated text: len=%d, want %d", len(msgs[0]), len(longText))
	}
	if strings.Contains(msgs[0], " ... [") {
		t.Errorf("TrackMessage content contains truncation marker ' ... [': %q", msgs[0][:50])
	}

	// Verify log output contains the full text (not truncated)
	logOut := buf.String()
	if strings.Contains(logOut, " ... [") {
		t.Errorf("logAgent output contains truncation marker ' ... [': %s", logOut[:100])
	}
	if !strings.Contains(logOut, longText) {
		t.Errorf("logAgent output does not contain the full text (len=%d)", len(longText))
	}
}

// TestHandleTextMessage_ExactBoundary_500_NoTruncation verifies text at exactly
// 500 chars is stored and logged in full.
func TestHandleTextMessage_ExactBoundary_500_NoTruncation(t *testing.T) {
	t.Parallel()
	s := noPoolSpawner()
	proc := minProc("sess-notrunc-2")

	text500 := strings.Repeat("a", 500)

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": text500,
				},
			},
		},
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 pending message, got %d", len(msgs))
	}
	if len(msgs[0]) != 500 {
		t.Errorf("expected len=500, got %d", len(msgs[0]))
	}
}

// TestHandleTextMessage_BeyondBoundary_501_NoTruncation verifies text at 501 chars
// (one beyond the old threshold) is stored in full.
func TestHandleTextMessage_BeyondBoundary_501_NoTruncation(t *testing.T) {
	t.Parallel()
	s := noPoolSpawner()
	proc := minProc("sess-notrunc-3")

	text501 := strings.Repeat("b", 501)

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": text501,
				},
			},
		},
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 pending message, got %d", len(msgs))
	}
	if len(msgs[0]) != 501 {
		t.Errorf("expected len=501 (no truncation), got %d", len(msgs[0]))
	}
	if strings.Contains(msgs[0], "...") {
		t.Errorf("message contains '...' truncation marker: %q", msgs[0][:50])
	}
}

// TestFormatToolDetail_LongDetail_NoTruncation verifies FormatToolDetail with a detail
// longer than 200 chars returns the full string without trailing "...".
func TestFormatToolDetail_LongDetail_NoTruncation(t *testing.T) {
	t.Parallel()

	longCmd := strings.Repeat("a", 500)
	result := FormatToolDetail("Bash", map[string]interface{}{
		"command": longCmd,
	})

	want := "[Bash] " + longCmd
	if result != want {
		t.Errorf("FormatToolDetail returned %d chars, want %d chars", len(result), len(want))
	}
	if strings.HasSuffix(result, "...") {
		t.Errorf("FormatToolDetail result has trailing '...': len=%d", len(result))
	}
}

// TestFormatToolDetail_ExactBoundary_200_NoTruncation verifies a detail of exactly
// 200 chars is returned in full.
func TestFormatToolDetail_ExactBoundary_200_NoTruncation(t *testing.T) {
	t.Parallel()

	cmd200 := strings.Repeat("c", 200)
	result := FormatToolDetail("Bash", map[string]interface{}{
		"command": cmd200,
	})

	want := "[Bash] " + cmd200
	if result != want {
		t.Errorf("FormatToolDetail(200-char detail) = len(%d), want len(%d)", len(result), len(want))
	}
}

// TestFormatToolDetail_Beyond200_NoTruncation verifies a detail of 201 chars
// (one beyond the old threshold) is returned in full.
func TestFormatToolDetail_Beyond200_NoTruncation(t *testing.T) {
	t.Parallel()

	cmd201 := strings.Repeat("d", 201)
	result := FormatToolDetail("Bash", map[string]interface{}{
		"command": cmd201,
	})

	want := "[Bash] " + cmd201
	if result != want {
		t.Errorf("FormatToolDetail(201-char detail): got len=%d, want len=%d", len(result), len(want))
	}
	if strings.HasSuffix(result, "...") {
		t.Errorf("FormatToolDetail(201-char detail) should not end with '...'")
	}
}

// TestFormatToolDetail_AllTools_LongDetail verifies truncation is absent across
// all tool types that extract a detail field.
func TestFormatToolDetail_AllTools_LongDetail(t *testing.T) {
	t.Parallel()

	longVal := strings.Repeat("z", 300)

	tests := []struct {
		toolName string
		input    map[string]interface{}
		wantSuffix string
	}{
		{"Bash", map[string]interface{}{"command": longVal}, longVal},
		{"Read", map[string]interface{}{"file_path": longVal}, longVal},
		{"Write", map[string]interface{}{"file_path": longVal}, longVal},
		{"Edit", map[string]interface{}{"file_path": longVal}, longVal},
		{"Grep", map[string]interface{}{"pattern": longVal}, longVal},
		{"WebFetch", map[string]interface{}{"url": longVal}, longVal},
		{"WebSearch", map[string]interface{}{"query": longVal}, longVal},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			result := FormatToolDetail(tt.toolName, tt.input)
			want := "[" + tt.toolName + "] " + tt.wantSuffix
			if result != want {
				t.Errorf("FormatToolDetail(%s, long input): got len=%d, want len=%d",
					tt.toolName, len(result), len(want))
			}
			if strings.HasSuffix(result, "...") {
				t.Errorf("FormatToolDetail(%s) result ends with '...' (truncated)", tt.toolName)
			}
		})
	}
}

// TestHandleClaudeToolResult_LongDescription_StillTruncated verifies the out-of-scope
// handleClaudeToolResult 200-char cap on TaskResult/AgentResult was NOT changed
// (regression guard).
func TestHandleClaudeToolResult_LongDescription_StillTruncated(t *testing.T) {
	t.Parallel()
	s := noPoolSpawner()
	proc := minProc("sess-tasktrunc-1")

	longDesc := strings.Repeat("e", 500)

	// Register a Task tool use
	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":  "tool_use",
					"id":    "toolu_tasktrunc",
					"name":  "Task",
					"input": map[string]interface{}{"description": longDesc},
				},
			},
		},
	})

	// Correlate with tool_result
	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "toolu_tasktrunc",
	})

	entries := pendingEntries(proc)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	msg := entries[1].Content
	// handleClaudeToolResult still truncates at 200 chars: "[TaskResult] " (13) + 200 + "..." (3) = 216
	const wantLen = 216
	if len(msg) != wantLen {
		t.Errorf("handleClaudeToolResult TaskResult: expected len=%d (still truncated), got %d: %q",
			wantLen, len(msg), msg)
	}
	if !strings.HasSuffix(msg, "...") {
		t.Errorf("handleClaudeToolResult TaskResult: expected trailing '...', got: %q", msg)
	}
}
