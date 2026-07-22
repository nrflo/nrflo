package custom

import (
	"context"
	"net/http"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// roundTripFunc lets a test supply a bare http.RoundTripper as a func value,
// mirroring openrouter_test.go / openai's fakeRoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func recordingTransport(gotHost, gotPath *string) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			*gotHost = req.URL.Host
			*gotPath = req.URL.Path
			return &http.Response{
				StatusCode: 401, // any response is fine; we only assert routing
				Body:       http.NoBody,
				Header:     make(http.Header),
			}, nil
		}),
	}
}

func minimalRequest() provider.Request {
	return provider.Request{
		Model:     "llama3",
		MaxTokens: 10,
		Messages: []provider.Message{
			{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}},
		},
	}
}

// TestNew_Name verifies the wrapper reports the registry row's own name, not
// the inner provider's.
func TestNew_Name(t *testing.T) {
	p := New(Config{Name: "local-ollama", BaseURL: "http://localhost:11434/v1"})
	if p.Name() != "local-ollama" {
		t.Errorf("Name() = %q, want local-ollama", p.Name())
	}
}

// TestNew_MaxContext_RowIndependentDefault mirrors openrouter's flat default.
func TestNew_MaxContext_RowIndependentDefault(t *testing.T) {
	p := New(Config{Name: "local-ollama", BaseURL: "http://localhost:11434/v1"})
	for _, model := range []string{"", "anything", "llama3"} {
		if got := p.MaxContext(model); got != 128000 {
			t.Errorf("MaxContext(%q) = %d, want 128000", model, got)
		}
	}
}

// TestNew_WireResponses_RoutesToConfiguredBaseURL_ResponsesPath verifies the
// default (empty Wire, and explicit WireResponses) selects the openai
// (Responses API) inner provider and targets the configured base URL.
func TestNew_WireResponses_RoutesToConfiguredBaseURL_ResponsesPath(t *testing.T) {
	for _, wire := range []string{"", WireResponses} {
		t.Run("wire="+wire, func(t *testing.T) {
			var gotHost, gotPath string
			hc := recordingTransport(&gotHost, &gotPath)
			p := NewWithHTTPClient(Config{
				Name: "local-ollama", BaseURL: "http://localhost:11434/v1", Wire: wire,
			}, hc)
			_, _ = p.Run(context.Background(), minimalRequest(), nil)
			if gotHost != "localhost:11434" {
				t.Errorf("request host = %q, want localhost:11434", gotHost)
			}
			if gotPath != "/v1/responses" {
				t.Errorf("request path = %q, want /v1/responses (Responses API wire)", gotPath)
			}
		})
	}
}

// TestNew_WireChatCompletions_RoutesToChatCompletionsPath verifies
// api_wire="chat_completions" selects the openaichat inner provider, which
// hits /chat/completions instead of /responses.
func TestNew_WireChatCompletions_RoutesToChatCompletionsPath(t *testing.T) {
	var gotHost, gotPath string
	hc := recordingTransport(&gotHost, &gotPath)
	p := NewWithHTTPClient(Config{
		Name: "local-llamacpp", BaseURL: "http://localhost:8080/v1", Wire: WireChatCompletions,
	}, hc)
	_, _ = p.Run(context.Background(), minimalRequest(), nil)
	if gotHost != "localhost:8080" {
		t.Errorf("request host = %q, want localhost:8080", gotHost)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("request path = %q, want /v1/chat/completions (chat_completions wire)", gotPath)
	}
}

// TestNew_NameReflectsWireSelection verifies Name() stays the configured
// registry name regardless of which wire was selected (custom's own Name(),
// never the inner provider's "openaichat"/openai name).
func TestNew_NameReflectsWireSelection(t *testing.T) {
	for _, wire := range []string{WireResponses, WireChatCompletions, WireOllamaNative} {
		p := New(Config{Name: "my-provider", BaseURL: "http://localhost:9/v1", Wire: wire})
		if p.Name() != "my-provider" {
			t.Errorf("wire=%s: Name() = %q, want my-provider", wire, p.Name())
		}
	}
}
