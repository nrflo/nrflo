package tools_web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// timeout for the jina reader client (rendering can be slow).
const jinaTimeout = 45 * time.Second

func init() { registerFetch("jina", newJinaProvider) }

const jinaDefaultBaseURL = "https://r.jina.ai"

// jinaProvider implements FetchProvider via Jina Reader (r.jina.ai), which
// renders JS and returns clean markdown — handling most anti-bot/paywall cases.
type jinaProvider struct {
	baseURL string
	apiKey  string // optional; raises rate limits when set
	client  *http.Client
}

func newJinaProvider(r *Resolver) FetchProvider {
	base := r.secret("JINA_BASE_URL")
	if base == "" {
		base = jinaDefaultBaseURL
	}
	return &jinaProvider{
		baseURL: strings.TrimRight(base, "/"),
		apiKey:  r.secret("JINA_API_KEY"),
		client:  newHTTPClient(jinaTimeout),
	}
}

func (j *jinaProvider) Name() string { return "jina" }

func (j *jinaProvider) Fetch(ctx context.Context, target string) Page {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.baseURL+"/"+target, nil)
	if err != nil {
		return Page{URL: target, OK: false, Err: err.Error()}
	}
	req.Header.Set("X-Return-Format", "markdown")
	req.Header.Set("Accept", "text/plain")
	if j.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+j.apiKey)
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return Page{URL: target, OK: false, Err: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Page{URL: target, OK: false, Err: fmt.Sprintf("jina: status %d", resp.StatusCode)}
	}
	md, err := io.ReadAll(resp.Body)
	if err != nil {
		return Page{URL: target, OK: false, Err: err.Error()}
	}
	return Page{URL: target, Markdown: string(md), Bytes: len(md), OK: true}
}
