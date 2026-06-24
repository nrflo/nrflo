package tools_builtin

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/tools_web"
)

const maxSearchQueries = 8

// webSearchHandler implements web_search: fan-out web search across queries via
// the configured provider, deduped by URL with a per-domain cap.
type webSearchHandler struct{}

func (webSearchHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "web_search",
		Description: "Search the web across one or more queries (run concurrently). Returns JSON {\"results\":[{\"query\",\"url\",\"title\",\"snippet\",\"score\"}],\"failed_queries\":[...]} — results are deduped by URL with a per-domain cap. Use web_fetch to read a result's full content.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"queries":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":8,"description":"Search queries (run concurrently)"},
"max_results_per_query":{"type":"integer","description":"Optional cap per query"}
},
"required":["queries"],
"additionalProperties":false
}`),
	}
}

type searchRow struct {
	Query   string  `json:"query"`
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

func (webSearchHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Queries            []string `json:"queries"`
		MaxResultsPerQuery int      `json:"max_results_per_query"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if len(args.Queries) == 0 {
		return "queries is required", true, nil
	}
	if len(args.Queries) > maxSearchQueries {
		args.Queries = args.Queries[:maxSearchQueries]
	}
	if env.Pool == nil {
		return missingService("pool")
	}

	resolver := tools_web.NewResolver(env.Pool, env.ProjectID)
	sp, err := resolver.SearchProvider()
	if err != nil {
		return err.Error(), true, nil
	}
	maxResults := args.MaxResultsPerQuery
	if maxResults <= 0 {
		maxResults = resolver.MaxResultsPerQuery()
	}

	perQuery := make([][]searchRow, len(args.Queries))
	failed := make([]string, len(args.Queries))
	var wg sync.WaitGroup
	for i, q := range args.Queries {
		wg.Add(1)
		go func(i int, q string) {
			defer wg.Done()
			results, serr := sp.Search(ctx, q, tools_web.SearchOpts{MaxResults: maxResults})
			if serr != nil {
				failed[i] = q // surfaced to the agent as failed_queries
				return
			}
			if len(results) > maxResults {
				results = results[:maxResults] // defend against a provider ignoring the cap
			}
			rows := make([]searchRow, 0, len(results))
			for _, r := range results {
				rows = append(rows, searchRow{Query: q, URL: r.URL, Title: r.Title, Snippet: r.Snippet, Score: r.Score})
			}
			perQuery[i] = rows
		}(i, q)
	}
	wg.Wait()

	var all []searchRow
	for _, rows := range perQuery {
		all = append(all, rows...)
	}
	deduped := dedupeAndCap(all, resolver.MaxPerDomain())

	payload := map[string]any{"results": deduped}
	var failedQueries []string
	for _, q := range failed {
		if q != "" {
			failedQueries = append(failedQueries, q)
		}
	}
	if len(failedQueries) > 0 {
		payload["failed_queries"] = failedQueries
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(out), false, nil
}

// dedupeAndCap drops duplicate URLs (normalized) and caps results per registered
// domain to curb echo-chamber sources. Order is preserved.
func dedupeAndCap(rows []searchRow, perDomain int) []searchRow {
	seen := map[string]bool{}
	domainCount := map[string]int{}
	out := make([]searchRow, 0, len(rows))
	for _, r := range rows {
		key := normalizeURL(r.URL)
		if key == "" || seen[key] {
			continue
		}
		host := hostOf(r.URL)
		if perDomain > 0 && domainCount[host] >= perDomain {
			continue
		}
		seen[key] = true
		domainCount[host]++
		out = append(out, r)
	}
	return out
}

func normalizeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.TrimSpace(raw))
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	path := strings.TrimRight(u.Path, "/")
	return host + strings.ToLower(path)
}

func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}
