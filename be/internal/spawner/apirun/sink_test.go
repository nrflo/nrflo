package apirun

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider"
)

// TestRunnerSink_TextDeltas_BelowThreshold_FlushOnUsage verifies that small
// fragmented deltas (each <80 chars, total <80 chars) stay buffered and
// produce one TrackMessage call when OnUsage flushes.
func TestRunnerSink_TextDeltas_BelowThreshold_FlushOnUsage(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink)
	t.Cleanup(rs.close)

	// 5 small fragments, total < 80 chars
	rs.OnTextDelta("a")
	rs.OnTextDelta("b")
	rs.OnTextDelta("c")
	rs.OnTextDelta("d")
	rs.OnTextDelta("e")

	// Before flush, sink should not have received any TrackMessage
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

// TestRunnerSink_TextDeltas_ThresholdFlush verifies that crossing the 80-char
// buffer threshold flushes immediately without waiting for OnUsage.
func TestRunnerSink_TextDeltas_ThresholdFlush(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink)
	t.Cleanup(rs.close)

	long := strings.Repeat("x", 100) // > 80 chars, single delta crosses threshold
	rs.OnTextDelta(long)

	calls := sink.Calls()
	if len(calls) != 1 {
		t.Fatalf("Calls = %d, want 1 (immediate threshold flush); got %+v", len(calls), calls)
	}
	if calls[0].content != long {
		t.Errorf("call[0].content = %q, want %d-char x string", calls[0].content, len(long))
	}
	if calls[0].category != "text" {
		t.Errorf("call[0].category = %q, want text", calls[0].category)
	}
}

// TestRunnerSink_TextDeltas_FragmentsCrossThreshold verifies that a sequence
// of small fragments exceeds the 80-char threshold and flushes once.
func TestRunnerSink_TextDeltas_FragmentsCrossThreshold(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink)
	t.Cleanup(rs.close)

	// 5 fragments of 20 chars each = 100 chars total, threshold (80) crossed
	// at the 4th fragment.
	frag := strings.Repeat("y", 20)
	for i := 0; i < 5; i++ {
		rs.OnTextDelta(frag)
	}

	// At least one flush should have occurred. Per spec: 1-2 calls for ~200
	// chars of streamed text.
	rs.OnUsage(provider.Usage{}) // ensure trailing buffer is flushed

	calls := sink.Calls()
	if len(calls) < 1 || len(calls) > 2 {
		t.Errorf("Calls = %d, want 1 or 2; got %+v", len(calls), calls)
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
	rs := newRunnerSink(sink)
	t.Cleanup(rs.close)

	rs.OnTextDelta("")
	rs.OnUsage(provider.Usage{})

	if got := len(sink.Calls()); got != 0 {
		t.Errorf("Calls = %d, want 0 (empty delta should not buffer)", got)
	}
}

// TestRunnerSink_ToolUseStart_FlushesBufferThenEmits verifies that a pending
// text buffer is flushed before the tool_use_start message and the message
// includes id and name.
func TestRunnerSink_ToolUseStart_FlushesBufferThenEmits(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink)
	t.Cleanup(rs.close)

	rs.OnTextDelta("preamble")
	rs.OnToolUseStart("tool-1", "Bash")

	calls := sink.Calls()
	if len(calls) != 2 {
		t.Fatalf("Calls = %d, want 2 (text flush + tool_use_start); got %+v", len(calls), calls)
	}
	if calls[0].content != "preamble" || calls[0].category != "text" {
		t.Errorf("call[0] = {%q, %q}, want {preamble, text}", calls[0].content, calls[0].category)
	}
	if calls[1].category != "tool_use_start" {
		t.Errorf("call[1].category = %q, want tool_use_start", calls[1].category)
	}
	if !strings.Contains(calls[1].content, "tool-1") {
		t.Errorf("call[1].content = %q, want it to contain tool-1", calls[1].content)
	}
	if !strings.Contains(calls[1].content, "Bash") {
		t.Errorf("call[1].content = %q, want it to contain Bash", calls[1].content)
	}
}

// TestRunnerSink_ToolUseStop_FlushesBufferThenEmitsInput verifies that
// OnToolUseStop flushes pending text and emits the tool_use_input message
// with id + JSON input.
func TestRunnerSink_ToolUseStop_FlushesBufferThenEmitsInput(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink)
	t.Cleanup(rs.close)

	rs.OnTextDelta("more text")
	rs.OnToolUseStop("tool-1", json.RawMessage(`{"command":"ls"}`))

	calls := sink.Calls()
	if len(calls) != 2 {
		t.Fatalf("Calls = %d, want 2; got %+v", len(calls), calls)
	}
	if calls[0].content != "more text" || calls[0].category != "text" {
		t.Errorf("call[0] = {%q, %q}, want {more text, text}", calls[0].content, calls[0].category)
	}
	if calls[1].category != "tool_use_input" {
		t.Errorf("call[1].category = %q, want tool_use_input", calls[1].category)
	}
	if !strings.Contains(calls[1].content, `"command":"ls"`) {
		t.Errorf("call[1].content = %q, want JSON input", calls[1].content)
	}
	if !strings.Contains(calls[1].content, "tool-1") {
		t.Errorf("call[1].content = %q, want tool-1 id", calls[1].content)
	}
}

// TestRunnerSink_ToolUseInputDelta_Discarded verifies partial JSON deltas are
// discarded (no TrackMessage emitted).
func TestRunnerSink_ToolUseInputDelta_Discarded(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink)
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
	rs := newRunnerSink(sink)
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

// TestRunnerSink_Close_StopsLateFlush verifies that close() suppresses pending
// idle-timer flushes so the runner sink is safe to discard after a turn.
func TestRunnerSink_Close_StopsLateFlush(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink)

	rs.OnTextDelta("pending")
	rs.close()

	// Wait past the 200ms idle threshold and confirm no late flush.
	deadline := time.Now().Add(350 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(sink.Calls()) > 0 {
			t.Fatalf("late flush after close: %+v", sink.Calls())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRunnerSink_IdleFlush_AfterTimeout verifies that with no further events,
// the buffered text is flushed by the idle timer (200ms threshold).
func TestRunnerSink_IdleFlush_AfterTimeout(t *testing.T) {
	sink := &recordingSink{}
	rs := newRunnerSink(sink)
	t.Cleanup(rs.close)

	rs.OnTextDelta("idle me")

	// Poll up to 1s for the idle flush. Threshold is 200ms.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if len(sink.Calls()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	calls := sink.Calls()
	if len(calls) != 1 {
		t.Fatalf("idle flush did not produce 1 call; got %+v", calls)
	}
	if calls[0].content != "idle me" || calls[0].category != "text" {
		t.Errorf("call[0] = {%q, %q}, want {idle me, text}", calls[0].content, calls[0].category)
	}
}
