package refinery

import (
	"context"
	"sync"
	"time"

	"be/internal/foldfmt"
	"be/internal/handoff"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// maxFoldDeltaChars caps the message-delta text handed to a single
// autonomous fold call, via foldfmt.JoinTail's tail-keep idiom.
const maxFoldDeltaChars = 8000

// maxFoldRowChars head-caps each delta row before JoinTail, so a single
// multi-KB tool row can never alone evict every earlier turn from the
// tail-keep budget.
const maxFoldRowChars = 2000

// autonomousSession pairs a sidecar (trigger channel + goroutine, reused
// as-is from the console path) with the slot identity and per-session fold
// progress an autonomous fold needs that a console fold does not: the
// relaunch-stable (workflow_instance_id, node_id) digest key, the shared
// per-slot mutex, and the seq boundary up to which agent_messages rows have
// already been folded.
type autonomousSession struct {
	sc                 *sidecar
	workflowInstanceID string
	nodeID             string
	taskAnchor         string
	slotMu             *sync.Mutex

	mu          sync.Mutex
	nextFoldSeq int
}

// newAutonomousSession builds an autonomousSession and acquires its shared
// per-slot mutex reference (released by StopSession after the final fold).
func (m *Manager) newAutonomousSession(workflowInstanceID, nodeID, taskAnchor string) *autonomousSession {
	return &autonomousSession{
		workflowInstanceID: workflowInstanceID,
		nodeID:             nodeID,
		taskAnchor:         taskAnchor,
		slotMu:             m.acquireSlotLock(workflowInstanceID, nodeID),
	}
}

// StartSession registers an autonomous fold sidecar for a spawner-managed
// session. No-op when refinery_autonomous_enabled is off, or when a sidecar
// is already live for sessionID (idempotent, mirrors console Start).
func (m *Manager) StartSession(sessionID, projectID, workflowInstanceID, nodeID string) {
	if !m.autonomousEnabled() {
		return
	}
	m.autonomousMu.Lock()
	defer m.autonomousMu.Unlock()
	if _, ok := m.autonomous[sessionID]; ok {
		return
	}
	anchor, err := repo.NewAgentSessionRepo(m.pool, m.clock).GetPrompt(sessionID)
	if err != nil {
		logger.Error(context.Background(), "refinery: read task anchor failed", "session_id", sessionID, "error", err)
		anchor = ""
	}
	as := m.newAutonomousSession(workflowInstanceID, nodeID, anchor)
	foldFn := func(ctx context.Context, sid, pid string, _ []string) {
		m.foldAutonomous(ctx, as, sid, pid)
	}
	as.sc = newSidecar(sessionID, projectID, m.clock, foldFn)
	m.autonomous[sessionID] = as
	as.sc.run()
}

// StopSession tears an autonomous sidecar down: best-effort final fold of any
// remaining message delta, then stop the goroutine and drop the entry.
// Idempotent — unknown session id is a no-op.
func (m *Manager) StopSession(sessionID string) {
	m.autonomousMu.Lock()
	as, ok := m.autonomous[sessionID]
	if ok {
		delete(m.autonomous, sessionID)
	}
	m.autonomousMu.Unlock()
	if !ok {
		return
	}
	as.sc.stop()
	ctx, cancel := context.WithTimeout(context.Background(), stopFoldTimeout)
	m.foldAutonomous(ctx, as, sessionID, as.sc.projectID)
	cancel()
	// Release after the final fold so a concurrently-started relaunch
	// sibling still shares this slot's mutex identity through it.
	m.releaseSlotLock(as.workflowInstanceID, as.nodeID)
}

// stopFoldTimeout bounds StopSession's synchronous final fold so a stalled
// provider.Run call can never hang the spawner's monitor/finalize/cancel
// paths that call StopSession inline.
const stopFoldTimeout = 20 * time.Second

// FoldNow runs one bounded synchronous fold for a still-live autonomous
// session, leaving the sidecar running and the slot lock held (unlike
// StopSession, which is the teardown path).
//
// The spawner's kill-time save path calls this before deciding whether to
// spawn a context-saver agent. Folds are debounced >=30s, so a session that
// burns from refinery_fold_start_context_pct down to the relaunch threshold
// inside one debounce window dies with no digest even though the refinery is
// healthy — the fold it had scheduled lands seconds later and is never read.
// Forcing it here costs one bounded local-model call instead of a full
// context-saver spawn. No-op for an unknown session; foldGateOpen and the
// empty-delta check inside foldAutonomous still apply, and the shared slot
// mutex serializes this against the sidecar's own debounced fold.
func (m *Manager) FoldNow(sessionID string) {
	m.autonomousMu.Lock()
	as, ok := m.autonomous[sessionID]
	m.autonomousMu.Unlock()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), stopFoldTimeout)
	defer cancel()
	m.foldAutonomous(ctx, as, sessionID, as.sc.projectID)
}

// autonomousEnabled reads refinery_autonomous_enabled with default-ON
// semantics (absence/anything but the literal "false" means ON) — the
// inverse of console refinery_enabled's default-off "val=='true'" read.
func (m *Manager) autonomousEnabled() bool {
	val, _ := service.NewGlobalSettingsService(m.pool, m.clock).Get("refinery_autonomous_enabled")
	return val != "false"
}

// foldGateOpen reports whether context_left has dropped to or below the
// configured refinery_fold_start_context_pct threshold (default 45), i.e.
// whether this session is due for an autonomous fold. Re-reads both the
// threshold and context_left per call so an admin edit takes effect on live
// sessions. A read error fails closed (returns false): the kill-time
// context-saver already covers an unexpected early death, so the worst case
// of skipping here is a slightly less rich handoff, never data loss.
func (m *Manager) foldGateOpen(ctx context.Context, sessionID string) bool {
	threshold, err := service.NewGlobalSettingsService(m.pool, m.clock).GetRefineryFoldStartContextPct()
	if err != nil {
		logger.Error(ctx, "refinery: read fold-start threshold failed", "session_id", sessionID, "error", err)
		return false
	}
	left, err := repo.NewAgentSessionRepo(m.pool, m.clock).GetContextLeft(sessionID)
	if err != nil {
		logger.Error(ctx, "refinery: read context_left failed", "session_id", sessionID, "error", err)
		return false
	}
	if left > threshold {
		logger.Info(ctx, "refinery: autonomous fold skipped, context above fold-start threshold",
			"session_id", sessionID, "context_left", left, "threshold", threshold,
			"note", "kill-time context-saver still covers an unexpected early death")
		return false
	}
	return true
}

// foldAutonomous folds the agent_messages delta since as.nextFoldSeq into
// the (workflow_instance_id, node_id) slot digest. No-op when there is no new
// message. Best-effort: errors are logged, never propagated — callers are a
// sidecar goroutine and StopSession, neither of which blocks on fold outcome.
func (m *Manager) foldAutonomous(ctx context.Context, as *autonomousSession, sessionID, projectID string) {
	if !m.foldGateOpen(ctx, sessionID) {
		return
	}
	messageRepo := repo.NewAgentMessageRepo(m.pool, m.clock)
	as.mu.Lock()
	fromSeq := as.nextFoldSeq
	as.mu.Unlock()
	delta, err := messageRepo.GetBySessionCategorizedFromSeq(sessionID, fromSeq)
	if err != nil {
		logger.Error(ctx, "refinery: autonomous read messages failed", "session_id", sessionID, "error", err)
		return
	}
	if len(delta) == 0 {
		return
	}

	// Serialize the GetSlot..UpsertSlot read-modify-write across sessions
	// sharing this (workflow_instance_id, node_id) slot — e.g. the old
	// session's StopSession final fold racing the new session's first fold
	// during a relaunch — so one fold never silently clobbers the other.
	as.slotMu.Lock()
	defer as.slotMu.Unlock()

	prevDigest, err := m.digestRepo.GetSlot(as.workflowInstanceID, as.nodeID)
	if err != nil {
		logger.Error(ctx, "refinery: autonomous read previous digest failed",
			"workflow_instance_id", as.workflowInstanceID, "node_id", as.nodeID, "error", err)
		return
	}
	prevContent := ""
	if prevDigest != nil {
		prevContent = prevDigest.Content
	}

	lines := make([]string, 0, len(delta))
	for _, msg := range delta {
		if msg.Category == model.MsgCategorySystemNotice {
			continue
		}
		lines = append(lines, "["+msg.Category+"] "+msg.Content)
	}
	if len(lines) == 0 {
		return
	}
	lines = foldfmt.CapRows(lines, maxFoldRowChars)
	userText := buildFoldUserText(as.taskAnchor, prevContent, []string{foldfmt.JoinTail(lines, maxFoldDeltaChars)}, nil)
	foldSeq := 0
	if prevDigest != nil {
		foldSeq = prevDigest.FoldCount
	}
	target := foldTarget{sessionID: sessionID, workflowInstanceID: as.workflowInstanceID, nodeID: as.nodeID, foldSeq: foldSeq}
	content, usage, ok := m.runFoldCore(ctx, target, projectID, userText)
	if !ok {
		return
	}

	foldCount, err := m.digestRepo.UpsertSlot(as.workflowInstanceID, as.nodeID, projectID, content)
	if err != nil {
		logger.Error(ctx, "refinery: autonomous upsert digest failed",
			"workflow_instance_id", as.workflowInstanceID, "node_id", as.nodeID, "error", err)
		return
	}

	m.broadcastHandoffDigest(ctx, sessionID, projectID, as.workflowInstanceID, as.nodeID)

	as.mu.Lock()
	as.nextFoldSeq = delta[len(delta)-1].Seq + 1
	as.mu.Unlock()

	if m.costAttributor != nil {
		m.costAttributor(sessionID, usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens, usage.CacheCreationTokens)
	}

	logger.Info(ctx, "refinery autonomous fold complete",
		"session_id", sessionID, "workflow_instance_id", as.workflowInstanceID, "node_id", as.nodeID,
		"fold_count", foldCount, "delta_messages", len(delta),
		"input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens,
		"digest_bytes", len(content))
}

// broadcastHandoffDigest re-reads the just-upserted slot row and, when a
// broadcaster is wired, emits a project-scoped EventAgentHandoffDigest
// carrying session_id (FE matches on it, mirroring context_ledger) so the
// UI can pick up the new digest without polling. Debounce is inherited from
// the fold cadence — one broadcast per successful UpsertSlot is already
// server-side rate-limited by the sidecar's >=30s trigger coalescing, so no
// extra per-slot timer is needed here. Best-effort: a re-read failure is
// logged, never propagated.
func (m *Manager) broadcastHandoffDigest(ctx context.Context, sessionID, projectID, workflowInstanceID, nodeID string) {
	if m.broadcaster == nil {
		return
	}
	digest, err := m.digestRepo.GetSlot(workflowInstanceID, nodeID)
	if err != nil || digest == nil {
		logger.Error(ctx, "refinery: autonomous re-read digest for broadcast failed",
			"workflow_instance_id", workflowInstanceID, "node_id", nodeID, "error", err)
		return
	}
	content := digest.Content
	if composed := handoff.Compose(ctx, m.pool, m.clock, sessionID, digest.Content); composed != "" {
		content = composed
	}
	data := map[string]interface{}{
		"session_id":           sessionID,
		"workflow_instance_id": workflowInstanceID,
		"node_id":              nodeID,
		"version":              digest.Version,
		"fold_count":           digest.FoldCount,
		"updated_at":           digest.UpdatedAt.Format(time.RFC3339Nano),
		"content":              content,
	}
	m.broadcaster(ws.NewEvent(ws.EventAgentHandoffDigest, projectID, "", "", data))
}
