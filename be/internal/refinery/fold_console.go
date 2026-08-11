package refinery

import (
	"context"
	"sync"

	"be/internal/foldfmt"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// maxConsoleFoldDeltaChars caps the message-delta text handed to a single
// console fold call, via foldfmt.JoinTail's tail-keep idiom. Larger than the
// autonomous maxFoldDeltaChars (8000) because a console turn carries both
// the user prompt AND the full delegation-findings body in one row.
const maxConsoleFoldDeltaChars = 12000

// consoleSession pairs a sidecar with the per-session fold progress a
// console fold needs: the seq boundary up to which agent_messages rows have
// already been folded. Built and closed over at Start time, mirroring
// autonomousSession — never added to the shared sidecar struct.
type consoleSession struct {
	sc *sidecar

	mu          sync.Mutex
	nextFoldSeq int
}

// consoleFoldGateOpen reports whether the chat session's context_left has
// dropped to or below its fold-start threshold — resolved per model
// tier/model with the refinery_console_fold_start_context_pct generic
// fallback (default 75, i.e. >=25% used; cheap-tier models default to 0 =
// never fold) — a barely-used chat has nothing worth paying a fold for.
// Mirrors the autonomous foldGateOpen: re-read per call so an admin edit
// takes effect live; a read error fails closed.
func (m *Manager) consoleFoldGateOpen(ctx context.Context, sessionID string) bool {
	left, modelID, err := repo.NewAgentSessionRepo(m.pool, m.clock).GetContextLeftAndModel(sessionID)
	if err != nil {
		logger.Error(ctx, "refinery: read console context_left failed", "session_id", sessionID, "error", err)
		return false
	}
	threshold, err := service.NewGlobalSettingsService(m.pool, m.clock).GetRefineryFoldStartPctForModel(modelID, true)
	if err != nil {
		logger.Error(ctx, "refinery: read console fold-start threshold failed", "session_id", sessionID, "error", err)
		return false
	}
	if threshold <= 0 {
		logger.Info(ctx, "refinery: console fold disabled for model tier",
			"session_id", sessionID, "model_id", modelID)
		return false
	}
	if left > threshold {
		logger.Info(ctx, "refinery: console fold skipped, context above fold-start threshold",
			"session_id", sessionID, "context_left", left, "threshold", threshold)
		return false
	}
	return true
}

// foldConsole folds the agent_messages delta since cs.nextFoldSeq, combined
// with the buffered WS event lines, into sessionID's digest. No-op when both
// the message delta and events are empty. Best-effort: errors are logged,
// never propagated — the caller is a sidecar goroutine that does not block
// on fold outcome.
func (m *Manager) foldConsole(ctx context.Context, cs *consoleSession, sessionID, projectID string, events []string) {
	if !m.consoleFoldGateOpen(ctx, sessionID) {
		// nextFoldSeq stays put, so the conversation delta is still covered
		// by the first gate-open fold; only the already-drained WS event
		// lines are dropped — early-session event metadata a barely-used
		// chat's digest doesn't need.
		return
	}
	messageRepo := repo.NewAgentMessageRepo(m.pool, m.clock)
	cs.mu.Lock()
	fromSeq := cs.nextFoldSeq
	cs.mu.Unlock()
	delta, err := messageRepo.GetBySessionCategorizedFromSeq(sessionID, fromSeq)
	if err != nil {
		logger.Error(ctx, "refinery: console read messages failed", "session_id", sessionID, "error", err)
		return
	}

	lines := make([]string, 0, len(delta))
	for _, msg := range delta {
		if msg.Category == model.MsgCategorySystemNotice {
			continue
		}
		lines = append(lines, "["+msg.Category+"] "+msg.Content)
	}
	if len(lines) == 0 && len(events) == 0 {
		return
	}

	var conversation []string
	if len(lines) > 0 {
		lines = foldfmt.CapRows(lines, maxFoldRowChars)
		conversation = []string{foldfmt.JoinTail(lines, maxConsoleFoldDeltaChars)}
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

	foldSeq := 0
	if prevDigest != nil {
		foldSeq = prevDigest.FoldCount
	}
	target := foldTarget{sessionID: sessionID, foldSeq: foldSeq}
	content, usage, _, ok := m.runFoldCore(ctx, target, projectID, buildFoldUserText("", prevContent, conversation, events))
	if !ok {
		return
	}

	foldCount, err := m.digestRepo.Upsert(sessionID, projectID, content)
	if err != nil {
		logger.Error(ctx, "refinery: upsert digest failed", "session_id", sessionID, "error", err)
		return
	}

	if len(delta) > 0 {
		cs.mu.Lock()
		cs.nextFoldSeq = delta[len(delta)-1].Seq + 1
		cs.mu.Unlock()
	}

	logger.Info(ctx, "refinery fold complete",
		"session_id", sessionID, "fold_count", foldCount, "delta_messages", len(delta),
		"input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens,
		"digest_bytes", len(content))
}
