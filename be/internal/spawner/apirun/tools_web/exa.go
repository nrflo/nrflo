package tools_web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// timeouts for the exa search client.
const exaTimeout = 30 * time.Second

func init() { registerSearch("exa", newExaProvider) }

const exaDefaultBaseURL = "https://api.exa.ai"

// exaProvider implements SearchProvider against the Exa search API.
type exaProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func newExaProvider(r *Resolver) SearchProvider {
	base := r.secret("EXA_BASE_URL")
	if base == "" {
		base = exaDefaultBaseURL
	}
	return &exaProvider{
		baseURL: base,
		apiKey:  r.secret("EXA_API_KEY"),
		client:  newHTTPClient(exaTimeout),
	}
}

func (e *exaProvider) Name() string { return "exa" }

func (e *exaProvider) Search(ctx context.Context, query string, opts SearchOpts) ([]Result, error) {
	if e.apiKey == "" {
		return nil, fmt.Errorf("exa: EXA_API_KEY not set")
	}
	n := opts.MaxResults
	if n <= 0 {
		n = defaultMaxResultsPerQuery
	}
	body, _ := json.Marshal(map[string]any{
		"query":      query,
		"numResults": n,
		"type":       "auto",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exa: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("exa: status %d: %s", resp.StatusCode, string(snippet))
	}

	var parsed struct {
		Results []struct {
			URL   string  `json:"url"`
			Title string  `json:"title"`
			Text  string  `json:"text"`
			Score float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("exa: decode: %w", err)
	}
	out := make([]Result, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		if r.URL == "" {
			continue
		}
		out = append(out, Result{URL: r.URL, Title: r.Title, Snippet: r.Text, Score: r.Score})
	}
	return out, nil
}
