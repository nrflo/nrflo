package tools_web

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	ddgTimeout     = 20 * time.Second
	ddgBaseURL     = "https://html.duckduckgo.com/html/"
	ddgMaxBodySize = 2 << 20 // 2MB
)

func init() { registerSearch("ddg", newDDGProvider) }

// ddgProvider implements SearchProvider by scraping DuckDuckGo's plain-HTML
// endpoint. No self-hosted service or API key, which makes it the natural
// last-resort fallback when the operator's SearXNG is down or hollowed out by
// upstream captchas. DDG itself captchas sustained bursts, so it is a
// fallback posture, not a primary.
type ddgProvider struct {
	baseURL string
	client  *http.Client
}

func newDDGProvider(r *Resolver) SearchProvider {
	eg, err := newEgress(r)
	if err != nil {
		return &ddgProvider{}
	}
	return &ddgProvider{baseURL: ddgBaseURL, client: eg.Trusted()}
}

func (d *ddgProvider) Name() string { return "ddg" }

var (
	ddgResultRe  = regexp.MustCompile(`(?s)<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	ddgSnippetRe = regexp.MustCompile(`(?s)<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)
	htmlTagRe    = regexp.MustCompile(`<[^>]+>`)
)

func (d *ddgProvider) Search(ctx context.Context, query string, opts SearchOpts) ([]Result, error) {
	if d.client == nil {
		return nil, fmt.Errorf("ddg: egress not configured (invalid WEB_PROXY_URL)")
	}

	ctx, cancel := context.WithTimeout(ctx, ddgTimeout)
	defer cancel()

	reqURL := d.baseURL + "?" + url.Values{"q": {query}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	applyRotatingHeaders(req)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ddg: request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ddg: status %d (likely captcha/rate-limit)", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, ddgMaxBodySize))
	if err != nil {
		return nil, fmt.Errorf("ddg: read: %w", err)
	}

	n := opts.MaxResults
	if n <= 0 {
		n = defaultMaxResultsPerQuery
	}
	anchors := ddgResultRe.FindAllStringSubmatch(string(body), -1)
	snippets := ddgSnippetRe.FindAllStringSubmatch(string(body), -1)
	out := make([]Result, 0, len(anchors))
	for i, m := range anchors {
		target := ddgDecodeHref(m[1])
		if target == "" {
			continue
		}
		snippet := ""
		if i < len(snippets) {
			snippet = cleanHTMLText(snippets[i][1])
		}
		out = append(out, Result{URL: target, Title: cleanHTMLText(m[2]), Snippet: snippet})
		if len(out) >= n {
			break
		}
	}
	return out, nil
}

// ddgDecodeHref unwraps DDG's redirect link (//duckduckgo.com/l/?uddg=<enc>)
// to the real target; direct external hrefs pass through. Links that stay on
// duckduckgo.com without a uddg param (ads, internal nav) are dropped.
func ddgDecodeHref(href string) string {
	href = html.UnescapeString(href)
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if uddg := u.Query().Get("uddg"); uddg != "" {
		if target, err := url.QueryUnescape(uddg); err == nil {
			return target
		}
		return ""
	}
	if strings.HasSuffix(u.Host, "duckduckgo.com") || u.Host == "" {
		return ""
	}
	return href
}

func cleanHTMLText(s string) string {
	return strings.TrimSpace(html.UnescapeString(htmlTagRe.ReplaceAllString(s, "")))
}
