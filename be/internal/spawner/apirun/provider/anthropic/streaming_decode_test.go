package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// fakeRoundTripper returns a canned text/event-stream HTTP response. The body
// is the joined SSE event payload provided at construction time.
type fakeRoundTripper struct {
	body    string
	status  int
	lastReq *http.Request
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.lastReq = req
	status := f.status
	if status == 0 {
		status = http.StatusOK
	}
	header := http.Header{}
	header.Set("Content-Type", "text/event-stream")
	return &http.Response{
		Status:     "200 OK",
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Request:    req,
	}, nil
}

// recordingSink captures all callbacks so tests can verify ordering and
// payloads.
type recordingSink struct {
	events []string
	final  provider.Usage
}

func (s *recordingSink) OnTextDelta(text string) {
	s.events = append(s.events, "text:"+text)
}

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

// sseEvent formats one SSE frame: an event line, a data line with the JSON
// payload, and the terminating blank line.
func sseEvent(name string, data string) string {
	return "event: " + name + "\ndata: " + data + "\n\n"
}

// happyPathSSE returns a canned stream covering text + tool_use deltas + final
// usage and stop_reason.
func happyPathSSE() string {
	var b bytes.Buffer
	b.WriteString(sseEvent("message_start",
		`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-4-7","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`))
	b.WriteString(sseEvent("content_block_start",
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`))
	b.WriteString(sseEvent("content_block_stop",
		`{"type":"content_block_stop","index":0}`))
	b.WriteString(sseEvent("content_block_start",
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_1","name":"Read","input":{}}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"file_path\":"}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":" \"/x\"}"}}`))
	b.WriteString(sseEvent("content_block_stop",
		`{"type":"content_block_stop","index":1}`))
	b.WriteString(sseEvent("message_delta",
		`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"input_tokens":10,"output_tokens":15}}`))
	b.WriteString(sseEvent("message_stop", `{"type":"message_stop"}`))
	return b.String()
}

func TestRun_StreamingHappyPath(t *testing.T) {
	rt := &fakeRoundTripper{body: happyPathSSE()}
	p := NewWithHTTPClient("test-key", &http.Client{Transport: rt})

	sink := &recordingSink{}
	resp, err := p.Run(context.Background(), provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 100,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "hi"}},
		}},
	}, sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	wantEvents := []string{
		"text:Hel",
		"text:lo",
		"tool_start:tool_1:Read",
		`tool_delta:tool_1:{"file_path":`,
		`tool_delta:tool_1: "/x"}`,
		`tool_stop:tool_1:{"file_path": "/x"}`,
		"usage",
	}
	if len(sink.events) != len(wantEvents) {
		t.Fatalf("event count = %d, want %d (events=%v)", len(sink.events), len(wantEvents), sink.events)
	}
	for i, w := range wantEvents {
		if sink.events[i] != w {
			t.Errorf("event[%d] = %q, want %q", i, sink.events[i], w)
		}
	}

	if resp.StopReason != "tool_use" {
		t.Errorf("StopReason = %q, want tool_use", resp.StopReason)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("Content len = %d, want 2", len(resp.Content))
	}
	if resp.Content[0].Type != "text" || resp.Content[0].Text != "Hello" {
		t.Errorf("Content[0] = %+v, want text 'Hello'", resp.Content[0])
	}
	if resp.Content[1].Type != "tool_use" || resp.Content[1].ToolName != "Read" || resp.Content[1].ToolUseID != "tool_1" {
		t.Errorf("Content[1] = %+v, want tool_use Read tool_1", resp.Content[1])
	}
	var parsed map[string]string
	if err := json.Unmarshal(resp.Content[1].Input, &parsed); err != nil {
		t.Fatalf("Content[1].Input not valid JSON: %v (%s)", err, resp.Content[1].Input)
	}
	if parsed["file_path"] != "/x" {
		t.Errorf("Content[1].Input parsed = %v, want file_path=/x", parsed)
	}
	if resp.Usage.OutputTokens != 15 {
		t.Errorf("Usage.OutputTokens = %d, want 15", resp.Usage.OutputTokens)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens = %d, want 10", resp.Usage.InputTokens)
	}

	if rt.lastReq == nil {
		t.Fatalf("fake transport never received a request")
	}
	if got := rt.lastReq.Header.Get("anthropic-beta"); !strings.Contains(got, "prompt-caching") {
		t.Errorf("anthropic-beta header = %q, want substring 'prompt-caching'", got)
	}
}

func TestRun_StreamingMalformedToolInputErrors(t *testing.T) {
	var b bytes.Buffer
	b.WriteString(sseEvent("message_start",
		`{"type":"message_start","message":{"id":"msg","type":"message","role":"assistant","model":"m","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`))
	b.WriteString(sseEvent("content_block_start",
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"X","input":{}}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{not"}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":" json}"}}`))
	b.WriteString(sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`))
	b.WriteString(sseEvent("message_stop", `{"type":"message_stop"}`))

	rt := &fakeRoundTripper{body: b.String()}
	p := NewWithHTTPClient("test-key", &http.Client{Transport: rt})

	_, err := p.Run(context.Background(), provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "hi"}},
		}},
	}, &recordingSink{})
	if err == nil {
		t.Fatalf("expected error for malformed tool_use input JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("err = %v, want it to mention invalid JSON", err)
	}
}

func TestRun_StreamingTextOnly(t *testing.T) {
	var b bytes.Buffer
	b.WriteString(sseEvent("message_start",
		`{"type":"message_start","message":{"id":"msg","type":"message","role":"assistant","model":"m","content":[],"usage":{"input_tokens":3,"output_tokens":0}}}`))
	b.WriteString(sseEvent("content_block_start",
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`))
	b.WriteString(sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`))
	b.WriteString(sseEvent("message_delta",
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":3,"output_tokens":1}}`))
	b.WriteString(sseEvent("message_stop", `{"type":"message_stop"}`))

	rt := &fakeRoundTripper{body: b.String()}
	p := NewWithHTTPClient("test-key", &http.Client{Transport: rt})
	sink := &recordingSink{}
	resp, err := p.Run(context.Background(), provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "hi"}},
		}},
	}, sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", resp.StopReason)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "ok" {
		t.Errorf("Content = %+v, want one text 'ok'", resp.Content)
	}
}

func TestRun_StreamingEmptyToolInput(t *testing.T) {
	// tool_use block with NO input_json_delta events — content_block_stop must
	// still produce a valid empty-object input.
	var b bytes.Buffer
	b.WriteString(sseEvent("message_start",
		`{"type":"message_start","message":{"id":"msg","type":"message","role":"assistant","model":"m","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`))
	b.WriteString(sseEvent("content_block_start",
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"NoArg","input":{}}}`))
	b.WriteString(sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`))
	b.WriteString(sseEvent("message_delta",
		`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"input_tokens":1,"output_tokens":1}}`))
	b.WriteString(sseEvent("message_stop", `{"type":"message_stop"}`))

	rt := &fakeRoundTripper{body: b.String()}
	p := NewWithHTTPClient("test-key", &http.Client{Transport: rt})
	sink := &recordingSink{}
	resp, err := p.Run(context.Background(), provider.Request{Model: "m", MaxTokens: 1}, sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(resp.Content))
	}
	if string(resp.Content[0].Input) != "{}" {
		t.Errorf("empty tool input = %q, want '{}'", resp.Content[0].Input)
	}
	// OnToolUseStart fires, then OnToolUseStop with `{}`, then OnUsage.
	wantEvents := []string{
		"tool_start:t1:NoArg",
		"tool_stop:t1:{}",
		"usage",
	}
	if len(sink.events) != len(wantEvents) {
		t.Fatalf("events = %v, want %v", sink.events, wantEvents)
	}
	for i, w := range wantEvents {
		if sink.events[i] != w {
			t.Errorf("event[%d] = %q, want %q", i, sink.events[i], w)
		}
	}
}
