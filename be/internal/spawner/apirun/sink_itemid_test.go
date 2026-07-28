package apirun

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// streamDelta is one delta as a StreamHook consumer sees it.
type streamDelta struct {
	itemID string
	text   string
}

// recordingStream records every StreamHook callback, standing in for the
// console engine's live consumer.
type recordingStream struct {
	text       []streamDelta
	thinking   []streamDelta
	toolStarts []toolSpan
	toolEnds   []toolSpan
}

func (s *recordingStream) OnTextDelta(itemID, text string) {
	s.text = append(s.text, streamDelta{itemID: itemID, text: text})
}

func (s *recordingStream) OnThinkingDelta(itemID, text string) {
	s.thinking = append(s.thinking, streamDelta{itemID: itemID, text: text})
}

type toolSpan struct {
	toolUseID, name string
	input           string
	isError         bool
}

func (s *recordingStream) OnToolStart(toolUseID, name string, input json.RawMessage) {
	s.toolStarts = append(s.toolStarts, toolSpan{toolUseID: toolUseID, name: name, input: string(input)})
}

func (s *recordingStream) OnToolEnd(toolUseID string, isError bool) {
	s.toolEnds = append(s.toolEnds, toolSpan{toolUseID: toolUseID, isError: isError})
}

// buffers folds the recorded deltas the way a live consumer does: accumulate
// text per item id, in first-seen order.
func buffers(deltas []streamDelta) []string {
	var order []string
	acc := map[string]string{}
	for _, d := range deltas {
		if _, seen := acc[d.itemID]; !seen {
			order = append(order, d.itemID)
		}
		acc[d.itemID] += d.text
	}
	out := make([]string, 0, len(order))
	for _, id := range order {
		out = append(out, acc[id])
	}
	return out
}

// TestRunnerSink_ItemIDRotatesPerFlush is the dedupe contract with a live
// consumer: every accumulated delta buffer must equal exactly one persisted
// text row. The consumer keys its buffer by item id and only drops it once a
// persisted row holds that buffer's WHOLE text (ui chatStream.ts mergeStream),
// so an id shared across flushes would leave a buffer holding the
// concatenation of several rows — matching none of them, and rendering as a
// permanent duplicated bubble.
func TestRunnerSink_ItemIDRotatesPerFlush(t *testing.T) {
	sink := &recordingSink{}
	stream := &recordingStream{}
	rs := newRunnerSink(sink, false, stream)
	t.Cleanup(rs.close)

	// Segment 1, then a tool call flushes it; segment 2, then end-of-turn flush.
	rs.OnTextDelta("Hel")
	rs.OnTextDelta("lo")
	rs.OnToolUseStart("tool-1", "web_search")
	rs.OnToolUseStop("tool-1", json.RawMessage(`{"q":"x"}`))
	rs.OnTextDelta("World")
	rs.OnUsage(provider.Usage{})

	var rows []string
	for _, c := range sink.Calls() {
		if c.category == "text" {
			rows = append(rows, c.content)
		}
	}
	if len(rows) != 2 || rows[0] != "Hello" || rows[1] != "World" {
		t.Fatalf("persisted text rows = %q, want [Hello World]", rows)
	}

	got := buffers(stream.text)
	if len(got) != 2 {
		t.Fatalf("live buffers = %q, want 2 (one per flushed segment); a single id would give 1 buffer %q", got, got)
	}
	// Each buffer must equal its row exactly — that is the dedupe condition.
	for i, want := range rows {
		if got[i] != want {
			t.Errorf("live buffer[%d] = %q, want %q (must equal the persisted row to dedupe)", i, got[i], want)
		}
	}
}

// TestRunnerSink_ItemIDStableWithinSegment verifies the id does NOT rotate per
// delta: all deltas of one unflushed segment share one id, so the consumer
// accumulates them into a single growing bubble rather than one bubble each.
func TestRunnerSink_ItemIDStableWithinSegment(t *testing.T) {
	stream := &recordingStream{}
	rs := newRunnerSink(&recordingSink{}, false, stream)
	t.Cleanup(rs.close)

	rs.OnTextDelta("a")
	rs.OnTextDelta("b")
	rs.OnTextDelta("c")

	if len(stream.text) != 3 {
		t.Fatalf("stream deltas = %d, want 3", len(stream.text))
	}
	first := stream.text[0].itemID
	if first == "" {
		t.Fatal("itemID is empty; a live consumer keys its delta buffer by id")
	}
	for i, d := range stream.text[1:] {
		if d.itemID != first {
			t.Errorf("delta[%d].itemID = %q, want %q (same unflushed segment)", i+1, d.itemID, first)
		}
	}
}

// TestRunnerSink_ItemIDRotatesOn4KBCap verifies the safety-cap flush rotates
// the id too: the cap splits one model turn into multiple persisted rows, and
// each must be covered by its own buffer.
func TestRunnerSink_ItemIDRotatesOn4KBCap(t *testing.T) {
	sink := &recordingSink{}
	stream := &recordingStream{}
	rs := newRunnerSink(sink, false, stream)
	t.Cleanup(rs.close)

	long := strings.Repeat("x", 5000) // trips the 4096 cap, flushing immediately
	rs.OnTextDelta(long)
	rs.OnTextDelta("tail")
	rs.OnUsage(provider.Usage{})

	got := buffers(stream.text)
	if len(got) != 2 {
		t.Fatalf("live buffers = %d, want 2 (cap flush must rotate the id)", len(got))
	}
	if got[0] != long || got[1] != "tail" {
		t.Errorf("buffers = [%d chars, %q], want [5000 chars, \"tail\"]", len(got[0]), got[1])
	}
}

// TestRunnerSink_ThinkingItemIDRotatesPerFlush verifies thinking segments carry
// their own rotating id, independent of the text buffer's.
func TestRunnerSink_ThinkingItemIDRotatesPerFlush(t *testing.T) {
	stream := &recordingStream{}
	rs := newRunnerSink(&recordingSink{}, true, stream)
	t.Cleanup(rs.close)

	rs.OnThinkingDelta("plan ")
	rs.OnThinkingDelta("one")
	rs.OnUsage(provider.Usage{}) // flush
	rs.OnThinkingDelta("plan two")
	rs.OnUsage(provider.Usage{})

	got := buffers(stream.thinking)
	if len(got) != 2 || got[0] != "plan one" || got[1] != "plan two" {
		t.Fatalf("thinking buffers = %q, want [\"plan one\" \"plan two\"]", got)
	}
	if stream.thinking[0].itemID == "" {
		t.Error("thinking itemID is empty, want a segment id")
	}
}
