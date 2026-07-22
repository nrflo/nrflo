// Package ollamanative implements provider.Provider on top of Ollama's
// native NDJSON POST /api/chat endpoint using net/http only (no openai-go
// SDK). Selected by the custom provider wrapper
// (be/internal/spawner/apirun/provider/custom) when a custom_providers row's
// api_wire=="ollama_native" — the only wire that can send Ollama's
// think:false to disable hybrid-thinking models, since the OpenAI-compatible
// /v1 wires (openai/openaichat) have no equivalent knob. Only API-key
// authentication is supported (key may be empty for local servers with no
// auth).
package ollamanative

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"be/internal/spawner/apirun/provider"
)

// Credentials holds an API key and base URL for an Ollama-compatible native
// /api/chat endpoint. Not env-ladder resolved: the custom provider wrapper
// passes the custom_providers-stored value directly.
type Credentials struct {
	Value   string
	BaseURL string
}

// New returns a provider.Provider backed by Ollama's native /api/chat.
func New(creds Credentials) provider.Provider {
	return &ollamaNativeProvider{creds: creds, client: http.DefaultClient}
}

type ollamaNativeProvider struct {
	creds  Credentials
	client *http.Client
}

func (p *ollamaNativeProvider) Name() string { return "ollamanative" }

// MaxContext returns a flat default; real context windows for custom
// providers come from models.api_context (user-configured, no seeded
// catalog).
func (p *ollamaNativeProvider) MaxContext(_ string) int { return 128000 }

func (p *ollamaNativeProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	body, err := translateRequest(req)
	if err != nil {
		return nil, fmt.Errorf("translate ollamanative request: %w", err)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal ollamanative request: %w", err)
	}

	url := strings.TrimRight(p.creds.BaseURL, "/") + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build ollamanative request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.creds.Value != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.creds.Value)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollamanative request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("ollamanative %s returned status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return decodeStream(resp.Body, sink)
}
