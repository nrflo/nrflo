// Package custom is a wire-selecting thin wrapper for BYO OpenAI-compatible
// API providers registered in the custom_providers table: a local/self-hosted
// server (Ollama, LM Studio, llama.cpp) or any other OpenAI-compatible
// endpoint. It mirrors the openrouter->openai thin-wrapper pattern
// (be/internal/spawner/apirun/provider/openrouter/openrouter.go): no
// translate/decode logic is duplicated here, it just constructs the right
// inner provider for the registry row's api_wire and reports its own Name().
package custom

import (
	"context"
	"net/http"

	"github.com/openai/openai-go/v3/option"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/ollamanative"
	"be/internal/spawner/apirun/provider/openai"
	"be/internal/spawner/apirun/provider/openaichat"
)

// Wire enum values, mirroring
// service.APIWireResponses/APIWireChatCompletions/APIWireOllamaNative.
const (
	WireResponses       = "responses"
	WireChatCompletions = "chat_completions"
	WireOllamaNative    = "ollama_native"
)

// Config identifies one custom_providers row's connection details.
type Config struct {
	Name    string
	BaseURL string
	APIKey  string
	// Wire selects the wire protocol: WireResponses (default — the
	// non-stateful /v1/responses API the openai provider speaks) or
	// WireChatCompletions (for servers that only speak /v1/chat/completions,
	// e.g. llama.cpp's llama-server).
	Wire string
}

// New returns a provider.Provider for a registered custom provider. Unlike
// openrouter.New, credentials come directly from the DB-stored row (no
// env-ladder) — cfg.APIKey may be empty for local servers with no auth. opts
// are openai-go request options and only apply to the responses/
// chat_completions wires; ollamanative speaks net/http directly and ignores
// them.
func New(cfg Config, opts ...option.RequestOption) provider.Provider {
	var inner provider.Provider
	switch cfg.Wire {
	case WireChatCompletions:
		inner = openaichat.New(openaichat.Credentials{Value: cfg.APIKey, BaseURL: cfg.BaseURL}, opts...)
	case WireOllamaNative:
		inner = ollamanative.New(ollamanative.Credentials{Value: cfg.APIKey, BaseURL: cfg.BaseURL})
	default:
		inner = openai.New(openai.Credentials{Value: cfg.APIKey, BaseURL: cfg.BaseURL}, opts...)
	}
	return &customProvider{name: cfg.Name, inner: inner}
}

// NewWithHTTPClient is a convenience constructor for tests: it injects the
// given *http.Client (fake transport) before applying credentials.
func NewWithHTTPClient(cfg Config, hc *http.Client) provider.Provider {
	return New(cfg, option.WithHTTPClient(hc))
}

type customProvider struct {
	name  string
	inner provider.Provider
}

func (p *customProvider) Name() string { return p.name }

// MaxContext returns a row-independent default; real context windows for
// custom providers come from models.api_context (user-configured, no seeded
// catalog) — mirrors openrouter.
func (p *customProvider) MaxContext(_ string) int { return 128000 }

func (p *customProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	return p.inner.Run(ctx, req, sink)
}
