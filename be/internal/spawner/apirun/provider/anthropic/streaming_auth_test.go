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
	if strings.Contains(betaHdr, "prompt-caching") {
		t.Errorf("anthropic-beta = %q, legacy prompt-caching beta header must not be sent (caching is GA)", betaHdr)
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
	if strings.Contains(betaHdr, "prompt-caching") {
		t.Errorf("anthropic-beta = %q, legacy prompt-caching beta header must not be sent (caching is GA)", betaHdr)
	}
}

// TestRun_OAuthBearer_PrependsClaudeCodeIdentity pins the
// Anthropic-mandated leading system block. Without it, sonnet/opus return
// 429 rate_limit_error on OAuth-bearer auth (haiku is exempt). The block
// must be the FIRST system entry; any caller-supplied system text follows.
func TestRun_OAuthBearer_PrependsClaudeCodeIdentity(t *testing.T) {
	rt := &fakeRoundTripper{body: minimalSSE()}
	p := NewWithHTTPClient(Credentials{Value: "sk-ant-oat01-mytoken", Method: MethodOAuthBearer}, &http.Client{Transport: rt})

	req := minimalRequest()
	req.System = "be brief"
	if _, err := p.Run(context.Background(), req, &recordingSink{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body := string(rt.lastBody)
	if !strings.Contains(body, "You are Claude Code, Anthropic's official CLI for Claude.") {
		t.Errorf("OAuth body missing Claude Code identity block; body=%s", body)
	}
	idIdx := strings.Index(body, "You are Claude Code")
	userIdx := strings.Index(body, "be brief")
	if idIdx < 0 || userIdx < 0 {
		t.Fatalf("missing markers: identity=%d user-system=%d body=%s", idIdx, userIdx, body)
	}
	if idIdx > userIdx {
		t.Errorf("identity block must precede caller system text: identity=%d user-system=%d", idIdx, userIdx)
	}
}

// TestRun_OAuthBearer_PrependsIdentity_NoUserSystem covers the case where
// the caller supplied no system prompt — the identity block must still be
// the sole system entry, otherwise premium models get 429s.
func TestRun_OAuthBearer_PrependsIdentity_NoUserSystem(t *testing.T) {
	rt := &fakeRoundTripper{body: minimalSSE()}
	p := NewWithHTTPClient(Credentials{Value: "sk-ant-oat01-mytoken", Method: MethodOAuthBearer}, &http.Client{Transport: rt})

	if _, err := p.Run(context.Background(), minimalRequest(), &recordingSink{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body := string(rt.lastBody)
	if !strings.Contains(body, "You are Claude Code, Anthropic's official CLI for Claude.") {
		t.Errorf("OAuth body missing Claude Code identity block; body=%s", body)
	}
}

// TestRun_APIKey_DoesNotPrependIdentity ensures we don't waste a system
// slot (or alter cache keys) for API-key auth, which has no identity gate.
func TestRun_APIKey_DoesNotPrependIdentity(t *testing.T) {
	rt := &fakeRoundTripper{body: minimalSSE()}
	p := NewWithHTTPClient(Credentials{Value: "sk-test-apikey", Method: MethodAPIKey}, &http.Client{Transport: rt})

	req := minimalRequest()
	req.System = "be brief"
	if _, err := p.Run(context.Background(), req, &recordingSink{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body := string(rt.lastBody)
	if strings.Contains(body, "You are Claude Code") {
		t.Errorf("API-key body must NOT contain Claude Code identity block; body=%s", body)
	}
}
