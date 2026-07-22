package refinery

import (
	"context"
	"encoding/json"
	"strings"

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

	content, usage, ok := m.runFoldCore(ctx, sessionID, projectID, buildFoldUserText(prevContent, events))
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
// userText, and cap the result to maxDigestBytes. logKey is the id (session
// id or workflow-instance:node slot) used in log lines. Best-effort: errors
// are logged and reported via ok=false, never propagated.
func (m *Manager) runFoldCore(ctx context.Context, logKey, projectID, userText string) (content string, usage provider.Usage, ok bool) {
	def, err := m.systemAgentSvc.GetForBackend("refinery", "api")
	if err != nil {
		logger.Warn(ctx, "refinery: _refinery def not found, skipping fold", "key", logKey, "error", err)
		return "", provider.Usage{}, false
	}

	chain, err := m.systemAgentSvc.ResolveAgentChain(def)
	if err != nil {
		logger.Error(ctx, "refinery: resolve agent chain failed", "key", logKey, "error", err)
		return "", provider.Usage{}, false
	}
	primaryModel := chain[0].ModelID

	modelRow, err := m.modelSvc.Get(primaryModel)
	if err != nil {
		logger.Error(ctx, "refinery: resolve model row failed", "key", logKey, "model", primaryModel, "error", err)
		return "", provider.Usage{}, false
	}
	if modelRow.APIModel == "" {
		logger.Error(ctx, "refinery: model row has no api_model", "key", logKey, "model", primaryModel)
		return "", provider.Usage{}, false
	}

	prov, err := buildProvider(ctx, m.pool, m.clock, modelRow.Provider, projectID)
	if err != nil {
		logger.Error(ctx, "refinery: build provider failed", "key", logKey, "provider", modelRow.Provider, "error", err)
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
		return "", provider.Usage{}, false
	}

	return capBytes(extractText(resp.Content), maxDigestBytes), resp.Usage, true
}

func buildFoldUserText(prevDigest string, events []string) string {
	var b strings.Builder
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

// capBytes truncates s to at most n bytes, backing off to a UTF-8 rune
// boundary so a multi-byte character is never split.
func capBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
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
