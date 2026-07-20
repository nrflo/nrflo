package tools_web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// searxngTimeout bounds a single search request.
const searxngTimeout = 30 * time.Second

func init() { registerSearch("searxng", newSearxngProvider) }

// searxngProvider implements SearchProvider against a self-hosted SearXNG
// instance's JSON API (SEARXNG_BASE_URL, formats: [html, json] required in
// the instance's settings.yml — see the actionable error below).
type searxngProvider struct {
	baseURL string
	client  *http.Client
}

func newSearxngProvider(r *Resolver) SearchProvider {
	eg, err := newEgress(r)
	if err != nil {
		return &searxngProvider{baseURL: "", client: nil}
	}
	return &searxngProvider{
		baseURL: strings.TrimRight(r.SearchBaseURL(), "/"),
		client:  eg.Trusted(),
	}
}

func (s *searxngProvider) Name() string { return "searxng" }

func (s *searxngProvider) Search(ctx context.Context, query string, opts SearchOpts) ([]Result, error) {
	if s.client == nil {
		return nil, fmt.Errorf("searxng: egress not configured (invalid WEB_PROXY_URL)")
	}
	if s.baseURL == "" {
		return nil, fmt.Errorf("searxng: SEARXNG_BASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(ctx, searxngTimeout)
	defer cancel()

	reqURL := s.baseURL + "/search?" + url.Values{
		"q":      {query},
		"format": {"json"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng: request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("searxng: status 403 — enable `formats: [html, json]` in the SearXNG instance's settings.yml (json format is disabled by default)")
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("searxng: status %d: %s", resp.StatusCode, string(snippet))
	}

	var parsed struct {
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
			Engine  string  `json:"engine"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("searxng: decode: %w", err)
	}

	n := opts.MaxResults
	if n <= 0 {
		n = defaultMaxResultsPerQuery
	}
	out := make([]Result, 0, len(parsed.Results))
	for _, res := range parsed.Results {
		if res.URL == "" {
			continue
		}
		out = append(out, Result{URL: res.URL, Title: res.Title, Snippet: res.Content, Score: res.Score})
		if len(out) >= n {
			break
		}
	}
	return out, nil
}
