package spawner

import "context"

// emit delivers one EngineEvent to the buffered Events channel, matching the
// EventEmitter signature so it can be passed directly to
// dispatchAppServerEvent. Only called from within runLoop's own goroutine,
// strictly before it closes loopDone/Stop closes the channel, so the send is
// safe. It abandons the send once Stop has begun, so a non-draining consumer
// can never wedge the run loop (see Stop).
func (e *codexEngine) emit(ev EngineEvent) {
	select {
	case e.events <- ev:
	case <-e.stopping:
	}
}

// runLoop consumes notifications and server requests until ctx is cancelled
// or the app-server connection closes. No idle timer, no nudge, no restart
// cap, no rate-limit dance — see codexEngine's type doc comment
// (console_engine_codex.go).
func (e *codexEngine) runLoop(ctx context.Context) {
	// Registered first, so it runs last: every emit happens on this goroutine,
	// so once the loop is unwinding nothing can send on events again. Closing
	// here (not only in Stop) is what Events() promises — "closed when the run
	// loop exits" — and it is the only way a consumer learns the engine died on
	// its own (app-server EOF) rather than blocking on a channel forever.
	defer e.stopOnce.Do(func() { close(e.events) })
	defer e.loopOnce.Do(func() { close(e.loopDone) })
	// Once the loop is gone no turn/completed can ever arrive, so a turn left
	// in flight (connection dropped mid-turn) would otherwise pin turnActive
	// forever and reject every later SendUserTurn with ErrTurnActive.
	defer func() {
		e.mu.Lock()
		e.turnActive = false
		e.turnID = ""
		e.mu.Unlock()
	}()

	e.mu.Lock()
	client := e.client
	e.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return
		case <-client.closed:
			e.emit(EngineEvent{Type: EventError, SessionID: e.spec.SessionID, Text: "app-server connection closed", IsError: true})
			return
		case req := <-client.reqCh:
			e.onServerRequest(req)
		case n := <-client.notifyCh:
			if n.Method == "serverRequest/resolved" {
				e.onServerRequestResolved(n.Params)
				continue
			}
			sig := dispatchAppServerEvent(e.spec.SessionID, n, e.sink, e.spec.MaxContext, e.emit)
			e.mu.Lock()
			if sig.turnStarted {
				e.turnActive = true
			}
			if sig.turnCompleted {
				e.turnActive = false
				e.turnID = ""
			}
			e.mu.Unlock()
		}
	}
}
