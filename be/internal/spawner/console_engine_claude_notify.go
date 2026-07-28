package spawner

// NotifySessionReady marks the TUI ready, unblocking any waitUntilReady call.
// Idempotent. Called by ConsoleHub.ConsoleSessionReady on the SessionStart hook.
func (e *claudeEngine) NotifySessionReady() {
	e.readyOnce.Do(func() { close(e.readyCh) })
}

// NotifyTurnEnd flushes the transcript tail, clears turnActive, emits
// EventTurnCompleted, and signals the Sink's OnTurnComplete. Called by
// ConsoleHub.ConsoleTurnEnd on the Stop hook.
func (e *claudeEngine) NotifyTurnEnd() {
	e.flushTranscript()
	e.mu.Lock()
	e.turnActive = false
	sessionID := e.spec.SessionID
	e.mu.Unlock()
	e.emit(EngineEvent{Type: EventTurnCompleted, SessionID: sessionID})
	if e.sink != nil {
		e.sink.OnTurnComplete(sessionID)
	}
}

// NotifyToolResult emits EventToolResult. Called by
// ConsoleHub.ConsoleToolResult on the PostToolUse/PostToolUseFailure hooks —
// the pairing "finished" signal to the EventToolInvoke that RequestApproval
// emits on every PreToolUse.
func (e *claudeEngine) NotifyToolResult(toolName string, isError bool) {
	e.mu.Lock()
	sessionID := e.spec.SessionID
	e.mu.Unlock()
	e.emit(EngineEvent{Type: EventToolResult, SessionID: sessionID, ToolName: toolName, IsError: isError})
}

// NotifyContextLeft emits EventTokenUsage. Called by
// ConsoleHub.ConsoleContextLeft on an agent.context_update (statusline).
func (e *claudeEngine) NotifyContextLeft(pct int) {
	e.mu.Lock()
	sessionID := e.spec.SessionID
	e.mu.Unlock()
	e.emit(EngineEvent{Type: EventTokenUsage, SessionID: sessionID, ContextLeftPct: pct})
}
