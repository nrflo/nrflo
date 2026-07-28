package spawner

import (
	"time"
)

// RequestRestart sends a restart signal for the given session ID.
// Non-blocking: if a restart is already pending, this is a no-op.
func (s *Spawner) RequestRestart(sessionID string) {
	select {
	case s.restartCh <- sessionID:
	default:
	}
}

// RequestTakeControl sends a take-control signal for the given session ID and
// registers a readiness channel that closes once monitorAll has finished
// killing the agent and flipped the session to user_interactive (or rejected
// the take-control request, or attached as a viewer for cli_interactive).
// Callers can wait for readiness via WaitForTakeControlReady. Non-blocking on
// the channel send: if a take-control is already pending, the existing
// readiness entry is reused.
func (s *Spawner) RequestTakeControl(sessionID string) {
	s.takeControlReadiesMu.Lock()
	if _, exists := s.takeControlReadies[sessionID]; !exists {
		s.takeControlReadies[sessionID] = make(chan struct{})
	}
	s.takeControlReadiesMu.Unlock()

	select {
	case s.takeControlCh <- sessionID:
	default:
	}
}

// WaitForTakeControlReady blocks until the take-control flow for the given
// session ID has completed its synchronous setup (kill + status flip to
// user_interactive, or reject, or viewer-attach), or until the timeout
// elapses. Returns true if the ready signal fired, false on timeout or when
// no readiness was registered for this session.
func (s *Spawner) WaitForTakeControlReady(sessionID string, timeout time.Duration) bool {
	s.takeControlReadiesMu.Lock()
	ch, ok := s.takeControlReadies[sessionID]
	s.takeControlReadiesMu.Unlock()
	if !ok {
		return false
	}
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

// signalTakeControlReady closes the readiness channel for the given session
// (idempotent) and removes it from the map. Called by monitorAll once the
// take-control flow has reached a state where a PTY connection can succeed.
func (s *Spawner) signalTakeControlReady(sessionID string) {
	s.takeControlReadiesMu.Lock()
	ch, ok := s.takeControlReadies[sessionID]
	if ok {
		delete(s.takeControlReadies, sessionID)
	}
	s.takeControlReadiesMu.Unlock()
	if ok {
		close(ch)
	}
}

// RequestTerminalSignal kills the matching agent so monitorAll exits the
// natural-exit wait and handleCompletion reads the DB result already written
// by the socket handler. Routes the signal to the specific monitorAll
// goroutine that owns this sessionID, so concurrent monitorAlls cannot
// steal each other's signals. No-op if the session is not registered
// (already finished or never started). Non-blocking on the channel send.
func (s *Spawner) RequestTerminalSignal(sessionID, result string) {
	s.terminalSignalsMu.Lock()
	ch, ok := s.terminalSignals[sessionID]
	s.terminalSignalsMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- terminalSignal{SessionID: sessionID, Result: result}:
	default:
	}
}

// registerTerminalSignal binds sessionID to ch in the registry so
// RequestTerminalSignal(sessionID, ...) routes to ch. Used by monitorAll
// at start and on continuation-relaunch to track new session IDs.
func (s *Spawner) registerTerminalSignal(sessionID string, ch chan terminalSignal) {
	s.terminalSignalsMu.Lock()
	s.terminalSignals[sessionID] = ch
	s.terminalSignalsMu.Unlock()
	// Fire callback outside the mutex to avoid lock-order inversion
	// (callback acquires orchestrator's mu; terminalSignalsMu must not be held).
	if s.config.OnSessionRegister != nil {
		s.config.OnSessionRegister(sessionID, s)
	}
}

// unregisterTerminalSignal removes sessionID from the registry. Subsequent
// RequestTerminalSignal calls for this sessionID become no-ops.
func (s *Spawner) unregisterTerminalSignal(sessionID string) {
	s.terminalSignalsMu.Lock()
	delete(s.terminalSignals, sessionID)
	s.terminalSignalsMu.Unlock()
	if s.config.OnSessionUnregister != nil {
		s.config.OnSessionUnregister(sessionID)
	}
}

// childSessionHooks composes this spawner's own session-registration callbacks
// with a child-local capture, for the one-off child Spawners built by
// context_save.go / consult_run.go / delegate.go.
//
// Forwarding the parent callbacks is not optional bookkeeping: they are what
// puts a session into the orchestrator's sessionID→*Spawner index, and that
// index is what serves the MCP bridge's tools/list and routes `record-event`
// heartbeats to the proc. A one-off child that omits them gets an empty tool
// list and no heartbeat, so a perfectly healthy agent trips stall detection at
// stall_start_timeout_sec and is killed on a loop. The child runs under the
// caller's workflow instance, so the parent's closure indexes it correctly.
//
// captureSID is the child's own use for the id (nil when it has none). It
// receives the registering spawner too: registrations bubble up through every
// composed hook in the chain, so a grandchild's session (e.g. an executor
// worker's own delegate fanout) reaches this capture as well — a caller
// tracking its direct worker must compare the pointer against the child
// Spawner it built, or a grandchild registration overwrites the captured id.
func (s *Spawner) childSessionHooks(captureSID func(string, *Spawner)) (func(string, *Spawner), func(string)) {
	parentRegister := s.config.OnSessionRegister
	parentUnregister := s.config.OnSessionUnregister
	register := func(sessionID string, child *Spawner) {
		if captureSID != nil {
			captureSID(sessionID, child)
		}
		if parentRegister != nil {
			parentRegister(sessionID, child)
		}
	}
	return register, parentUnregister
}

// MarkSessionReady closes the matching proc's sessionStartCh — the canonical
// TUI-ready signal from Claude's SessionStart hook. Idempotent. Called by the
// socket handler when SessionStart arrives.
func (s *Spawner) MarkSessionReady(sessionID string) {
	proc := s.lookupSessionProc(sessionID)
	if proc == nil || proc.sessionStartCh == nil {
		return
	}
	proc.sessionStartOnce.Do(func() {
		s.logAgent(proc, "ready signal: SessionStart hook")
		close(proc.sessionStartCh)
	})
}

// CompleteInteractive signals that the interactive session has ended,
// unblocking the spawner's monitorAll wait.
func (s *Spawner) CompleteInteractive(sessionID string) {
	s.mu.Lock()
	ch, ok := s.interactiveWaits[sessionID]
	if ok {
		delete(s.interactiveWaits, sessionID)
	}
	s.mu.Unlock()
	if ok {
		select {
		case <-ch:
			// already closed
		default:
			close(ch)
		}
		// Note: OnSessionUnregister is NOT fired here. The orchestrator holds
		// o.mu when it calls this method (iterating rs.spawners), so firing the
		// callback would deadlock. Pre-step spawners remain in rs.spawners as
		// harmless orphans until the runState is GC'd; take-control spawners are
		// cleaned up by unregisterTerminalSignal when monitorAll unblocks.
	}
}

// KillInteractive signals that the interactive session should be treated as a failure,
// unblocking the spawner's monitorAll wait and marking the session as FAIL.
func (s *Spawner) KillInteractive(sessionID string) {
	s.mu.Lock()
	s.killedInteractive[sessionID] = struct{}{}
	ch, ok := s.interactiveWaits[sessionID]
	if ok {
		delete(s.interactiveWaits, sessionID)
	}
	s.mu.Unlock()
	if ok {
		select {
		case <-ch:
			// already closed
		default:
			close(ch)
		}
	}
}

// RegisterInteractiveWait creates and returns a channel that blocks until
// CompleteInteractive is called for the given session ID. Used by the
// orchestrator to wait on interactive/plan PTY sessions before entering
// the layer execution loop. Fires OnSessionRegister so the orchestrator's
// sessionID→*Spawner index includes this spawner for the duration of the wait.
func (s *Spawner) RegisterInteractiveWait(sessionID string) <-chan struct{} {
	ch := make(chan struct{})
	s.mu.Lock()
	s.interactiveWaits[sessionID] = ch
	s.mu.Unlock()
	// Fire outside the mutex (same discipline as registerTerminalSignal) to
	// avoid lock-order inversion with the orchestrator's mu.
	if s.config.OnSessionRegister != nil {
		s.config.OnSessionRegister(sessionID, s)
	}
	return ch
}

// Close is a no-op retained for API compatibility (e.g. orchestrator defer).
func (s *Spawner) Close() {}

// registerSessionProc tracks a live proc by sessionID so RecordUserInput can
// route user keystrokes through the normal TrackMessage pipeline.
func (s *Spawner) registerSessionProc(sessionID string, proc *processInfo) {
	s.sessionProcsMu.Lock()
	s.sessionProcs[sessionID] = proc
	s.sessionProcsMu.Unlock()
}

// lookupSessionProc returns the live proc for sessionID, or nil if unknown.
func (s *Spawner) lookupSessionProc(sessionID string) *processInfo {
	s.sessionProcsMu.Lock()
	defer s.sessionProcsMu.Unlock()
	return s.sessionProcs[sessionID]
}

// unregisterSessionProcs removes completed procs from the session proc map
// and kills any background shells they left running (native fs bash
// run_in_background) so none outlive the session.
func (s *Spawner) unregisterSessionProcs(procs []*processInfo) {
	if len(procs) == 0 {
		return
	}
	s.sessionProcsMu.Lock()
	for _, proc := range procs {
		delete(s.sessionProcs, proc.sessionID)
	}
	s.sessionProcsMu.Unlock()
	for _, proc := range procs {
		proc.apiToolEnv.FS.KillAll()
	}
}
