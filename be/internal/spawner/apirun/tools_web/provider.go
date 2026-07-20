// Package tools_web provides a provider-agnostic web search + fetch layer for
// nrflo's in-process tools. The default providers are a self-hosted SearXNG
// instance (search) and a direct, SSRF-guarded readability fetch (fetch); the
// web_search / web_fetch builtin handlers (tools_builtin) resolve the active
// provider by name from config and call these interfaces, so swapping in a
// different provider is a config change with no handler edits (CLAUDE.md
// Rule 6: polymorphism in the impl). Egress (egress.go/egress_dial.go)
// supplies each provider's http.Client and is proxy-aware via WEB_PROXY_URL.
package tools_web

import "context"

// Result is one normalized web search hit.
type Result struct {
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

// SearchOpts tunes a single search call.
type SearchOpts struct {
	MaxResults int // 0 -> provider default
}

// SearchProvider discovers URLs for a query. Implementations live in one file
// each (searxng.go, ...) and self-register via init() into the registry.
type SearchProvider interface {
	Name() string
	Search(ctx context.Context, query string, opts SearchOpts) ([]Result, error)
}

// Page is a normalized fetched document. OK=false carries Err and an empty
// Markdown; callers surface the drop rather than failing the agent.
type Page struct {
	URL      string `json:"url"`
	Markdown string `json:"-"`
	Bytes    int    `json:"bytes"`
	OK       bool   `json:"ok"`
	Err      string `json:"error,omitempty"`
}

// FetchProvider retrieves a single URL as clean markdown. The anti-bot
// burden lives here; the default (direct) provider does not render JS, so
// JS-gated pages surface as Page{OK:false} rather than garbage markdown.
type FetchProvider interface {
	Name() string
	Fetch(ctx context.Context, url string) Page
}
