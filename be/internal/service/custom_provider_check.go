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

// CheckConnection probes an OpenAI-compatible server by GETing its /models
// listing and returns the advertised model ids. apiWire is accepted for
// parity with Create/Update but is otherwise unused: the /models listing is
// wire-independent.
func (s *CustomProviderService) CheckConnection(baseURL, apiKey, apiWire string) ([]string, error) {
	base, err := validateBaseURL(baseURL)
	if err != nil {
		return nil, err
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
