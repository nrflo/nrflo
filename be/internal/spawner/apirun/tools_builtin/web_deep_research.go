package tools_builtin

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

const deepResearchSummaryCap = 4000

// deepResearchHeartbeatInterval is how often the caller's stall timer is bumped
// during a blocking run. A var (not const) so tests can shrink it.
var deepResearchHeartbeatInterval = 30 * time.Second

// webDeepResearchHandler implements web_deep_research: run the deep-research
// workflow as a synchronous sub-workflow and return a cited summary.
type webDeepResearchHandler struct{}

func (webDeepResearchHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "web_deep_research",
		Description: "Run a deep, multi-source, fact-checked web-research workflow on a question and return a cited summary. Blocks until research completes (can take minutes); the full report is saved as an artifact.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{"question":{"type":"string","description":"The research question (be specific)"}},
"required":["question"],
"additionalProperties":false
}`),
	}
}

func (webDeepResearchHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if strings.TrimSpace(args.Question) == "" {
		return "question is required", true, nil
	}
	// Recursion guard (defense in depth; spawn-time stripping is primary).
	if strings.EqualFold(env.WorkflowName, service.DeepResearchWorkflow) {
		return "web_deep_research cannot be called from within the deep-research workflow", true, nil
	}
	if env.DeepResearch == nil {
		return missingService("deep_research")
	}

	// Keep the caller's stall timer alive during the long blocking run.
	if env.Heartbeat != nil {
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			t := time.NewTicker(deepResearchHeartbeatInterval)
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					env.Heartbeat()
				}
			}
		}()
	}

	report, err := env.DeepResearch.RunDeepResearch(ctx, env.ProjectID, args.Question)
	if err != nil {
		return "deep research failed: " + err.Error(), true, nil
	}

	artifactName := ""
	if env.ArtifactSvc != nil {
		sum := sha1.Sum([]byte(args.Question))
		name := fmt.Sprintf("deep_research_%x.json", sum[:8])
		if a, aerr := env.ArtifactSvc.AddFromAgent(ctx, env.SessionID, env.ProjectID, env.WorkflowInstanceID, name, "application/json", report); aerr == nil {
			artifactName = a.Name
		}
	}
	return summarizeReport(report, artifactName), false, nil
}

// summarizeReport renders a compact markdown answer from the report JSON,
// bounded to keep the calling agent's context lean. Full JSON is in the artifact.
func summarizeReport(report json.RawMessage, artifactName string) string {
	var rep struct {
		Summary  string `json:"summary"`
		Findings []struct {
			Claim      string `json:"claim"`
			Confidence string `json:"confidence"`
		} `json:"findings"`
		Caveats       string   `json:"caveats"`
		OpenQuestions []string `json:"openQuestions"`
	}
	if err := json.Unmarshal(report, &rep); err != nil || rep.Summary == "" {
		body, _ := clip(string(report), deepResearchSummaryCap)
		return appendArtifactNote(body, artifactName)
	}
	var b strings.Builder
	b.WriteString("## Summary\n")
	b.WriteString(rep.Summary)
	if len(rep.Findings) > 0 {
		b.WriteString("\n\n## Key findings\n")
		for _, f := range rep.Findings {
			conf := f.Confidence
			if conf == "" {
				conf = "?"
			}
			b.WriteString("- [" + conf + "] " + f.Claim + "\n")
		}
	}
	if rep.Caveats != "" {
		b.WriteString("\n## Caveats\n" + rep.Caveats + "\n")
	}
	if len(rep.OpenQuestions) > 0 {
		b.WriteString("\n## Open questions\n")
		for _, q := range rep.OpenQuestions {
			b.WriteString("- " + q + "\n")
		}
	}
	body, _ := clip(b.String(), deepResearchSummaryCap)
	return appendArtifactNote(body, artifactName)
}

func appendArtifactNote(body, artifactName string) string {
	if artifactName != "" {
		return body + "\n\n(full report in artifact " + artifactName + ")"
	}
	return body
}
