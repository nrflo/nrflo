package spawner

import (
	"context"

	"be/internal/logger"
)

// dispatchManualRestart finds the running proc matching a manual restart
// request (drained from restartCh by monitorAll) and initiates a context
// save for it. No-op if the session already finished or is already saving.
func (s *Spawner) dispatchManualRestart(ctx context.Context, running []*processInfo, req SpawnRequest, sessionID string) {
	for _, proc := range running {
		if proc.sessionID != sessionID || proc.lowContextSaving {
			continue
		}
		logger.Info(ctx, "manual restart requested", "session_id", sessionID)
		proc.lowContextSaving = true
		oldDoneCh := proc.doneCh
		newDoneCh := make(chan struct{})
		proc.doneCh = newDoneCh
		go s.initiateContextSave(ctx, proc, req, oldDoneCh, newDoneCh)
		return
	}
}

// RotateSignals implements apirun.StepSession: returns the calling session's
// current ledger token total and its resolved proactive-restart threshold,
// (0,0) when either is unknown — ShouldRotate treats a <=0 threshold as
// disabled, so an unknown session never rotates.
func (s *Spawner) RotateSignals(sessionID string) (contextTokens, thresholdTokens int) {
	summary, ok := globalLedgerStore.epochSummary(sessionID)
	if !ok {
		return 0, 0
	}
	proc := s.lookupSessionProc(sessionID)
	if proc == nil {
		return 0, 0
	}
	return summary.TotalTokens, proc.proactiveRestartThreshold
}

// NoteStepBoundary implements apirun.StepSession: stamps the task-boundary
// signal for sessionID at the ledger's current turn, the same signal a
// finding-recorded boundary gives the idle proactive-restart watcher.
func (s *Spawner) NoteStepBoundary(sessionID string) {
	turn, ok := globalLedgerStore.turnNow(sessionID)
	if !ok {
		return
	}
	globalRestartStore.noteBoundary(sessionID, turn, s.config.Clock.Now())
}

// RequestStepRotation implements apirun.StepSession: a non-blocking send of
// sessionID onto stepRotateCh, mirroring RequestRestart's shape. A full
// channel (unexpected — buffered 4) silently drops the request rather than
// blocking the tool-call goroutine.
func (s *Spawner) RequestStepRotation(sessionID string) {
	select {
	case s.stepRotateCh <- sessionID:
	default:
	}
}

// dispatchStepRotation is monitorAll's stepRotateCh drain: complete_step's
// OutcomeRotate leg has already committed the Advance transaction, so this
// mirrors checkProactiveRestart's tail (context_save_proactive.go) minus the
// idle/threshold gates — the rotate decision was already made by
// stepengine.ShouldRotate inside Advance. No-op for an unknown session, a
// proc already saving, or a backend that doesn't track context.
func (s *Spawner) dispatchStepRotation(ctx context.Context, running []*processInfo, req SpawnRequest, sessionID string) {
	var proc *processInfo
	for _, p := range running {
		if p.sessionID == sessionID {
			proc = p
			break
		}
	}
	if proc == nil || proc.lowContextSaving {
		return
	}
	if proc.backend == nil || !proc.backend.TracksContext() {
		return
	}

	tokensBefore := 0
	if summary, ok := globalLedgerStore.epochSummary(sessionID); ok {
		tokensBefore = summary.TotalTokens
	}

	NoteProactiveRestart(sessionID, s.config.Clock)
	proc.proactiveRotationPending = true
	proc.proactiveTokensBefore = tokensBefore
	proc.lowContextSaving = true

	oldDoneCh := proc.doneCh
	newDoneCh := make(chan struct{})
	proc.doneCh = newDoneCh

	logger.Info(ctx, "step rotation triggered", "session_id", sessionID, "tokens_before", tokensBefore)
	go s.initiateContextSaveProactive(ctx, proc, req, oldDoneCh, newDoneCh)
}
