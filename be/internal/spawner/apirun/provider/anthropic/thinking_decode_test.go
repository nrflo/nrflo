package anthropic

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// thinkingSSE builds a stream with a single thinking block.
func thinkingSSE(thinkText, signature string) string {
	var b bytes.Buffer
	b.WriteString(sseEvent("message_start",
		`{"type":"message_start","message":{"id":"msg_t","type":"message","role":"assistant","model":"claude-opus-4-7","content":[],"usage":{"input_tokens":5,"output_tokens":0}}}`))
	b.WriteString(sseEvent("content_block_start",
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"`+thinkText+`"}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"`+signature+`"}}`))
	b.WriteString(sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`))
	b.WriteString(sseEvent("message_delta",
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":5,"output_tokens":10}}`))
	b.WriteString(sseEvent("message_stop", `{"type":"message_stop"}`))
	return b.String()
}

// redactedThinkingSSE builds a stream with a single redacted_thinking block.
func redactedThinkingSSE(data string) string {
	var b bytes.Buffer
	b.WriteString(sseEvent("message_start",
		`{"type":"message_start","message":{"id":"msg_r","type":"message","role":"assistant","model":"claude-opus-4-7","content":[],"usage":{"input_tokens":5,"output_tokens":0}}}`))
	b.WriteString(sseEvent("content_block_start",
		`{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"`+data+`"}}`))
	b.WriteString(sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`))
	b.WriteString(sseEvent("message_delta",
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":5,"output_tokens":2}}`))
	b.WriteString(sseEvent("message_stop", `{"type":"message_stop"}`))
	return b.String()
}

func thinkingProvider(body string) provider.Provider {
	rt := &fakeRoundTripper{body: body}
	return NewWithHTTPClient(Credentials{Value: "test-key", Method: MethodAPIKey}, &http.Client{Transport: rt})
}

func thinkingBaseReq() provider.Request {
	return provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 100,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "think"}},
		}},
	}
}

func TestRun_Thinking_DecodesBlockWithSignature(t *testing.T) {
	p := thinkingProvider(thinkingSSE("let me think", "sig-abc"))
	sink := &recordingSink{}
	resp, err := p.Run(context.Background(), thinkingBaseReq(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// OnThinkingDelta called with the thinking text only — never with signature
	foundThink := false
	for _, ev := range sink.events {
		if ev == "think:let me think" {
			foundThink = true
		}
		if ev == "think:sig-abc" {
			t.Errorf("OnThinkingDelta fired with signature content: %q", ev)
		}
	}
	if !foundThink {
		t.Errorf("expected think:let me think event; events=%v", sink.events)
	}

	if len(resp.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(resp.Content))
	}
	cb := resp.Content[0]
	if cb.Type != "thinking" {
		t.Errorf("Content[0].Type = %q, want thinking", cb.Type)
	}
	if cb.Text != "let me think" {
		t.Errorf("Content[0].Text = %q, want 'let me think'", cb.Text)
	}
	if cb.Signature != "sig-abc" {
		t.Errorf("Content[0].Signature = %q, want 'sig-abc'", cb.Signature)
	}
}

func TestRun_RedactedThinking_DecodesBlockWithData(t *testing.T) {
	p := thinkingProvider(redactedThinkingSSE("opaque-payload"))
	sink := &recordingSink{}
	resp, err := p.Run(context.Background(), thinkingBaseReq(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// OnThinkingDelta must NOT be called for redacted_thinking
	for _, ev := range sink.events {
		if len(ev) > 6 && ev[:6] == "think:" {
			t.Errorf("unexpected OnThinkingDelta for redacted_thinking: %q", ev)
		}
	}

	if len(resp.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(resp.Content))
	}
	cb := resp.Content[0]
	if cb.Type != "redacted_thinking" {
		t.Errorf("Content[0].Type = %q, want redacted_thinking", cb.Type)
	}
	if cb.Data != "opaque-payload" {
		t.Errorf("Content[0].Data = %q, want 'opaque-payload'", cb.Data)
	}
}

func TestRun_Thinking_MultiDeltaAccumulation(t *testing.T) {
	var b bytes.Buffer
	b.WriteString(sseEvent("message_start",
		`{"type":"message_start","message":{"id":"msg_m","type":"message","role":"assistant","model":"claude-opus-4-7","content":[],"usage":{"input_tokens":5,"output_tokens":0}}}`))
	b.WriteString(sseEvent("content_block_start",
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"part1"}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" part2"}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig1"}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig2"}}`))
	b.WriteString(sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`))
	b.WriteString(sseEvent("message_delta",
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":5,"output_tokens":5}}`))
	b.WriteString(sseEvent("message_stop", `{"type":"message_stop"}`))

	p := thinkingProvider(b.String())
	sink := &recordingSink{}
	resp, err := p.Run(context.Background(), thinkingBaseReq(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(resp.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(resp.Content))
	}
	cb := resp.Content[0]
	if cb.Text != "part1 part2" {
		t.Errorf("Content[0].Text = %q, want 'part1 part2'", cb.Text)
	}
	if cb.Signature != "sig1sig2" {
		t.Errorf("Content[0].Signature = %q, want 'sig1sig2'", cb.Signature)
	}

	// OnThinkingDelta fires per thinking_delta, never for signature_delta
	thinkEvents := []string{}
	for _, ev := range sink.events {
		if len(ev) > 6 && ev[:6] == "think:" {
			thinkEvents = append(thinkEvents, ev)
		}
	}
	if len(thinkEvents) != 2 {
		t.Errorf("think event count = %d, want 2; events=%v", len(thinkEvents), thinkEvents)
	}
}

func TestRun_Thinking_BeforeToolUseOrdering(t *testing.T) {
	var b bytes.Buffer
	b.WriteString(sseEvent("message_start",
		`{"type":"message_start","message":{"id":"msg_o","type":"message","role":"assistant","model":"claude-opus-4-7","content":[],"usage":{"input_tokens":5,"output_tokens":0}}}`))
	b.WriteString(sseEvent("content_block_start",
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reasoning"}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sigX"}}`))
	b.WriteString(sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`))
	b.WriteString(sseEvent("content_block_start",
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"Bash","input":{}}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}`))
	b.WriteString(sseEvent("content_block_stop", `{"type":"content_block_stop","index":1}`))
	b.WriteString(sseEvent("message_delta",
		`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"input_tokens":5,"output_tokens":8}}`))
	b.WriteString(sseEvent("message_stop", `{"type":"message_stop"}`))

	p := thinkingProvider(b.String())
	sink := &recordingSink{}
	resp, err := p.Run(context.Background(), thinkingBaseReq(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// thinking block precedes tool_use in resp.Content
	if len(resp.Content) != 2 {
		t.Fatalf("Content len = %d, want 2", len(resp.Content))
	}
	if resp.Content[0].Type != "thinking" {
		t.Errorf("Content[0].Type = %q, want thinking", resp.Content[0].Type)
	}
	if resp.Content[1].Type != "tool_use" {
		t.Errorf("Content[1].Type = %q, want tool_use", resp.Content[1].Type)
	}

	// think event precedes tool_start event in callback order
	thinkIdx, toolIdx := -1, -1
	for i, ev := range sink.events {
		if len(ev) > 6 && ev[:6] == "think:" && thinkIdx < 0 {
			thinkIdx = i
		}
		if len(ev) >= 10 && ev[:10] == "tool_start" && toolIdx < 0 {
			toolIdx = i
		}
	}
	if thinkIdx < 0 {
		t.Errorf("no think event; events=%v", sink.events)
	}
	if toolIdx < 0 {
		t.Errorf("no tool_start event; events=%v", sink.events)
	}
	if thinkIdx >= toolIdx {
		t.Errorf("think event (%d) must precede tool_start (%d)", thinkIdx, toolIdx)
	}
}
