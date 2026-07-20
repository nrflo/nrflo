package spawner

import (
	"context"
	"syscall"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
)

// checkProactiveRestart is monitorAll's watcher-triggered counterpart to the
// low-context kill check: at a task boundary (a finding recorded, within the
// policy's boundary window) while proc is idle — never mid-tool-chain — and
// over its resolved proactive_restart_threshold_tokens, it fires the same
// kill->save->relaunch chain the emergency low-context path uses, but marks
// the relaunch as a rotation (proc.proactiveRotationPending) so
// relaunchForContinuation resets the continuation counter instead of
// incrementing it. Never fires while a save is already in flight, and only
// for backends the context ledger tracks (claude PTY, codex app-server).
func (s *Spawner) checkProactiveRestart(ctx context.Context, proc *processInfo, req SpawnRequest) {
	if proc.lowContextSaving || proc.proactiveRestartThreshold <= 0 {
		return
	}
	if proc.backend == nil || !proc.backend.TracksContext() {
		return
	}
	if !idleWindowElapsed(s.config.Clock, proc) {
		return
	}

	summary, ok := globalLedgerStore.epochSummary(proc.sessionID)
	if !ok {
		return
	}
	currentTurn, _ := globalLedgerStore.turnNow(proc.sessionID)

	fire, tokensBefore := ProactiveRestartDecision(s.pool(), s.config.Clock, proc.sessionID,
		summary.TotalTokens, proc.proactiveRestartThreshold, currentTurn, true,
		lastPlanItemInFlight(s.pool(), proc.workflowInstanceID))
	if !fire {
		return
	}

	NoteProactiveRestart(proc.sessionID, s.config.Clock)
	proc.proactiveRotationPending = true
	proc.proactiveTokensBefore = tokensBefore
	proc.lowContextSaving = true

	oldDoneCh := proc.doneCh
	newDoneCh := make(chan struct{})
	proc.doneCh = newDoneCh

	logger.Info(ctx, "proactive restart triggered", "session_id", proc.sessionID,
		"tokens_before", tokensBefore, "threshold", proc.proactiveRestartThreshold)
	go s.initiateContextSaveProactive(ctx, proc, req, oldDoneCh, newDoneCh)
}

// idleWindowElapsed reports whether proc has been silent past its idle
// window, using the same lastMessageTime/hasReceivedMessage computation
// checkIdleNudge (idle_nudge.go) uses to gate a nudge — the mid-tool-chain
// guard: any recent activity (tool call, message) resets lastMessageTime, so
// an elapsed idle window is the same "safe to interrupt" signal a nudge
// already relies on.
func idleWindowElapsed(clk clock.Clock, proc *processInfo) bool {
	proc.messagesMutex.Lock()
	sinceLastMsg := clk.Now().Sub(proc.lastMessageTime)
	hasMsg := proc.hasReceivedMessage
	proc.messagesMutex.Unlock()

	idleWindow := proc.idleAfterMessageTimeout
	if !hasMsg {
		idleWindow = proc.idleStartTimeout
	}
	if idleWindow <= 0 {
		return false
	}
	return sinceLastMsg > idleWindow
}

// lastPlanItemInFlight reports whether the highest-layer materialized plan
// node for a workflow instance currently has a running (not yet ended)
// agent session — the proactive-restart safety rail that defers rotation
// until the run's last item finishes. Non-plan-driven workflows (no
// materialized nodes) report false: there is no "last item" concept to gate
// on.
func lastPlanItemInFlight(pool *db.Pool, workflowInstanceID string) bool {
	if pool == nil || workflowInstanceID == "" {
		return false
	}
	var count int
	err := pool.QueryRow(`
		SELECT COUNT(*) FROM agent_sessions
		WHERE workflow_instance_id = ? AND ended_at IS NULL AND node_id = (
			SELECT node_id FROM workflow_instance_nodes
			WHERE instance_id = ? ORDER BY layer DESC LIMIT 1
		)`, workflowInstanceID, workflowInstanceID).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// applyProactiveRotationCarry sets newProc's ancestor/continuation-counter
// fields for a relaunch: a proactive rotation resets restartCount instead of
// incrementing it (it is not a failure/low-context retry, so it should not
// count against defaultMaxContinuations) and bumps its own
// proactiveRestartCount, logging tokens-before (captured at trigger time) /
// tokens-after (the new session's ledger total). A normal continuation just
// increments restartCount as before.
func applyProactiveRotationCarry(ctx context.Context, oldProc, newProc *processInfo, ancestorID string) {
	newProc.ancestorSessionID = ancestorID
	if !oldProc.proactiveRotationPending {
		newProc.restartCount = oldProc.restartCount + 1
		return
	}
	newProc.restartCount = 0
	newProc.proactiveRestartCount = oldProc.proactiveRestartCount + 1
	tokensAfter := 0
	if summary, ok := globalLedgerStore.epochSummary(newProc.sessionID); ok {
		tokensAfter = summary.TotalTokens
	}
	logger.Info(ctx, "proactive restart rotation complete", "old_session", oldProc.sessionID, "new_session", newProc.sessionID,
		"tokens_before", oldProc.proactiveTokensBefore, "tokens_after", tokensAfter, "proactive_restart_count", newProc.proactiveRestartCount)
}

// initiateContextSaveProactive reuses the standard kill->flush->save chain
// (initiateContextSave) for a watcher-triggered proactive restart. A fresh
// autonomous refinery slot digest for this session skips the context-saver
// spawn via the shared contextSaveViaAgent (see digest_freshness.go);
// otherwise it saves via the context-saver system agent, exactly as the
// emergency low-context path does.
func (s *Spawner) initiateContextSaveProactive(ctx context.Context, proc *processInfo, req SpawnRequest, processDoneCh, completeCh chan struct{}) {
	defer close(completeCh)

	logger.Info(ctx, "proactive restart: context save initiated", "session_id", proc.sessionID, "tokens_before", proc.proactiveTokensBefore)

	proc.backend.Kill(ctx, proc, syscall.SIGTERM)
	select {
	case <-processDoneCh:
	case <-time.After(killGracePeriod):
		proc.backend.Kill(ctx, proc, syscall.SIGKILL)
		<-processDoneCh
	}

	s.saveMessages(proc)

	s.contextSaveViaAgent(ctx, proc, req)

	logger.Info(ctx, "proactive restart: context save complete", "session_id", proc.sessionID,
		"final_status", proc.finalStatus)
}
