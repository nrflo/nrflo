package tools_web

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// directTimeout bounds an entire fetch (dial + headers + body read +
// readability parse), not just one round trip.
const directTimeout = 30 * time.Second

func init() { registerFetch("direct", newDirectProvider) }

// jsChallengeMarkers are substrings that indicate the response is an
// anti-bot interstitial (Cloudflare "Just a moment...", cf-challenge/
// __cf_chl cookies-and-JS gates, Cloudflare Turnstile) rather than real page
// content. Returning garbage markdown for these is worse than returning
// ok:false with a reason — the agent can then try another URL or give up
// cleanly instead of reasoning over a CAPTCHA page.
var jsChallengeMarkers = []string{
	"Just a moment...",
	"cf-challenge",
	"__cf_chl",
	"Enable JavaScript and cookies to continue",
	"cf-turnstile",
	"Checking your browser before accessing",
}

// directProvider implements FetchProvider via direct HTTP + local readability
// extraction (no JS rendering — see the accepted-tradeoff note in provider.go
// and this package's CLAUDE.md paragraph).
type directProvider struct {
	client   *http.Client
	maxBytes int64
}

func newDirectProvider(r *Resolver) FetchProvider {
	eg, err := newEgress(r)
	if err != nil {
		return &directProvider{client: nil, maxBytes: int64(r.MaxBytes())}
	}
	return &directProvider{client: eg.Guarded(), maxBytes: eg.MaxBytes()}
}

func (d *directProvider) Name() string { return "direct" }

func (d *directProvider) Fetch(ctx context.Context, target string) Page {
	if d.client == nil {
		return Page{URL: target, OK: false, Err: "direct: egress not configured (invalid WEB_PROXY_URL)"}
	}

	parsedURL, err := url.Parse(target)
	if err != nil {
		return Page{URL: target, OK: false, Err: fmt.Sprintf("direct: invalid url: %s", err)}
	}

	ctx, cancel := context.WithTimeout(ctx, directTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Page{URL: target, OK: false, Err: err.Error()}
	}
	applyRotatingHeaders(req)

	resp, err := d.client.Do(req)
	if err != nil {
		return Page{URL: target, OK: false, Err: err.Error()}
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return Page{URL: target, OK: false, Err: fmt.Sprintf("direct: status %d", resp.StatusCode)}
	}

	body, err := io.ReadAll(limitedBody(resp.Body, d.maxBytes))
	if err != nil {
		return Page{URL: target, OK: false, Err: fmt.Sprintf("direct: read body: %s", err)}
	}

	if marker, hit := detectJSChallenge(body); hit {
		return Page{URL: target, OK: false, Err: fmt.Sprintf("direct: JS-challenge page detected (%q) — no JS rendering in this fetcher", marker)}
	}

	article, err := readability.FromReader(bytes.NewReader(body), parsedURL)
	if err != nil {
		return Page{URL: target, OK: false, Err: fmt.Sprintf("direct: readability parse: %s", err)}
	}
	if article.Node == nil {
		return Page{URL: target, OK: false, Err: "direct: no readable content extracted"}
	}

	var htmlBuf bytes.Buffer
	if err := article.RenderHTML(&htmlBuf); err != nil {
		return Page{URL: target, OK: false, Err: fmt.Sprintf("direct: render article: %s", err)}
	}
	md, err := htmltomarkdown.ConvertString(htmlBuf.String())
	if err != nil {
		return Page{URL: target, OK: false, Err: fmt.Sprintf("direct: markdown convert: %s", err)}
	}

	return Page{URL: target, Markdown: md, Bytes: len(md), OK: true}
}

// detectJSChallenge scans the raw (pre-readability) body for known anti-bot
// interstitial markers. Deliberately crude substring matching — this is
// meant to catch the common, static interstitial pages, not to be a general
// bot-detection classifier.
func detectJSChallenge(body []byte) (string, bool) {
	s := string(body)
	for _, marker := range jsChallengeMarkers {
		if strings.Contains(s, marker) {
			return marker, true
		}
	}
	return "", false
}
