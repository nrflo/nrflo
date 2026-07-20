package apirun

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// TestRunnerSink_TextDeltas_BelowThreshold_FlushOnUsage verifies that small
// fragmented deltas (well below 4 KB) stay buffered and produce one TrackMessage
// call when OnUsage flushes.
func TestRunnerSink_TextDeltas_BelowThreshold_FlushOnUsage(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)
	t.Cleanup(rs.close)

	rs.OnTextDelta("a")
	rs.OnTextDelta("b")
	rs.OnTextDelta("c")
	rs.OnTextDelta("d")
	rs.OnTextDelta("e")

	if got := len(sink.Calls()); got != 0 {
		t.Errorf("Calls before flush = %d, want 0", got)
	}

	rs.OnUsage(provider.Usage{})

	calls := sink.Calls()
	if len(calls) != 1 {
		t.Fatalf("Calls after OnUsage = %d, want 1; got %+v", len(calls), calls)
	}
	if calls[0].content != "abcde" || calls[0].category != "text" {
		t.Errorf("call[0] = {content:%q, category:%q}, want {abcde, text}", calls[0].content, calls[0].category)
	}
}

// TestRunnerSink_TextDeltas_4KBSafetyCapFlushes verifies that crossing the 4 KB
// buffer cap flushes immediately without waiting for OnUsage.
func TestRunnerSink_TextDeltas_4KBSafetyCapFlushes(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)
	t.Cleanup(rs.close)

	long := strings.Repeat("x", 5000) // > 4096
	rs.OnTextDelta(long)

	calls := sink.Calls()
	if len(calls) != 1 {
		t.Fatalf("Calls = %d, want 1 (immediate 4KB safety flush); got %+v", len(calls), calls)
	}
	if calls[0].content != long {
		t.Errorf("call[0].content len = %d, want %d", len(calls[0].content), len(long))
	}
	if calls[0].category != "text" {
		t.Errorf("call[0].category = %q, want text", calls[0].category)
	}
}

// TestRunnerSink_TextDeltas_FragmentsBelowCap verifies that fragments totalling
// less than 4 KB are all buffered and flushed in a single call on OnUsage.
func TestRunnerSink_TextDeltas_FragmentsBelowCap(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)
	t.Cleanup(rs.close)

	frag := strings.Repeat("y", 20)
	for i := 0; i < 5; i++ {
		rs.OnTextDelta(frag)
	}

	if got := len(sink.Calls()); got != 0 {
		t.Errorf("Calls before OnUsage = %d, want 0 (100 chars is well below 4 KB)", got)
	}

	rs.OnUsage(provider.Usage{})

	calls := sink.Calls()
	if len(calls) != 1 {
		t.Errorf("Calls = %d, want 1; got %+v", len(calls), calls)
	}
	combined := ""
	for _, c := range calls {
		if c.category != "text" {
			t.Errorf("category = %q, want text", c.category)
		}
		combined += c.content
	}
	if combined != strings.Repeat("y", 100) {
		t.Errorf("combined text = %q, want 100 y's", combined)
	}
}

// TestRunnerSink_EmptyDeltaIgnored verifies that an empty text delta is a no-op.
func TestRunnerSink_EmptyDeltaIgnored(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)
	t.Cleanup(rs.close)

	rs.OnTextDelta("")
	rs.OnUsage(provider.Usage{})

	if got := len(sink.Calls()); got != 0 {
		t.Errorf("Calls = %d, want 0 (empty delta should not buffer)", got)
	}
}

// TestRunnerSink_ToolUseStart_FlushesBufferOnly verifies that a pending text
// buffer is flushed before OnToolUseStart, but no tool_use_start row is emitted.
func TestRunnerSink_ToolUseStart_FlushesBufferOnly(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)
	t.Cleanup(rs.close)

	rs.OnTextDelta("preamble")
	rs.OnToolUseStart("tool-1", "Bash")

	calls := sink.Calls()
	if len(calls) != 1 {
		t.Fatalf("Calls = %d, want 1 (text flush only, no tool_use_start row); got %+v", len(calls), calls)
	}
	if calls[0].content != "preamble" || calls[0].category != "text" {
		t.Errorf("call[0] = {%q, %q}, want {preamble, text}", calls[0].content, calls[0].category)
	}
}

// TestRunnerSink_ToolUseStartStop_EmitsSingleRow verifies that a start+stop pair
// yields exactly one [<name>] <input> row with category tool (no tool_use_start
// or tool_use_input rows emitted).
func TestRunnerSink_ToolUseStartStop_EmitsSingleRow(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)
	t.Cleanup(rs.close)

	rs.OnToolUseStart("tool-1", "Bash")
	rs.OnToolUseStop("tool-1", json.RawMessage(`{"command":"ls"}`))

	calls := sink.Calls()
	if len(calls) != 1 {
		t.Fatalf("Calls = %d, want 1 ([Bash] input row); got %+v", len(calls), calls)
	}
	if calls[0].category != "tool" {
		t.Errorf("call[0].category = %q, want tool", calls[0].category)
	}
	if !strings.Contains(calls[0].content, "[Bash]") {
		t.Errorf("call[0].content = %q, want [Bash] prefix", calls[0].content)
	}
	if !strings.Contains(calls[0].content, `"command":"ls"`) {
		t.Errorf("call[0].content = %q, want JSON input", calls[0].content)
	}
	if string(calls[0].rawInput) != `{"command":"ls"}` {
		t.Errorf("call[0].rawInput = %q, want the raw tool input to reach the sink", calls[0].rawInput)
	}
}

// TestRunnerSink_ToolUseStop_FlushesBufferThenEmitsInput verifies that
// OnToolUseStop flushes pending text and emits the [<name>] <input> row.
func TestRunnerSink_ToolUseStop_FlushesBufferThenEmitsInput(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)
	t.Cleanup(rs.close)

	rs.OnToolUseStart("tool-1", "Bash")
	rs.OnTextDelta("more text")
	rs.OnToolUseStop("tool-1", json.RawMessage(`{"command":"ls"}`))

	calls := sink.Calls()
	if len(calls) != 2 {
		t.Fatalf("Calls = %d, want 2 (text flush + tool row); got %+v", len(calls), calls)
	}
	if calls[0].content != "more text" || calls[0].category != "text" {
		t.Errorf("call[0] = {%q, %q}, want {more text, text}", calls[0].content, calls[0].category)
	}
	if calls[1].category != "tool" {
		t.Errorf("call[1].category = %q, want tool", calls[1].category)
	}
	if !strings.Contains(calls[1].content, `"command":"ls"`) {
		t.Errorf("call[1].content = %q, want JSON input", calls[1].content)
	}
	if !strings.Contains(calls[1].content, "[Bash]") {
		t.Errorf("call[1].content = %q, want [Bash] prefix", calls[1].content)
	}
}

// TestRunnerSink_ToolUseInputDelta_Discarded verifies partial JSON deltas are
// discarded (no TrackMessage emitted).
func TestRunnerSink_ToolUseInputDelta_Discarded(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)
	t.Cleanup(rs.close)

	rs.OnToolUseInputDelta("tool-1", `{"cmd":`)
	rs.OnToolUseInputDelta("tool-1", `"ls"}`)

	if got := len(sink.Calls()); got != 0 {
		t.Errorf("Calls = %d, want 0 (partial deltas should be discarded)", got)
	}
}

// TestRunnerSink_OnUsage_FlushesBuffer verifies that OnUsage emits the buffer
// even without other events.
func TestRunnerSink_OnUsage_FlushesBuffer(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)
	t.Cleanup(rs.close)

	rs.OnTextDelta("buffered")
	rs.OnUsage(provider.Usage{InputTokens: 10})

	calls := sink.Calls()
	if len(calls) != 1 {
		t.Fatalf("Calls = %d, want 1; got %+v", len(calls), calls)
	}
	if calls[0].content != "buffered" || calls[0].category != "text" {
		t.Errorf("call[0] = {%q, %q}, want {buffered, text}", calls[0].content, calls[0].category)
	}
}

// TestRunnerSink_Close_FlushesBuffer verifies that close() flushes any pending
// text synchronously before marking the sink closed.
func TestRunnerSink_Close_FlushesBuffer(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)

	rs.OnTextDelta("pending")
	rs.close()

	calls := sink.Calls()
	if len(calls) != 1 {
		t.Fatalf("Calls after close = %d, want 1 (sync flush); got %+v", len(calls), calls)
	}
	if calls[0].content != "pending" || calls[0].category != "text" {
		t.Errorf("call[0] = {%q, %q}, want {pending, text}", calls[0].content, calls[0].category)
	}
}

// TestRunnerSink_ToolUseStop_UnknownIDFallback verifies that calling
// OnToolUseStop with an id that was never registered via OnToolUseStart does
// not panic and produces a reasonable row (the id itself used as the name).
func TestRunnerSink_ToolUseStop_UnknownIDFallback(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink, false, nil)
	t.Cleanup(rs.close)

	rs.OnToolUseStop("orphan-id", json.RawMessage(`{"x":1}`))

	calls := sink.Calls()
	if len(calls) != 1 {
		t.Fatalf("Calls = %d, want 1 (fallback row using id as name); got %+v", len(calls), calls)
	}
	if !strings.Contains(calls[0].content, "orphan-id") {
		t.Errorf("call[0].content = %q, want it to contain the id as fallback name", calls[0].content)
	}
	if calls[0].category != "tool" {
		t.Errorf("call[0].category = %q, want tool", calls[0].category)
	}
}
