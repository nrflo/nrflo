// Package openaichat implements provider.Provider on top of the official
// github.com/openai/openai-go SDK using the Chat Completions API with
// streaming. Used for OpenAI-compatible servers that do not support the
// Responses API (e.g. llama.cpp's llama-server) — see the custom provider
// wrapper (be/internal/spawner/apirun/provider/custom) which selects this
// wire by api_wire=="chat_completions". Only API-key authentication is
// supported (key may be empty for local servers with no auth).
package openaichat

import (
	"context"
	"fmt"
	"net/http"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"be/internal/spawner/apirun/provider"
)

// Credentials holds an API key and base URL for an OpenAI-compatible
// chat-completions endpoint. Unlike the openai/openrouter packages, these are
// not env-ladder resolved: the custom provider wrapper passes the
// custom_providers-stored value directly.
type Credentials struct {
	Value   string
	BaseURL string
}

// New returns a provider.Provider backed by the Chat Completions API.
func New(creds Credentials, opts ...option.RequestOption) provider.Provider {
	var all []option.RequestOption
	if creds.Value != "" {
		all = append(all, option.WithAPIKey(creds.Value))
	}
	if creds.BaseURL != "" {
		all = append(all, option.WithBaseURL(creds.BaseURL))
	}
	all = append(all, opts...)
	client := openaisdk.NewClient(all...)
	return &openaiChatProvider{client: client}
}

// NewWithHTTPClient is a convenience constructor for tests: it injects the
// given *http.Client (fake transport) before applying credentials.
func NewWithHTTPClient(creds Credentials, hc *http.Client) provider.Provider {
	return New(creds, option.WithHTTPClient(hc))
}

type openaiChatProvider struct {
	client openaisdk.Client
}

func (p *openaiChatProvider) Name() string { return "openaichat" }

// MaxContext returns a flat default; real context windows for custom
// providers come from models.api_context (user-configured, no seeded
// catalog).
func (p *openaiChatProvider) MaxContext(_ string) int { return 128000 }

func (p *openaiChatProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	params, err := translateRequest(req)
	if err != nil {
		return nil, fmt.Errorf("translate openaichat request: %w", err)
	}
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()
	return decodeStream(stream, sink)
}
