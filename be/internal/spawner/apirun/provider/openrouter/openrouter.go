package openrouter

import (
	"context"
	"net/http"

	"github.com/openai/openai-go/v3/option"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/openai"
)

// New returns a provider.Provider backed by OpenRouter's /responses beta,
// which is wire-compatible with the openai provider's decoder (smoke verdict
// openrouter_responses_compat). This is a thin wrapper: it constructs the
// existing openai provider with the OpenRouter base URL + key and reports its
// own Name(); no translate/decode logic is duplicated here.
func New(creds Credentials, opts ...option.RequestOption) provider.Provider {
	baseURL := creds.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	inner := openai.New(openai.Credentials{Value: creds.Value, BaseURL: baseURL}, opts...)
	return &openrouterProvider{inner: inner}
}

// NewWithHTTPClient is a convenience constructor for tests: it injects the
// given *http.Client (fake transport) before applying credentials.
func NewWithHTTPClient(creds Credentials, hc *http.Client) provider.Provider {
	return New(creds, option.WithHTTPClient(hc))
}

type openrouterProvider struct {
	inner provider.Provider
}

func (p *openrouterProvider) Name() string { return "openrouter" }

// MaxContext returns a row-independent default; real context windows for
// openrouter models come from models.api_context (user-configured, no seeded
// catalog).
func (p *openrouterProvider) MaxContext(_ string) int { return 128000 }

func (p *openrouterProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	return p.inner.Run(ctx, req, sink)
}
