package openai

import (
	"context"
	"strings"
	"testing"
)

// TestRun_StreamEndsWithoutCompleted_DerivesStopReason is the regression test
// for "unexpected stop_reason=\"\"": some OpenRouter upstreams close the SSE
// stream after the last output_item.done without ever emitting
// response.completed. The decoder must derive a stop reason from the assembled
// content instead of returning an empty one (which fails the agent session in
// the runner loop).
func TestRun_StreamEndsWithoutCompleted_DerivesStopReason(t *testing.T) {
	t.Run("text_only_yields_end_turn", func(t *testing.T) {
		var b strings.Builder
		b.WriteString(sseEvent("response.output_item.added",
			`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","status":"in_progress"}}`))
		b.WriteString(sseEvent("response.output_text.delta",
			`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"partial answer"}`))
		b.WriteString(sseEvent("response.output_item.done",
			`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","status":"completed"}}`))
		// No response.completed — stream just ends.

		sink := &recordingSink{}
		resp, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), sink)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if resp.StopReason != "end_turn" {
			t.Errorf("StopReason = %q, want end_turn (derived, never empty)", resp.StopReason)
		}
		if len(resp.Content) != 1 || resp.Content[0].Type != "text" || resp.Content[0].Text != "partial answer" {
			t.Errorf("Content = %+v, want one text block 'partial answer'", resp.Content)
		}
	})

	t.Run("tool_call_yields_tool_use", func(t *testing.T) {
		var b strings.Builder
		b.WriteString(sseEvent("response.output_item.added",
			`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"Read","status":"in_progress"}}`))
		b.WriteString(sseEvent("response.function_call_arguments.delta",
			`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"path\":\"/tmp/x\"}"}`))
		b.WriteString(sseEvent("response.function_call_arguments.done",
			`{"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_1","arguments":"{\"path\":\"/tmp/x\"}"}`))
		b.WriteString(sseEvent("response.output_item.done",
			`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"Read","status":"completed"}}`))
		// No response.completed — stream just ends.

		sink := &recordingSink{}
		resp, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), sink)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if resp.StopReason != "tool_use" {
			t.Errorf("StopReason = %q, want tool_use (derived from function_call content)", resp.StopReason)
		}
		if len(resp.Content) != 1 || resp.Content[0].Type != "tool_use" || resp.Content[0].ToolName != "Read" {
			t.Errorf("Content = %+v, want one tool_use Read block", resp.Content)
		}
	})
}

// TestRunner_EmptyStopReason_Normalized is mirrored at the apirun package
// level; here we verify the anthropic-style guarantee for openai too: a
// FinalResponse must never carry an empty StopReason after decodeStream.
func TestRun_EmptyStream_NeverEmptyStopReason(t *testing.T) {
	// A stream that ends with no output items and no terminal event at all.
	resp, err := newTestProvider("").Run(context.Background(), minimalRequest(), &recordingSink{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.StopReason == "" {
		t.Errorf("StopReason = empty, want derived end_turn")
	}
}
