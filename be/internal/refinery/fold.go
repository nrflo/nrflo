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
	def, err := m.systemAgentSvc.GetForBackend("refinery", "api")
	if err != nil {
		logger.Warn(ctx, "refinery: _refinery def not found, skipping fold", "session_id", sessionID, "error", err)
		return
	}

	modelRow, err := m.modelSvc.Get(def.Model)
	if err != nil {
		logger.Error(ctx, "refinery: resolve model row failed", "session_id", sessionID, "model", def.Model, "error", err)
		return
	}
	if modelRow.APIModel == "" {
		logger.Error(ctx, "refinery: model row has no api_model", "session_id", sessionID, "model", def.Model)
		return
	}

	prov, err := buildProvider(ctx, m.pool, m.clock, modelRow.Provider, projectID)
	if err != nil {
		logger.Error(ctx, "refinery: build provider failed", "session_id", sessionID, "provider", modelRow.Provider, "error", err)
		return
	}

	prevDigest, err := m.digestRepo.Get(sessionID)
	if err != nil {
		logger.Error(ctx, "refinery: read previous digest failed", "session_id", sessionID, "error", err)
		return
	}
	prevContent := ""
	if prevDigest != nil {
		prevContent = prevDigest.Content
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
				Text: buildFoldUserText(prevContent, events),
			}},
		}},
		MaxTokens: maxTokens,
		Model:     modelRow.APIModel,
	}

	resp, err := prov.Run(ctx, req, noopSink{})
	if err != nil {
		logger.Error(ctx, "refinery: provider run failed", "session_id", sessionID, "error", err)
		return
	}

	content := capBytes(extractText(resp.Content), maxDigestBytes)
	foldCount, err := m.digestRepo.Upsert(sessionID, projectID, content)
	if err != nil {
		logger.Error(ctx, "refinery: upsert digest failed", "session_id", sessionID, "error", err)
		return
	}

	logger.Info(ctx, "refinery fold complete",
		"session_id", sessionID, "fold_count", foldCount,
		"input_tokens", resp.Usage.InputTokens, "output_tokens", resp.Usage.OutputTokens,
		"digest_bytes", len(content))
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
