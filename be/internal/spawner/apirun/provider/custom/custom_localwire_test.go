package custom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// localWireRecordingSink captures the callbacks Run emits, mirroring
// openaichat/decode_test.go's recordingSink (kept minimal — only the fields
// this round-trip test asserts on).
type localWireRecordingSink struct {
	usage provider.Usage
}

func (s *localWireRecordingSink) OnTextDelta(string)                    {}
func (s *localWireRecordingSink) OnThinkingDelta(string)                {}
func (s *localWireRecordingSink) OnToolUseStart(string, string)         {}
func (s *localWireRecordingSink) OnToolUseInputDelta(string, string)    {}
func (s *localWireRecordingSink) OnToolUseStop(string, json.RawMessage) {}
func (s *localWireRecordingSink) OnUsage(u provider.Usage)              { s.usage = u }

// TestCustomNew_WireChatCompletions_RealHTTPRoundTrip is the optional
// wire-level check: custom.New(Wire=chat_completions) driven against a real
// httptest.Server (not a fake RoundTripper) standing in for Ollama's /v1
// endpoint — end to end through the actual net/http client custom.New wires
// up, decoding a genuine SSE response into a FinalResponse.
func TestCustomNew_WireChatCompletions_RealHTTPRoundTrip(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: " +
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"local-model",` +
			`"choices":[{"index":0,"delta":{"content":"pong"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	p := New(Config{Name: "local-ollama", BaseURL: srv.URL, APIKey: "", Wire: WireChatCompletions})

	sink := &localWireRecordingSink{}
	resp, err := p.Run(context.Background(), minimalRequest(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("request path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "" {
		t.Errorf("Authorization header = %q, want empty (blank api_key, no bearer)", gotAuth)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", resp.StopReason)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "pong" {
		t.Errorf("Content = %+v, want one text block 'pong'", resp.Content)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 1 {
		t.Errorf("Usage = %+v, want in=5 out=1", resp.Usage)
	}
	if sink.usage.InputTokens != 5 {
		t.Errorf("sink.usage = %+v, want in=5", sink.usage)
	}
}

// TestCustomNew_WireOllamaNative_RealHTTPRoundTrip verifies
// custom.New(Wire=ollama_native) routes to Ollama's native /api/chat NDJSON
// endpoint (not /v1/chat/completions), driven against a real httptest.Server
// since ollamanative.New ignores any openai-go options — custom.New must be
// used directly (not NewWithHTTPClient, which only wires openai-go
// transports).
func TestCustomNew_WireOllamaNative_RealHTTPRoundTrip(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"message":{"content":"pong"},"done":true,"done_reason":"stop",` +
			`"prompt_eval_count":5,"eval_count":1}` + "\n"))
	}))
	defer srv.Close()

	p := New(Config{Name: "local-ollama", BaseURL: srv.URL, APIKey: "", Wire: WireOllamaNative})

	sink := &localWireRecordingSink{}
	resp, err := p.Run(context.Background(), minimalRequest(), sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotPath != "/api/chat" {
		t.Errorf("request path = %q, want /api/chat", gotPath)
	}
	if gotAuth != "" {
		t.Errorf("Authorization header = %q, want empty (blank api_key, no bearer)", gotAuth)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", resp.StopReason)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "pong" {
		t.Errorf("Content = %+v, want one text block 'pong'", resp.Content)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 1 {
		t.Errorf("Usage = %+v, want in=5 out=1", resp.Usage)
	}
	if p.Name() != "local-ollama" {
		t.Errorf("Name() = %q, want local-ollama (custom wrapper's own name)", p.Name())
	}
}
