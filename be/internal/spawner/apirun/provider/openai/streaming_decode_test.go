package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// fakeRoundTripper returns a canned text/event-stream response.
type fakeRoundTripper struct {
	body string
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Request:    req,
	}, nil
}

// recordingSink captures callbacks for assertion.
type recordingSink struct {
	events []string
	final  provider.Usage
}

func (s *recordingSink) OnTextDelta(text string) {
	s.events = append(s.events, "text:"+text)
}

func (s *recordingSink) OnThinkingDelta(text string) {}
func (s *recordingSink) OnToolUseStart(id, name string) {
	s.events = append(s.events, "tool_start:"+id+":"+name)
}
func (s *recordingSink) OnToolUseInputDelta(id, partial string) {
	s.events = append(s.events, "tool_delta:"+id+":"+partial)
}
func (s *recordingSink) OnToolUseStop(id string, full json.RawMessage) {
	s.events = append(s.events, "tool_stop:"+id+":"+string(full))
}
func (s *recordingSink) OnUsage(u provider.Usage) {
	s.events = append(s.events, "usage")
	s.final = u
}

// sseEvent formats one SSE frame.
func sseEvent(name, data string) string {
	return "event: " + name + "\ndata: " + data + "\n\n"
}

// completedJSON returns a minimal response.completed event data payload.
func completedJSON(status, incompleteReason string, inTok, outTok int) string {
	var incDetails string
	if incompleteReason != "" {
		incDetails = `"incomplete_details":{"reason":"` + incompleteReason + `"},`
	} else {
		incDetails = `"incomplete_details":{},`
	}
	return `{"type":"response.completed","response":{"id":"resp_1","created_at":1234567890,"status":"` + status + `","model":"gpt-4o",` +
		`"usage":{"input_tokens":` + itoa(inTok) + `,"output_tokens":` + itoa(outTok) +
		`,"total_tokens":` + itoa(inTok+outTok) +
		`,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}},` +
		incDetails +
		`"output":[],"error":{},"instructions":null,"metadata":{},"parallel_tool_calls":false,"temperature":1,"tool_choice":{},"object":"response"}}`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	// Simple non-negative int to string for test helpers.
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func newTestProvider(body string) provider.Provider {
	return NewWithHTTPClient(Credentials{Value: "test-key"}, &http.Client{
		Transport: &fakeRoundTripper{body: body},
	})
}

func minimalRequest() provider.Request {
	return provider.Request{
		Model:     "gpt-4o",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "hi"}},
		}},
	}
}

// TestRun_TextOnly verifies a text-only stream produces end_turn stop reason
// and the assembled text content block.
func TestRun_TextOnly(t *testing.T) {
	var b strings.Builder
	b.WriteString(sseEvent("response.output_item.added",
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","status":"in_progress"}}`))
	// Real wire shape (OpenAI + OpenRouter): the chunk is in "delta"; "text"
	// only appears on the ".done" variant.
	b.WriteString(sseEvent("response.output_text.delta",
		`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Hel"}`))
	b.WriteString(sseEvent("response.output_text.delta",
		`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"lo"}`))
	b.WriteString(sseEvent("response.output_item.done",
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","status":"completed"}}`))
	b.WriteString(sseEvent("response.completed", completedJSON("completed", "", 10, 3)))

	sink := &recordingSink{}
	resp, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", resp.StopReason)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "text" || resp.Content[0].Text != "Hello" {
		t.Errorf("Content = %+v, want one text block 'Hello'", resp.Content)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 3 {
		t.Errorf("Usage = %+v, want in=10 out=3", resp.Usage)
	}
	wantEvents := []string{"text:Hel", "text:lo", "usage"}
	if len(sink.events) != len(wantEvents) {
		t.Fatalf("events = %v, want %v", sink.events, wantEvents)
	}
	for i, w := range wantEvents {
		if sink.events[i] != w {
			t.Errorf("events[%d] = %q, want %q", i, sink.events[i], w)
		}
	}
}

// TestRun_EmptyToolArgs verifies that a function_call with no argument deltas
// produces a tool_use ContentBlock with valid "{}" input.
func TestRun_EmptyToolArgs(t *testing.T) {
	var b strings.Builder
	b.WriteString(sseEvent("response.output_item.added",
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"NoArg","status":"in_progress"}}`))
	b.WriteString(sseEvent("response.output_item.done",
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"NoArg","arguments":"{}","status":"completed"}}`))
	b.WriteString(sseEvent("response.completed", completedJSON("completed", "", 5, 2)))

	sink := &recordingSink{}
	resp, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("StopReason = %q, want tool_use", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(resp.Content))
	}
	blk := resp.Content[0]
	if blk.Type != "tool_use" || blk.ToolName != "NoArg" || blk.ToolUseID != "call_abc" {
		t.Errorf("Content[0] = %+v, want tool_use NoArg call_abc", blk)
	}
	if string(blk.Input) != "{}" {
		t.Errorf("Input = %q, want {}", blk.Input)
	}
	// Callbacks: tool_start → tool_stop → usage.
	if len(sink.events) < 3 {
		t.Fatalf("events = %v, want at least tool_start + tool_stop + usage", sink.events)
	}
	if sink.events[0] != "tool_start:call_abc:NoArg" {
		t.Errorf("events[0] = %q, want tool_start:call_abc:NoArg", sink.events[0])
	}
	if sink.events[len(sink.events)-1] != "usage" {
		t.Errorf("last event = %q, want usage", sink.events[len(sink.events)-1])
	}
}

// TestRun_MaxTokens verifies that an incomplete response with
// max_output_tokens reason yields StopReason "max_tokens".
func TestRun_MaxTokens(t *testing.T) {
	var b strings.Builder
	b.WriteString(sseEvent("response.output_item.added",
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","status":"in_progress"}}`))
	b.WriteString(sseEvent("response.output_text.delta",
		`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"partial"}`))
	b.WriteString(sseEvent("response.output_item.done",
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","status":"completed"}}`))
	b.WriteString(sseEvent("response.completed",
		completedJSON("incomplete", "max_output_tokens", 5, 10)))

	sink := &recordingSink{}
	resp, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.StopReason != "max_tokens" {
		t.Errorf("StopReason = %q, want max_tokens", resp.StopReason)
	}
}

// TestRun_FunctionCallWithDeltas verifies that incremental
// response.function_call_arguments.delta chunks (carried in the event's "delta"
// field) accumulate into the assembled tool_use ContentBlock.Input.
func TestRun_FunctionCallWithDeltas(t *testing.T) {
	var b strings.Builder
	b.WriteString(sseEvent("response.output_item.added",
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_xyz","name":"Read","status":"in_progress"}}`))
	b.WriteString(sseEvent("response.function_call_arguments.delta",
		`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"path\":\""}`))
	b.WriteString(sseEvent("response.function_call_arguments.delta",
		`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"/tmp/x\"}"}`))
	b.WriteString(sseEvent("response.function_call_arguments.done",
		`{"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_1","arguments":"{\"path\":\"/tmp/x\"}"}`))
	b.WriteString(sseEvent("response.output_item.done",
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_xyz","name":"Read","arguments":"{\"path\":\"/tmp/x\"}","status":"completed"}}`))
	b.WriteString(sseEvent("response.completed", completedJSON("completed", "", 8, 4)))

	sink := &recordingSink{}
	resp, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("StopReason = %q, want tool_use", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(resp.Content))
	}
	blk := resp.Content[0]
	if blk.Type != "tool_use" || blk.ToolName != "Read" || blk.ToolUseID != "call_xyz" {
		t.Errorf("Content[0] = %+v, want tool_use Read call_xyz", blk)
	}
	if string(blk.Input) != `{"path":"/tmp/x"}` {
		t.Errorf("Input = %q, want %q", blk.Input, `{"path":"/tmp/x"}`)
	}
	// Deltas must surface through OnToolUseInputDelta and assemble the full input.
	wantEvents := []string{
		"tool_start:call_xyz:Read",
		`tool_delta:call_xyz:{"path":"`,
		`tool_delta:call_xyz:/tmp/x"}`,
		`tool_stop:call_xyz:{"path":"/tmp/x"}`,
		"usage",
	}
	if len(sink.events) != len(wantEvents) {
		t.Fatalf("events = %v, want %v", sink.events, wantEvents)
	}
	for i, w := range wantEvents {
		if sink.events[i] != w {
			t.Errorf("events[%d] = %q, want %q", i, sink.events[i], w)
		}
	}
}

// TestRun_MalformedToolArgs_Error verifies that argument deltas which assemble
// into invalid JSON cause Run to return an error rather than a bad ContentBlock.
func TestRun_MalformedToolArgs_Error(t *testing.T) {
	var b strings.Builder
	b.WriteString(sseEvent("response.output_item.added",
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_bad","name":"Read","status":"in_progress"}}`))
	b.WriteString(sseEvent("response.function_call_arguments.delta",
		`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"path\":"}`))
	b.WriteString(sseEvent("response.function_call_arguments.done",
		`{"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_1","arguments":"{\"path\":"}`))
	b.WriteString(sseEvent("response.completed", completedJSON("completed", "", 8, 4)))

	sink := &recordingSink{}
	_, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), sink)
	if err == nil {
		t.Fatalf("expected error for malformed tool arguments")
	}
	if !strings.Contains(err.Error(), "invalid JSON input") {
		t.Errorf("err = %v, want mention of invalid JSON input", err)
	}
}
