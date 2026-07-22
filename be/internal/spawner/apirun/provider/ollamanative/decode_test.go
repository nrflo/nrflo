package ollamanative

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// recordingSink captures callbacks for assertion, mirroring
// openaichat/decode_test.go's recordingSink.
type recordingSink struct {
	events []string
	final  provider.Usage
}

func (s *recordingSink) OnTextDelta(text string) { s.events = append(s.events, "text:"+text) }
func (s *recordingSink) OnThinkingDelta(string)  {}
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

func minimalRequest() provider.Request {
	return provider.Request{
		Model:     "llama3",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "hi"}},
		}},
	}
}

// newStubServerProvider spins a stub /api/chat httptest.Server that streams
// body verbatim as the NDJSON response, and wires an ollamanative provider
// against it. No live Ollama — CI-safe.
func newStubServerProvider(t *testing.T, body string) provider.Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %q, want /api/chat", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(Credentials{BaseURL: srv.URL})
}

// TestDecodeStream_TextOnly verifies plain content deltas assemble into one
// text block, stop reason defaults from done_reason, and usage comes from
// the done:true chunk.
func TestDecodeStream_TextOnly(t *testing.T) {
	body := `{"message":{"content":"Hel"},"done":false}` + "\n" +
		`{"message":{"content":"lo"},"done":false}` + "\n" +
		`{"message":{"content":""},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":3}` + "\n"

	sink := &recordingSink{}
	resp, err := newStubServerProvider(t, body).Run(context.Background(), minimalRequest(), sink)
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

// TestDecodeStream_ToolCallAssembly verifies a tool call arriving complete
// (no id, no index — Ollama's native shape) in one NDJSON line, amid other
// text-bearing lines, synthesizes a sequential id and emits the identical
// start/delta/stop event ordering as openaichat's incremental assembly.
func TestDecodeStream_ToolCallAssembly(t *testing.T) {
	body := `{"message":{"content":"Sure, "},"done":false}` + "\n" +
		`{"message":{"content":"","tool_calls":[{"function":{"name":"ls","arguments":{"path":"."}}}]},"done":false}` + "\n" +
		`{"message":{},"done":true,"done_reason":"stop","prompt_eval_count":8,"eval_count":4}` + "\n"

	sink := &recordingSink{}
	resp, err := newStubServerProvider(t, body).Run(context.Background(), minimalRequest(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("StopReason = %q, want tool_use", resp.StopReason)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("Content len = %d, want 2 (text + tool_use)", len(resp.Content))
	}
	if resp.Content[0].Type != "text" || resp.Content[0].Text != "Sure, " {
		t.Errorf("Content[0] = %+v, want text 'Sure, '", resp.Content[0])
	}
	blk := resp.Content[1]
	if blk.Type != "tool_use" || blk.ToolName != "ls" || blk.ToolUseID != "call_0" {
		t.Errorf("Content[1] = %+v, want tool_use ls call_0", blk)
	}
	if string(blk.Input) != `{"path":"."}` {
		t.Errorf("Input = %q, want %q", blk.Input, `{"path":"."}`)
	}
	wantEvents := []string{
		"text:Sure, ",
		"tool_start:call_0:ls",
		`tool_delta:call_0:{"path":"."}`,
		`tool_stop:call_0:{"path":"."}`,
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

// TestDecodeStream_MultipleToolCalls_SequentialIDs verifies two tool calls in
// the same chunk get sequential synthesized ids (call_0, call_1).
func TestDecodeStream_MultipleToolCalls_SequentialIDs(t *testing.T) {
	body := `{"message":{"tool_calls":[` +
		`{"function":{"name":"ls","arguments":{"path":"."}}},` +
		`{"function":{"name":"cat","arguments":{"path":"a.txt"}}}` +
		`]},"done":false}` + "\n" +
		`{"message":{},"done":true,"done_reason":"stop"}` + "\n"

	resp, err := newStubServerProvider(t, body).Run(context.Background(), minimalRequest(), &recordingSink{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("Content len = %d, want 2", len(resp.Content))
	}
	if resp.Content[0].ToolUseID != "call_0" || resp.Content[0].ToolName != "ls" {
		t.Errorf("Content[0] = %+v, want call_0/ls", resp.Content[0])
	}
	if resp.Content[1].ToolUseID != "call_1" || resp.Content[1].ToolName != "cat" {
		t.Errorf("Content[1] = %+v, want call_1/cat", resp.Content[1])
	}
}

// TestDecodeStream_EmptyArgToolCall verifies a tool call with an omitted
// arguments field still produces a valid "{}" input.
func TestDecodeStream_EmptyArgToolCall(t *testing.T) {
	body := `{"message":{"tool_calls":[{"function":{"name":"NoArg"}}]},"done":false}` + "\n" +
		`{"message":{},"done":true,"done_reason":"stop"}` + "\n"

	resp, err := newStubServerProvider(t, body).Run(context.Background(), minimalRequest(), &recordingSink{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(resp.Content) != 1 || string(resp.Content[0].Input) != "{}" {
		t.Errorf("Content = %+v, want tool_use with {} input", resp.Content)
	}
}

// TestDecodeStream_MalformedChunk_Error verifies an NDJSON line that isn't
// valid JSON surfaces a descriptive decode error rather than being skipped.
func TestDecodeStream_MalformedChunk_Error(t *testing.T) {
	body := `not json at all` + "\n"

	_, err := newStubServerProvider(t, body).Run(context.Background(), minimalRequest(), &recordingSink{})
	if err == nil {
		t.Fatal("expected error for malformed NDJSON chunk")
	}
	if !strings.Contains(err.Error(), "decode ollamanative chunk") {
		t.Errorf("err = %v, want mention of decode ollamanative chunk", err)
	}
}

// TestDecodeStream_DoneReasonMapping table-drives Ollama's done_reason ->
// provider-neutral StopReason mapping (no tool calls in play).
func TestDecodeStream_DoneReasonMapping(t *testing.T) {
	tests := []struct {
		doneReason string
		want       string
	}{
		{"length", "max_tokens"},
		{"stop", "end_turn"},
		{"", "end_turn"},
	}
	for _, tc := range tests {
		t.Run(tc.doneReason, func(t *testing.T) {
			body := `{"message":{"content":"hi"},"done":true,"done_reason":"` + tc.doneReason + `"}` + "\n"
			resp, err := newStubServerProvider(t, body).Run(context.Background(), minimalRequest(), &recordingSink{})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if resp.StopReason != tc.want {
				t.Errorf("done_reason=%q -> StopReason = %q, want %q", tc.doneReason, resp.StopReason, tc.want)
			}
		})
	}
}

// TestDecodeStream_ToolCallsOverrideDoneReason verifies presence of tool
// calls forces StopReason=tool_use even when done_reason says "length".
func TestDecodeStream_ToolCallsOverrideDoneReason(t *testing.T) {
	body := `{"message":{"tool_calls":[{"function":{"name":"ls","arguments":{}}}]},"done":false}` + "\n" +
		`{"message":{},"done":true,"done_reason":"length"}` + "\n"

	resp, err := newStubServerProvider(t, body).Run(context.Background(), minimalRequest(), &recordingSink{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("StopReason = %q, want tool_use (overrides done_reason=length)", resp.StopReason)
	}
}
