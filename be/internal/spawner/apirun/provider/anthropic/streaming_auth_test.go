package anthropic

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// minimalSSE returns the smallest valid stream for header-assertion tests.
func minimalSSE() string {
	var b strings.Builder
	b.WriteString(sseEvent("message_start",
		`{"type":"message_start","message":{"id":"msg","type":"message","role":"assistant","model":"m","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`))
	b.WriteString(sseEvent("content_block_start",
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`))
	b.WriteString(sseEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`))
	b.WriteString(sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`))
	b.WriteString(sseEvent("message_delta",
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":1,"output_tokens":1}}`))
	b.WriteString(sseEvent("message_stop", `{"type":"message_stop"}`))
	return b.String()
}

func minimalRequest() provider.Request {
	return provider.Request{
		Model:     "m",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "hi"}},
		}},
	}
}

func TestRun_APIKey_Headers(t *testing.T) {
	rt := &fakeRoundTripper{body: minimalSSE()}
	p := NewWithHTTPClient(Credentials{Value: "sk-test-apikey", Method: MethodAPIKey}, &http.Client{Transport: rt})

	_, err := p.Run(context.Background(), minimalRequest(), &recordingSink{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rt.lastReq == nil {
		t.Fatal("no request recorded")
	}

	if got := rt.lastReq.Header.Get("x-api-key"); got != "sk-test-apikey" {
		t.Errorf("x-api-key = %q, want %q", got, "sk-test-apikey")
	}
	if got := rt.lastReq.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty for API key auth", got)
	}
	betaHdr := rt.lastReq.Header.Get("anthropic-beta")
	if !strings.Contains(betaHdr, "prompt-caching-2024-07-31") {
		t.Errorf("anthropic-beta = %q, want it to contain prompt-caching-2024-07-31", betaHdr)
	}
	if strings.Contains(betaHdr, "oauth-2025-04-20") {
		t.Errorf("anthropic-beta = %q, must NOT contain oauth-2025-04-20 for API key auth", betaHdr)
	}
}

func TestRun_OAuthBearer_Headers(t *testing.T) {
	rt := &fakeRoundTripper{body: minimalSSE()}
	tok := "sk-ant-oat01-mytoken"
	p := NewWithHTTPClient(Credentials{Value: tok, Method: MethodOAuthBearer}, &http.Client{Transport: rt})

	_, err := p.Run(context.Background(), minimalRequest(), &recordingSink{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rt.lastReq == nil {
		t.Fatal("no request recorded")
	}

	if got := rt.lastReq.Header.Get("Authorization"); got != "Bearer "+tok {
		t.Errorf("Authorization = %q, want %q", got, "Bearer "+tok)
	}
	if got := rt.lastReq.Header.Get("x-api-key"); got != "" {
		t.Errorf("x-api-key = %q, want empty for OAuth bearer auth", got)
	}
	// SDK may deliver beta headers as separate entries; join all values.
	betaHdr := strings.Join(rt.lastReq.Header.Values("anthropic-beta"), ",")
	if !strings.Contains(betaHdr, "oauth-2025-04-20") {
		t.Errorf("anthropic-beta = %q, want it to contain oauth-2025-04-20", betaHdr)
	}
	if !strings.Contains(betaHdr, "prompt-caching-2024-07-31") {
		t.Errorf("anthropic-beta = %q, want it to contain prompt-caching-2024-07-31", betaHdr)
	}
}
