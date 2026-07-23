package refinery

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/foldfmt"
	"be/internal/logger"
	"be/internal/service"
	"be/internal/spawner/apirun/provider"
)

// maxDigestBytes caps the folded digest written to refinery_digests.content.
const maxDigestBytes = 4096

// defaultFoldMaxTokens is used when the _refinery def has no api_max_tokens set.
const defaultFoldMaxTokens = 1500

// buildProvider is a test seam (same package-var idiom as
// spawner.newConsoleAPIProvider) so tests can inject a fake provider without
// real credentials or a network call.
var buildProvider = service.BuildAPIProvider

// fold loads the `_refinery` api-mode def, resolves its model row, reads the
// previous digest, runs a single direct provider.Run (no tools), caps the
// result to maxDigestBytes, and upserts it. Best-effort: errors are logged,
// never propagated (a sidecar's caller does not block on fold outcome).
func (m *Manager) fold(ctx context.Context, sessionID, projectID string, events []string) {
	prevDigest, err := m.digestRepo.Get(sessionID)
	if err != nil {
		logger.Error(ctx, "refinery: read previous digest failed", "session_id", sessionID, "error", err)
		return
	}
	prevContent := ""
	if prevDigest != nil {
		prevContent = prevDigest.Content
	}

	foldSeq := 0
	if prevDigest != nil {
		foldSeq = prevDigest.FoldCount
	}
	target := foldTarget{sessionID: sessionID, foldSeq: foldSeq}
	content, usage, ok := m.runFoldCore(ctx, target, projectID, buildFoldUserText("", prevContent, events))
	if !ok {
		return
	}

	foldCount, err := m.digestRepo.Upsert(sessionID, projectID, content)
	if err != nil {
		logger.Error(ctx, "refinery: upsert digest failed", "session_id", sessionID, "error", err)
		return
	}

	logger.Info(ctx, "refinery fold complete",
		"session_id", sessionID, "fold_count", foldCount,
		"input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens,
		"digest_bytes", len(content))
}

// runFoldCore is the provider-run core shared by the console fold above and
// the autonomous fold (session_sidecar.go): load the `_refinery` api-mode
// def, resolve its model row, run one direct provider.Run (no tools) over
// userText, and cap the result to maxDigestBytes. target identifies the fold
// slot for log lines and the refinery_runs footprint row, written on every
// return path (recordFoldRun, best-effort). Best-effort: errors are logged
// and reported via ok=false, never propagated.
func (m *Manager) runFoldCore(ctx context.Context, target foldTarget, projectID, userText string) (content string, usage provider.Usage, ok bool) {
	logKey := target.logKey()

	def, err := m.systemAgentSvc.GetForBackend("refinery", "api")
	if err != nil {
		logger.Warn(ctx, "refinery: _refinery def not found, skipping fold", "key", logKey, "error", err)
		m.recordFoldRun(ctx, target, projectID, "", "", provider.Usage{}, "failed", err.Error())
		return "", provider.Usage{}, false
	}

	chain, err := m.systemAgentSvc.ResolveAgentChain(def)
	if err != nil {
		logger.Error(ctx, "refinery: resolve agent chain failed", "key", logKey, "error", err)
		m.recordFoldRun(ctx, target, projectID, "", "", provider.Usage{}, "failed", err.Error())
		return "", provider.Usage{}, false
	}
	primaryModel := chain[0].ModelID

	modelRow, err := m.modelSvc.Get(primaryModel)
	if err != nil {
		logger.Error(ctx, "refinery: resolve model row failed", "key", logKey, "model", primaryModel, "error", err)
		m.recordFoldRun(ctx, target, projectID, "", primaryModel, provider.Usage{}, "failed", err.Error())
		return "", provider.Usage{}, false
	}
	if modelRow.APIModel == "" {
		logger.Error(ctx, "refinery: model row has no api_model", "key", logKey, "model", primaryModel)
		m.recordFoldRun(ctx, target, projectID, modelRow.Provider, primaryModel, provider.Usage{}, "failed", "model row has no api_model")
		return "", provider.Usage{}, false
	}

	prov, err := buildProvider(ctx, m.pool, m.clock, modelRow.Provider, projectID)
	if err != nil {
		logger.Error(ctx, "refinery: build provider failed", "key", logKey, "provider", modelRow.Provider, "error", err)
		m.recordFoldRun(ctx, target, projectID, modelRow.Provider, modelRow.APIModel, provider.Usage{}, "failed", err.Error())
		return "", provider.Usage{}, false
	}

	maxTokens := defaultFoldMaxTokens
	if def.APIMaxTokens != nil && *def.APIMaxTokens > 0 {
		maxTokens = *def.APIMaxTokens
	}

	req := provider.Request{
		System: def.Prompt,
		Messages: []provider.Message{{
			Role: "user",
			Content: []provider.ContentBlock{{
				Type: "text",
				Text: userText,
			}},
		}},
		MaxTokens: maxTokens,
		Model:     modelRow.APIModel,
	}

	resp, err := prov.Run(ctx, req, noopSink{})
	if err != nil {
		logger.Error(ctx, "refinery: provider run failed", "key", logKey, "error", err)
		m.recordFoldRun(ctx, target, projectID, modelRow.Provider, modelRow.APIModel, provider.Usage{}, "failed", err.Error())
		return "", provider.Usage{}, false
	}

	text := extractText(resp.Content)
	if strings.TrimSpace(text) == "" || isDegenerateStopReason(resp.StopReason) {
		logger.Warn(ctx, "refinery: rejecting degenerate fold output", "key", logKey, "stop_reason", resp.StopReason, "text_bytes", len(text))
		m.recordFoldRun(ctx, target, projectID, modelRow.Provider, modelRow.APIModel, resp.Usage, "failed", "degenerate fold output")
		return "", provider.Usage{}, false
	}

	content = foldfmt.CapBytes(text, maxDigestBytes)
	m.recordFoldRun(ctx, target, projectID, modelRow.Provider, modelRow.APIModel, resp.Usage, "ok", "")
	return content, resp.Usage, true
}

// isDegenerateStopReason reports whether sr indicates a truncated (max_tokens)
// or refused response that should not be folded into the digest.
func isDegenerateStopReason(sr string) bool {
	return sr == "max_tokens" || sr == "refusal"
}

// buildFoldUserText assembles the fold prompt's user text. taskAnchor, when
// non-empty (autonomous fold only), is prepended as an immutable ## Task
// section supplied verbatim each fold — the model must anchor the digest to
// it but never summarize/drop/contradict it. Console fold passes "".
func buildFoldUserText(taskAnchor, prevDigest string, events []string) string {
	var b strings.Builder
	if taskAnchor != "" {
		b.WriteString("## Task\n\n")
		b.WriteString(taskAnchor)
		b.WriteString("\n\n")
	}
	b.WriteString("## Previous Digest\n\n")
	if prevDigest == "" {
		b.WriteString("_none yet_")
	} else {
		b.WriteString(prevDigest)
	}
	b.WriteString("\n\n## New Events\n\n")
	b.WriteString(strings.Join(events, "\n"))
	return b.String()
}

func extractText(blocks []provider.ContentBlock) string {
	var b strings.Builder
	for _, block := range blocks {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

// noopSink discards every streaming callback: fold is a single blocking
// Run() call with no need to observe deltas.
type noopSink struct{}

func (noopSink) OnTextDelta(string)                    {}
func (noopSink) OnThinkingDelta(string)                {}
func (noopSink) OnToolUseStart(string, string)         {}
func (noopSink) OnToolUseInputDelta(string, string)    {}
func (noopSink) OnToolUseStop(string, json.RawMessage) {}
func (noopSink) OnUsage(provider.Usage)                {}

var _ provider.EventSink = noopSink{}
