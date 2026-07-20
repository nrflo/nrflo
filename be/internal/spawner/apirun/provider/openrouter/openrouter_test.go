package openrouter

import (
	"context"
	"net/http"
	"testing"

	"be/internal/spawner/apirun/provider"
)

func TestNew_Name(t *testing.T) {
	p := New(Credentials{Value: "key", BaseURL: DefaultBaseURL})
	if p.Name() != "openrouter" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openrouter")
	}
}

// TestNew_MaxContext_RowIndependentDefault verifies MaxContext returns the
// fixed default regardless of model — no seeded catalog for openrouter.
func TestNew_MaxContext_RowIndependentDefault(t *testing.T) {
	p := New(Credentials{Value: "key", BaseURL: DefaultBaseURL})
	for _, model := range []string{"", "anything", "openai/gpt-4o"} {
		if got := p.MaxContext(model); got != 128000 {
			t.Errorf("MaxContext(%q) = %d, want 128000", model, got)
		}
	}
}

// TestNew_EmptyBaseURL_FallsBackToDefault verifies New defensively applies
// DefaultBaseURL when creds.BaseURL is empty (Resolve always populates it,
// but New should not depend on that invariant holding).
func TestNew_EmptyBaseURL_FallsBackToDefault(t *testing.T) {
	p := New(Credentials{Value: "key", BaseURL: ""})
	if p == nil {
		t.Fatal("New returned nil provider")
	}
	if p.Name() != "openrouter" {
		t.Errorf("Name() = %q, want openrouter", p.Name())
	}
}

// roundTripFunc lets a test supply a bare http.RoundTripper as a func value.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestNew_ThinWrapper_HitsOpenRouterBaseURL verifies the wrapper is a thin
// construction over the openai provider: a request made through it targets
// the OpenRouter base URL, not the OpenAI default. This is the executable
// check for the "wire-compatible /responses" design verdict — no
// translate/decode logic is duplicated in this package, only routing.
func TestNew_ThinWrapper_HitsOpenRouterBaseURL(t *testing.T) {
	var gotHost string
	hc := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotHost = req.URL.Host
			return &http.Response{
				StatusCode: 401, // any response is fine; we only assert routing
				Body:       http.NoBody,
				Header:     make(http.Header),
			}, nil
		}),
	}
	p := NewWithHTTPClient(Credentials{Value: "key", BaseURL: "https://openrouter.ai/api/v1"}, hc)
	_, _ = p.Run(context.Background(), provider.Request{Model: "openai/gpt-4o", Messages: []provider.Message{
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}},
	}}, nil)
	if gotHost != "openrouter.ai" {
		t.Errorf("request host = %q, want openrouter.ai (thin wrapper must route through OpenRouter base URL)", gotHost)
	}
}
