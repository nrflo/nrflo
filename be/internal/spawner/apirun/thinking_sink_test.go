package apirun

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// TestRunnerSink_ThinkingCaptureEnabled_YieldsThinkingRow verifies that
// OnThinkingDelta + OnTextDelta + OnToolUseStop produce rows in order
// thinking → text → tool when captureThinking=true.
func TestRunnerSink_ThinkingCaptureEnabled_YieldsThinkingRow(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, true, nil)

	rs.OnThinkingDelta("reasoning here")
	rs.OnTextDelta("response text")
	rs.OnToolUseStart("t1", "Bash")
	rs.OnToolUseStop("t1", json.RawMessage(`{"cmd":"ls"}`))

	calls := sink.Calls()
	// flush triggered by OnToolUseStart (for text) and OnToolUseStop (emits tool row)
	// thinking flushed before text inside flush()
	if len(calls) < 3 {
		t.Fatalf("Calls = %d, want >= 3; got %+v", len(calls), calls)
	}

	// first call must be thinking category
	if calls[0].category != "thinking" {
		t.Errorf("calls[0].category = %q, want thinking; all=%+v", calls[0].category, calls)
	}
	if !strings.Contains(calls[0].content, "reasoning here") {
		t.Errorf("calls[0].content = %q, want 'reasoning here'", calls[0].content)
	}

	// second call must be text
	if calls[1].category != "text" {
		t.Errorf("calls[1].category = %q, want text; all=%+v", calls[1].category, calls)
	}
	if !strings.Contains(calls[1].content, "response text") {
		t.Errorf("calls[1].content = %q, want 'response text'", calls[1].content)
	}

	// third call must be tool category
	if calls[2].category != "tool" {
		t.Errorf("calls[2].category = %q, want tool; all=%+v", calls[2].category, calls)
	}
}

// TestRunnerSink_ThinkingCaptureDisabled_NoThinkingRow verifies that with
// captureThinking=false, no thinking row is emitted; text and tool rows are unchanged.
func TestRunnerSink_ThinkingCaptureDisabled_NoThinkingRow(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)

	rs.OnThinkingDelta("internal reasoning")
	rs.OnTextDelta("visible response")
	rs.OnToolUseStart("t1", "Read")
	rs.OnToolUseStop("t1", json.RawMessage(`{"path":"/x"}`))

	calls := sink.Calls()
	for _, c := range calls {
		if c.category == "thinking" {
			t.Errorf("got thinking row with captureThinking=false: %+v", c)
		}
	}

	// text and tool rows must still be present
	hasText, hasTool := false, false
	for _, c := range calls {
		if c.category == "text" {
			hasText = true
		}
		if c.category == "tool" {
			hasTool = true
		}
	}
	if !hasText {
		t.Errorf("no text row; all=%+v", calls)
	}
	if !hasTool {
		t.Errorf("no tool row; all=%+v", calls)
	}
}

// TestRunnerSink_Thinking_4KBFlush verifies that a thinking delta crossing 4 KB
// triggers an immediate flush (captureThinking=true).
func TestRunnerSink_Thinking_4KBFlush(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, true, nil)

	long := strings.Repeat("t", 5000) // > 4096
	rs.OnThinkingDelta(long)

	calls := sink.Calls()
	if len(calls) != 1 {
		t.Fatalf("Calls = %d, want 1 (immediate 4KB flush); got %+v", len(calls), calls)
	}
	if calls[0].category != "thinking" {
		t.Errorf("calls[0].category = %q, want thinking", calls[0].category)
	}
	if len(calls[0].content) != 5000 {
		t.Errorf("calls[0].content len = %d, want 5000", len(calls[0].content))
	}
}

// TestRunnerSink_Thinking_4KBFlush_CaptureDisabled verifies that a >4KB thinking
// delta does NOT emit any row when captureThinking=false.
func TestRunnerSink_Thinking_4KBFlush_CaptureDisabled(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)

	long := strings.Repeat("t", 5000)
	rs.OnThinkingDelta(long)

	if got := len(sink.Calls()); got != 0 {
		t.Errorf("Calls = %d, want 0 (thinking suppressed); got %+v", got, sink.Calls())
	}
}

// TestRunnerSink_ThinkingFlushedBeforeText verifies that flush() always drains
// thinkBuf before buf, regardless of arrival order.
func TestRunnerSink_ThinkingFlushedBeforeText(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, true, nil)

	// text arrives first, thinking second — flush order must still be think → text
	rs.OnTextDelta("text first")
	rs.OnThinkingDelta("think second")
	rs.OnUsage(provider.Usage{})

	calls := sink.Calls()
	if len(calls) != 2 {
		t.Fatalf("Calls = %d, want 2; got %+v", len(calls), calls)
	}
	if calls[0].category != "thinking" {
		t.Errorf("calls[0].category = %q, want thinking (flushed before text)", calls[0].category)
	}
	if calls[1].category != "text" {
		t.Errorf("calls[1].category = %q, want text", calls[1].category)
	}
}

// TestRunnerSink_EmptyThinkingDelta_Ignored verifies that an empty OnThinkingDelta
// call is a no-op and produces no row even after flush.
func TestRunnerSink_EmptyThinkingDelta_Ignored(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, true, nil)

	rs.OnThinkingDelta("")
	rs.OnUsage(provider.Usage{})

	for _, c := range sink.Calls() {
		if c.category == "thinking" {
			t.Errorf("got thinking row from empty delta: %+v", c)
		}
	}
}
