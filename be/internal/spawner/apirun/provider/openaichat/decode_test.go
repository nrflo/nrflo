package openaichat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// fakeRoundTripper returns a canned text/event-stream response, mirroring
// the openai package's contract_test.go fixture style. Chat Completions
// streaming frames are plain "data: {...}\n\n" lines terminated by
// "data: [DONE]\n\n" (no "event:" line, unlike the Responses API).
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

// recordingSink captures callbacks for assertion, mirroring openai's
// contract_test.go recordingSink.
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

func sseData(data string) string { return "data: " + data + "\n\n" }

func sseDone() string { return "data: [DONE]\n\n" }

func newTestProvider(body string) provider.Provider {
	return NewWithHTTPClient(Credentials{Value: "test-key"}, &http.Client{
		Transport: &fakeRoundTripper{body: body},
	})
}

func minimalRequest() provider.Request {
	return provider.Request{
		Model:     "local-model",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "hi"}},
		}},
	}
}

// chunkJSON builds one ChatCompletionChunk data payload.
func chunkJSON(delta, finishReason, usageJSON string) string {
	fr := "null"
	if finishReason != "" {
		fr = `"` + finishReason + `"`
	}
	usage := ""
	if usageJSON != "" {
		usage = `,"usage":` + usageJSON
	}
	return `{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"local-model",` +
		`"choices":[{"index":0,"delta":` + delta + `,"finish_reason":` + fr + `}]` + usage + `}`
}

// TestDecodeStream_TextOnly verifies plain text deltas assemble into one
// text block, stop reason defaults to end_turn, and events fire in order.
func TestDecodeStream_TextOnly(t *testing.T) {
	var b strings.Builder
	b.WriteString(sseData(chunkJSON(`{"content":"Hel"}`, "", "")))
	b.WriteString(sseData(chunkJSON(`{"content":"lo"}`, "stop", "")))
	b.WriteString(sseData(chunkJSON(`{}`, "", `{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}`)))
	b.WriteString(sseDone())

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

// TestDecodeStream_ToolCallAssembly verifies tool-call deltas keyed by array
// index assemble id/name/arguments across multiple chunks.
func TestDecodeStream_ToolCallAssembly(t *testing.T) {
	var b strings.Builder
	b.WriteString(sseData(chunkJSON(
		`{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"Read","arguments":""}}]}`, "", "")))
	b.WriteString(sseData(chunkJSON(
		`{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\""}}]}`, "", "")))
	b.WriteString(sseData(chunkJSON(
		`{"tool_calls":[{"index":0,"function":{"arguments":"/tmp/x\"}"}}]}`, "tool_calls", "")))
	b.WriteString(sseData(chunkJSON(`{}`, "", `{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}`)))
	b.WriteString(sseDone())

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
	if blk.Type != "tool_use" || blk.ToolName != "Read" || blk.ToolUseID != "call_1" {
		t.Errorf("Content[0] = %+v, want tool_use Read call_1", blk)
	}
	if string(blk.Input) != `{"path":"/tmp/x"}` {
		t.Errorf("Input = %q, want %q", blk.Input, `{"path":"/tmp/x"}`)
	}
	wantEvents := []string{
		"tool_start:call_1:Read",
		`tool_delta:call_1:{"path":"`,
		`tool_delta:call_1:/tmp/x"}`,
		`tool_stop:call_1:{"path":"/tmp/x"}`,
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

// TestDecodeStream_EmptyToolArgs verifies a tool call with no argument
// deltas still produces a valid "{}" input.
func TestDecodeStream_EmptyToolArgs(t *testing.T) {
	var b strings.Builder
	b.WriteString(sseData(chunkJSON(
		`{"tool_calls":[{"index":0,"id":"call_noarg","function":{"name":"NoArg","arguments":""}}]}`, "tool_calls", "")))
	b.WriteString(sseData(chunkJSON(`{}`, "", `{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}`)))
	b.WriteString(sseDone())

	sink := &recordingSink{}
	resp, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(resp.Content) != 1 || string(resp.Content[0].Input) != "{}" {
		t.Errorf("Content = %+v, want tool_use with {} input", resp.Content)
	}
}

// TestDecodeStream_FinishReasonMapping table-drives Chat Completions
// finish_reason -> provider-neutral StopReason mapping.
func TestDecodeStream_FinishReasonMapping(t *testing.T) {
	tests := []struct {
		finishReason string
		want         string
	}{
		{"stop", "end_turn"},
		{"tool_calls", "tool_use"},
		{"length", "max_tokens"},
		{"content_filter", "refusal"},
		{"function_call", "end_turn"}, // unmapped/deprecated reason falls back to end_turn
	}
	for _, tc := range tests {
		t.Run(tc.finishReason, func(t *testing.T) {
			var b strings.Builder
			b.WriteString(sseData(chunkJSON(`{"content":"hi"}`, tc.finishReason, "")))
			b.WriteString(sseDone())
			resp, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), &recordingSink{})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if resp.StopReason != tc.want {
				t.Errorf("finish_reason=%q -> StopReason = %q, want %q", tc.finishReason, resp.StopReason, tc.want)
			}
		})
	}
}

// TestDecodeStream_UsageFromIncludeUsageChunk verifies usage is only read
// from the final include_usage chunk (an interim chunk with no usage field
// must not clobber it with a zero value).
func TestDecodeStream_UsageFromIncludeUsageChunk(t *testing.T) {
	var b strings.Builder
	b.WriteString(sseData(chunkJSON(`{"content":"hi"}`, "stop", "")))
	b.WriteString(sseData(chunkJSON(`{}`, "", `{"prompt_tokens":20,"completion_tokens":7,"total_tokens":27}`)))
	b.WriteString(sseDone())

	resp, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), &recordingSink{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Usage.InputTokens != 20 || resp.Usage.OutputTokens != 7 {
		t.Errorf("Usage = %+v, want in=20 out=7", resp.Usage)
	}
}

// TestDecodeStream_MalformedToolArgs_Error verifies argument deltas that
// assemble into invalid JSON cause Run to return an error.
func TestDecodeStream_MalformedToolArgs_Error(t *testing.T) {
	var b strings.Builder
	b.WriteString(sseData(chunkJSON(
		`{"tool_calls":[{"index":0,"id":"call_bad","function":{"name":"Read","arguments":"{\"path\":"}}]}`,
		"tool_calls", "")))
	b.WriteString(sseDone())

	_, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), &recordingSink{})
	if err == nil {
		t.Fatal("expected error for malformed tool arguments")
	}
	if !strings.Contains(err.Error(), "invalid JSON input") {
		t.Errorf("err = %v, want mention of invalid JSON input", err)
	}
}
