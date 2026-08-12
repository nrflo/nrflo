package spawner

import "context"

// compile-time assertions: claudeEngine must satisfy both ConsoleEngine
// (the public engine contract) and consoleTarget (the hub-facing surface).
var (
	_ ConsoleEngine = (*claudeEngine)(nil)
	_ consoleTarget = (*claudeEngine)(nil)
)

// turnEndDrainRetries bounds drainFinalText's re-flush loop (each retry
// waits one tailInterval, so the default worst case is 4×750ms).
const turnEndDrainRetries = 4

// NotifySessionReady marks the TUI ready, unblocking any waitUntilReady call.
// Idempotent. Called by ConsoleHub.ConsoleSessionReady on the SessionStart hook.
func (e *claudeEngine) NotifySessionReady() {
	e.readyOnce.Do(func() { close(e.readyCh) })
}

// NotifyTurnEnd flushes the transcript tail (draining for the turn's final
// assistant text — see drainFinalText), clears turnActive, emits
// EventTurnCompleted, and signals the Sink's OnTurnComplete. Called by
// ConsoleHub.ConsoleTurnEnd on the Stop hook.
func (e *claudeEngine) NotifyTurnEnd() {
	e.flushTranscript()
	e.drainFinalText()
	e.mu.Lock()
	e.turnActive = false
	sessionID := e.spec.SessionID
	e.mu.Unlock()
	e.emit(EngineEvent{Type: EventTurnCompleted, SessionID: sessionID})
	if e.sink != nil {
		e.sink.OnTurnComplete(sessionID)
	}
}

// drainFinalText re-flushes the transcript until an assistant text block has
// surfaced for the current turn, bounded by turnEndDrainRetries. The Stop
// hook can outrun the CLI's append of the turn's final assistant text, and
// EventTurnCompleted consumers may stop this engine immediately (console
// rotation) — killing the tail ticker that would otherwise pick the line up
// on its next tick and losing the reply entirely.
func (e *claudeEngine) drainFinalText() {
	for i := 0; i < turnEndDrainRetries && !e.textSeenThisTurn(); i++ {
		e.pause(context.Background(), e.tailInterval)
		e.flushTranscript()
	}
}

// textSeenThisTurn reports whether an assistant text block was emitted since
// the last SendUserTurn armed the turn.
func (e *claudeEngine) textSeenThisTurn() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.turnTextSeen
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
