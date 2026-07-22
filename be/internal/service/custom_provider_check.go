package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const customProviderCheckTimeout = 10 * time.Second

type customProviderModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// ollamaTagsResponse is Ollama's native /api/tags listing shape — no /v1
// prefix, and models are keyed by "name" rather than OpenAI's "id".
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// CheckConnection probes a custom provider's model-listing endpoint and
// returns the advertised model ids. apiWire selects the endpoint shape:
// ollama_native probes Ollama's native /api/tags (base has no /v1); the
// OpenAI-compatible wires (responses, chat_completions) probe /models.
func (s *CustomProviderService) CheckConnection(baseURL, apiKey, apiWire string) ([]string, error) {
	base, err := validateBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if apiWire == APIWireOllamaNative {
		return checkOllamaNativeConnection(base, apiKey)
	}

	url := strings.TrimRight(base, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: customProviderCheckTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s returned status %d", url, resp.StatusCode)
	}

	var body customProviderModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to decode response from %s: %w", url, err)
	}

	ids := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// checkOllamaNativeConnection probes Ollama's native /api/tags listing
// (base has no /v1 segment, unlike the OpenAI-compatible wires).
func checkOllamaNativeConnection(base, apiKey string) ([]string, error) {
	url := strings.TrimRight(base, "/") + "/api/tags"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: customProviderCheckTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s returned status %d", url, resp.StatusCode)
	}

	var body ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to decode response from %s: %w", url, err)
	}

	names := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		names = append(names, m.Name)
	}
	return names, nil
}
