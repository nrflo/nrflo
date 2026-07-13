// Package openai implements provider.Provider on top of the official
// github.com/openai/openai-go SDK using the Responses API with streaming.
// Only API-key authentication is supported.
package openai

import (
	"context"
	"fmt"
	"net/http"
	"os"

	openaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"be/internal/spawner/apirun/provider"
)

// New returns a provider.Provider backed by the OpenAI Responses API.
// The api key from creds is applied via option.WithAPIKey; optional opts are
// forwarded to the SDK client (e.g. for injecting a fake http.Client in tests).
// OPENAI_BASE_URL and OPENAI_ORG_ID environment variables are applied when set.
func New(creds Credentials, opts ...option.RequestOption) provider.Provider {
	all := []option.RequestOption{option.WithAPIKey(creds.Value)}
	if base := os.Getenv("OPENAI_BASE_URL"); base != "" {
		all = append(all, option.WithBaseURL(base))
	}
	if org := os.Getenv("OPENAI_ORG_ID"); org != "" {
		all = append(all, option.WithOrganization(org))
	}
	all = append(all, opts...)
	client := openaisdk.NewClient(all...)
	return &openaiProvider{client: client}
}

// NewWithHTTPClient is a convenience constructor for tests: it injects the
// given *http.Client (fake transport) before applying credentials.
func NewWithHTTPClient(creds Credentials, hc *http.Client) provider.Provider {
	return New(creds, option.WithHTTPClient(hc))
}

type openaiProvider struct {
	client openaisdk.Client
}

func (p *openaiProvider) Name() string { return "openai" }

// MaxContext returns the model's input context window in tokens.
// Values are hardcoded; unknown models default to 128k.
func (p *openaiProvider) MaxContext(model string) int {
	switch model {
	case "gpt-5", "gpt-5-mini":
		return 1000000
	case "gpt-5.3-codex", "gpt-5.4", "gpt-5.4-mini", "gpt-5.5", "gpt-5.5-mini":
		return 200000
	case "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna":
		return 372000
	case "gpt-4.1", "gpt-4.1-mini", "gpt-4.1-nano":
		return 1047576
	case "o4-mini", "o3":
		return 200000
	case "o3-mini":
		return 200000
	}
	return 128000
}

// Run executes a single streaming model turn using the Responses API. It
// blocks until the stream terminates, invoking sink callbacks in event order
// and returning the assembled FinalResponse.
func (p *openaiProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	params, err := translateRequest(req)
	if err != nil {
		return nil, fmt.Errorf("translate openai request: %w", err)
	}
	stream := p.client.Responses.NewStreaming(ctx, params)
	defer stream.Close()
	return decodeStream(stream, sink)
}
