package refinery

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
)

// maxFoldDeltaChars caps the message-delta text handed to a single
// autonomous fold call — mirrors formatMessagesForSave's tail-keep idiom
// (spawner/context_save_format.go) without importing spawner.
const maxFoldDeltaChars = 8000

// autonomousSession pairs a sidecar (trigger channel + goroutine, reused
// as-is from the console path) with the slot identity and per-session fold
// progress an autonomous fold needs that a console fold does not: the
// relaunch-stable (workflow_instance_id, node_id) digest key and how many
// agent_messages rows have already been folded.
type autonomousSession struct {
	sc                 *sidecar
	workflowInstanceID string
	nodeID             string

	mu              sync.Mutex
	lastFoldedCount int
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
	as := &autonomousSession{workflowInstanceID: workflowInstanceID, nodeID: nodeID}
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
	defer cancel()
	m.foldAutonomous(ctx, as, sessionID, as.sc.projectID)
}

// stopFoldTimeout bounds StopSession's synchronous final fold so a stalled
// provider.Run call can never hang the spawner's monitor/finalize/cancel
// paths that call StopSession inline.
const stopFoldTimeout = 20 * time.Second

// autonomousEnabled reads refinery_autonomous_enabled with default-ON
// semantics (absence/anything but the literal "false" means ON) — the
// inverse of console refinery_enabled's default-off "val=='true'" read.
func (m *Manager) autonomousEnabled() bool {
	val, _ := service.NewGlobalSettingsService(m.pool, m.clock).Get("refinery_autonomous_enabled")
	return val != "false"
}

// foldAutonomous folds the agent_messages delta since as.lastFoldedCount into
// the (workflow_instance_id, node_id) slot digest. No-op when there is no new
// message. Best-effort: errors are logged, never propagated — callers are a
// sidecar goroutine and StopSession, neither of which blocks on fold outcome.
func (m *Manager) foldAutonomous(ctx context.Context, as *autonomousSession, sessionID, projectID string) {
	messageRepo := repo.NewAgentMessageRepo(m.pool, m.clock)
	messages, err := messageRepo.GetBySession(sessionID)
	if err != nil {
		logger.Error(ctx, "refinery: autonomous read messages failed", "session_id", sessionID, "error", err)
		return
	}

	as.mu.Lock()
	start := as.lastFoldedCount
	if start > len(messages) {
		start = len(messages)
	}
	delta := messages[start:]
	as.mu.Unlock()
	if len(delta) == 0 {
		return
	}

	// Serialize the GetSlot..UpsertSlot read-modify-write across sessions
	// sharing this (workflow_instance_id, node_id) slot — e.g. the old
	// session's StopSession final fold racing the new session's first fold
	// during a relaunch — so one fold never silently clobbers the other.
	slotMu := m.lockSlot(as.workflowInstanceID, as.nodeID)
	slotMu.Lock()
	defer slotMu.Unlock()

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

	userText := buildFoldUserText(prevContent, []string{formatMessageDelta(delta, maxFoldDeltaChars)})
	content, usage, ok := m.runFoldCore(ctx, sessionID, projectID, userText)
	if !ok {
		return
	}

	foldCount, err := m.digestRepo.UpsertSlot(as.workflowInstanceID, as.nodeID, projectID, content)
	if err != nil {
		logger.Error(ctx, "refinery: autonomous upsert digest failed",
			"workflow_instance_id", as.workflowInstanceID, "node_id", as.nodeID, "error", err)
		return
	}

	as.mu.Lock()
	as.lastFoldedCount = len(messages)
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

// formatMessageDelta joins delta messages with newlines, keeping only the
// tail that fits within maxChars — the same tail-keep + cap idiom as
// spawner's formatMessagesForSave (context_save_format.go), reimplemented
// locally so refinery never imports spawner.
func formatMessageDelta(messages []string, maxChars int) string {
	joined := strings.Join(messages, "\n")
	if len(joined) <= maxChars {
		return joined
	}

	var kept []string
	total := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgLen := len(messages[i])
		if total > 0 {
			msgLen++
		}
		if total+msgLen > maxChars {
			break
		}
		total += msgLen
		kept = append(kept, messages[i])
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}

	header := fmt.Sprintf("[truncated: showing last %d of %d messages]", len(kept), len(messages))
	return header + "\n" + strings.Join(kept, "\n")
}
