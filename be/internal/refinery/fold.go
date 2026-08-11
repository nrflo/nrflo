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

// hasAPICreds is the matching seam for the static credential pre-check the
// chain walk uses to skip doomed api entries (fold_chain.go).
var hasAPICreds = service.HasAPICredentials

// runFoldCore is the provider-run core shared by the console fold
// (fold_console.go) and the autonomous fold (session_sidecar.go): load the
// `_refinery` api-mode def, resolve its model fallback chain, and walk the
// chain (fold_chain.go) until one entry lands or the chain is exhausted.
// target identifies the fold slot for log lines and the refinery_runs
// footprint rows, written on every attempt (recordFoldRun, best-effort).
// execMode is the execution mode of the entry that actually landed (empty
// when ok=false), so a cli_interactive landing can be excluded from cost
// double-attribution (session_sidecar.go). Best-effort: errors are logged
// and reported via ok=false, never propagated.
func (m *Manager) runFoldCore(ctx context.Context, target foldTarget, projectID, userText string) (content string, usage provider.Usage, execMode string, ok bool) {
	logKey := target.logKey()

	def, err := m.systemAgentSvc.GetForBackend("refinery", "api")
	if err != nil {
		logger.Warn(ctx, "refinery: _refinery def not found, skipping fold", "key", logKey, "error", err)
		m.recordFoldRun(ctx, target, projectID, "", "", provider.Usage{}, "failed", err.Error(), 0, "", "")
		return "", provider.Usage{}, "", false
	}

	chain, err := m.systemAgentSvc.ResolveAgentChain(def)
	if err != nil {
		logger.Error(ctx, "refinery: resolve agent chain failed", "key", logKey, "error", err)
		m.recordFoldRun(ctx, target, projectID, "", "", provider.Usage{}, "failed", err.Error(), 0, "", "")
		return "", provider.Usage{}, "", false
	}

	return m.walkFoldChain(ctx, target, projectID, userText, def, chain)
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
// conversation is the categorized message delta (user turns, assistant
// replies, consumed delegation findings) that makes the digest's subject;
// events is the compact WS event-metadata line batch, rendered as a
// secondary ## New Events section that is omitted entirely when empty (the
// autonomous fold has none — it reads its delta straight from
// agent_messages and passes nil).
func buildFoldUserText(taskAnchor, prevDigest string, conversation, events []string) string {
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
	b.WriteString("\n\n## Conversation\n\n")
	b.WriteString(strings.Join(conversation, "\n"))
	if len(events) > 0 {
		b.WriteString("\n\n## New Events\n\n")
		b.WriteString(strings.Join(events, "\n"))
	}
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
