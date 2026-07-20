package spawner

// Stop cancels runCtx, waits for any in-flight turn goroutine to finish, and
// closes Events exactly once. `stopping` is closed BEFORE waiting so emit can
// always unwind (codexEngine's Stop, console_engine_codex.go:185-201): a
// caller that stops mid-drain must never deadlock a turn blocked on a full
// events buffer.
func (e *apiConsoleEngine) Stop() {
	e.stoppingOnce.Do(func() { close(e.stopping) })
	e.mu.Lock()
	e.stopped = true
	cancel := e.cancel
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	e.turnWG.Wait()
	e.stopOnce.Do(func() { close(e.events) })
}

// emit delivers one EngineEvent to the buffered Events channel, abandoning
// the send once Stop has begun so a non-draining consumer can never wedge the
// turn goroutine.
func (e *apiConsoleEngine) emit(ev EngineEvent) {
	select {
	case e.events <- ev:
	case <-e.stopping:
	}
}
