package tools_builtin

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"sync"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/tools_web"
)

const (
	maxFetchURLs     = 20
	fetchConcurrency = 6
)

// webFetchHandler implements web_fetch: concurrently fetch URLs as clean
// markdown via the configured provider. Full content is offloaded to an
// artifact; an excerpt is returned inline to keep the agent's context bounded.
type webFetchHandler struct{}

func (webFetchHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "web_fetch",
		Description: "Fetch one or more URLs as clean markdown. Returns JSON {\"pages\":[{\"url\",\"ok\",\"excerpt\",\"artifact_name\",\"bytes\",\"error\"}]}. Each successful page returns an excerpt inline; when truncated, the full content is stored under artifact_name (read it with artifact_get). Blocked or failed fetches return ok:false with an error and do NOT fail the turn.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"urls":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":20,"description":"Absolute https URLs to fetch"}
},
"required":["urls"],
"additionalProperties":false
}`),
	}
}

type fetchRow struct {
	URL          string `json:"url"`
	OK           bool   `json:"ok"`
	Excerpt      string `json:"excerpt,omitempty"`
	ArtifactName string `json:"artifact_name,omitempty"`
	Bytes        int    `json:"bytes,omitempty"`
	Error        string `json:"error,omitempty"`
}

func (webFetchHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		URLs []string `json:"urls"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if len(args.URLs) == 0 {
		return "urls is required", true, nil
	}
	args.URLs = dedupeStrings(args.URLs) // avoid two goroutines writing the same artifact
	if len(args.URLs) > maxFetchURLs {
		args.URLs = args.URLs[:maxFetchURLs]
	}
	if env.Pool == nil {
		return missingService("pool")
	}

	resolver := tools_web.NewResolver(env.Pool, env.ProjectID)
	fp, err := resolver.FetchProvider()
	if err != nil {
		return err.Error(), true, nil
	}
	excerptBytes := resolver.ExcerptBytes()

	rows := make([]fetchRow, len(args.URLs))
	sem := make(chan struct{}, fetchConcurrency)
	var wg sync.WaitGroup
	for i, u := range args.URLs {
		rows[i] = fetchRow{URL: u, OK: false, Error: "not processed"} // overwritten by the goroutine
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// SSRF guard before any provider call.
			if verr := tools_web.ValidateFetchURLSyntax(u); verr != nil {
				rows[i] = fetchRow{URL: u, OK: false, Error: "blocked_by_policy: " + verr.Error()}
				return
			}
			page := fp.Fetch(ctx, u)
			if !page.OK {
				rows[i] = fetchRow{URL: u, OK: false, Error: page.Err}
				return
			}
			body, truncated := clip(page.Markdown, excerptBytes)
			row := fetchRow{URL: u, OK: true, Bytes: page.Bytes}
			switch {
			case !truncated:
				row.Excerpt = body
			case env.ArtifactSvc == nil:
				row.Excerpt = body + truncMarker("full content unavailable: no artifact store")
			default:
				sum := sha1.Sum([]byte(u))
				name := fmt.Sprintf("websrc_%x.md", sum[:8])
				a, aerr := env.ArtifactSvc.AddFromAgent(ctx, env.SessionID, env.ProjectID, env.WorkflowInstanceID, name, "text/markdown", []byte(page.Markdown))
				if aerr == nil {
					row.ArtifactName = a.Name
					row.Excerpt = body + truncMarker("full content in artifact "+a.Name)
				} else {
					row.Excerpt = body + truncMarker("full-content artifact failed: "+aerr.Error())
				}
			}
			rows[i] = row
		}(i, u)
	}
	wg.Wait()

	out, err := json.Marshal(map[string]any{"pages": rows})
	if err != nil {
		return err.Error(), true, nil
	}
	return string(out), false, nil
}

// clip returns the first n bytes of s on a rune boundary and whether it was
// truncated. The truncation marker is added by the caller (which knows whether
// the full content was successfully offloaded to an artifact). n<=0 or s<=n
// returns s untouched with truncated=false.
func clip(s string, n int) (string, bool) {
	if n <= 0 || len(s) <= n {
		return s, false
	}
	cut := n
	for cut > 0 && !utf8RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

func truncMarker(note string) string { return "\n…(truncated; " + note + ")" }

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }

// dedupeStrings returns in with duplicate values removed, preserving order.
func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
