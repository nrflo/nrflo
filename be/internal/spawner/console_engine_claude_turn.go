package spawner

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SendUserTurn waits for the TUI to be ready, persists the user turn BEFORE
// writing it to the PTY (so the transcript tailer's assistant rows cannot
// land first), writes the body + a submit CR, and marks a turn active. Only
// the Stop hook (NotifyTurnEnd) clears turnActive, so a mid-turn turn is
// rejected with ErrTurnActive rather than typed into a busy TUI.
//
// turn.Skill is ignored: the raw turn.Text (e.g. "/name args") is typed into
// the TUI unchanged, letting claude's own slash-command handling resolve it —
// pass-through is this engine's side of the Rule 6 seam (codex/api expand
// instead, console_engine_codex.go/console_engine_api.go).
func (e *claudeEngine) SendUserTurn(ctx context.Context, turn UserTurn) error {
	text := turn.Text
	e.mu.Lock()
	if e.turnActive {
		e.mu.Unlock()
		return ErrTurnActive
	}
	sess, spec := e.ptySession, e.spec
	if sess == nil {
		e.mu.Unlock()
		return fmt.Errorf("console engine: not started")
	}
	e.turnActive = true
	e.mu.Unlock()

	e.waitUntilReady(ctx)

	if e.sink != nil {
		emitMessage(spec.SessionID, text, "user_input", e.sink)
	}
	// Arm the echo dedupe: the UserPromptSubmit hook for THIS text is our own
	// submission and must not be persisted twice (NotifyUserPrompt).
	e.mu.Lock()
	e.pendingEcho = text
	e.mu.Unlock()

	if _, err := sess.Write([]byte(text)); err != nil {
		e.mu.Lock()
		e.turnActive = false
		e.mu.Unlock()
		return fmt.Errorf("console engine: write turn: %w", err)
	}
	// A leading '/' opens the TUI's own command-palette autocomplete, which
	// would otherwise swallow the submit CR below as a palette-navigation
	// keystroke instead of submitting the line. A trailing space dismisses
	// the palette the same way a human typing a space after the command name
	// would, without changing what is actually submitted. This write is
	// isolated to claudeEngine: pendingEcho (armed below) stays the un-spaced
	// text so NotifyUserPrompt dedupe still matches the hook's echoed line.
	if strings.HasPrefix(text, "/") {
		if _, err := sess.Write([]byte(" ")); err != nil {
			e.mu.Lock()
			e.turnActive = false
			e.mu.Unlock()
			return fmt.Errorf("console engine: write turn: %w", err)
		}
	}
	// Gap before the submit CR: coalesced into a single PTY read, the TUI can
	// swallow the CR and the turn is typed but never sent (deliverPrompt takes
	// the same 150ms precaution).
	e.pause(ctx, e.submitDelay)
	if _, err := sess.Write([]byte("\r")); err != nil {
		e.mu.Lock()
		e.turnActive = false
		e.mu.Unlock()
		return fmt.Errorf("console engine: submit turn: %w", err)
	}

	e.emit(EngineEvent{Type: EventTurnStarted, SessionID: spec.SessionID})
	return nil
}

// SteerUserTurn types text into the BUSY TUI: claude natively queues input
// submitted mid-turn and steers it to the model at the next tool boundary
// (or auto-submits it as the next turn when the turn ends first — either
// way it is delivered, so this method persists the user row itself).
// turnActive is re-checked after the submit-delay pause: if the turn ended
// in that window, submitting would start a turn behind the server's back,
// so the typed line is cleared (Ctrl+U) and ErrNoActiveTurn tells the
// caller to send a normal turn instead.
func (e *claudeEngine) SteerUserTurn(ctx context.Context, text string) error {
	e.mu.Lock()
	if !e.turnActive {
		e.mu.Unlock()
		return ErrNoActiveTurn
	}
	sess, spec := e.ptySession, e.spec
	if sess == nil {
		e.mu.Unlock()
		return fmt.Errorf("console engine: not started")
	}
	e.mu.Unlock()

	if _, err := sess.Write([]byte(text)); err != nil {
		return fmt.Errorf("console engine: steer turn: %w", err)
	}
	if strings.HasPrefix(text, "/") {
		if _, err := sess.Write([]byte(" ")); err != nil {
			return fmt.Errorf("console engine: steer turn: %w", err)
		}
	}
	e.pause(ctx, e.submitDelay)

	e.mu.Lock()
	active := e.turnActive
	if active {
		// Arm the echo dedupe before the submit CR can trigger the
		// UserPromptSubmit hook for this text.
		e.pendingEcho = text
	}
	e.mu.Unlock()
	if !active {
		sess.Write([]byte{0x15}) //nolint:errcheck // Ctrl+U clears the typed line
		return ErrNoActiveTurn
	}
	if e.sink != nil {
		emitMessage(spec.SessionID, text, "user_input", e.sink)
	}
	if _, err := sess.Write([]byte("\r")); err != nil {
		return fmt.Errorf("console engine: steer submit: %w", err)
	}
	return nil
}

// InterruptTurn sends Ctrl+C to Claude's PTY. The Stop hook remains the owner
// of the idle transition and EventTurnCompleted emission.
func (e *claudeEngine) InterruptTurn(_ context.Context) error {
	e.mu.Lock()
	if !e.turnActive {
		e.mu.Unlock()
		return ErrNoActiveTurn
	}
	sess := e.ptySession
	e.mu.Unlock()
	if sess == nil {
		return ErrEngineStopped
	}
	if _, err := sess.Write([]byte{0x03}); err != nil {
		return fmt.Errorf("console engine: interrupt claude turn: %w", err)
	}
	return nil
}

// waitUntilReady blocks until SessionStart has signaled TUI-ready (or
// sessionStartTimeout elapses), then enforces the bootstrap floor — mirrors
// waitForReady's two-stage strategy (backend_interactive_helpers.go) without
// depending on a *processInfo. The floor exists to let the TUI finish its
// initial paint, so it is applied once: later turns land on a TUI that has
// already completed a turn and would only be delayed for nothing.
func (e *claudeEngine) waitUntilReady(ctx context.Context) {
	e.mu.Lock()
	bootstrapped := e.bootstrapped
	e.bootstrapped = true
	e.mu.Unlock()
	if bootstrapped {
		return
	}

	select {
	case <-e.readyCh:
	case <-time.After(e.sessionStartTimeout):
	case <-ctx.Done():
		return
	case <-e.stopping:
		return
	}
	e.pause(ctx, e.bootstrapFloor)
}

// pause sleeps for d, cutting the wait short on ctx cancellation or Stop. A
// non-positive d (the test default) returns immediately.
func (e *claudeEngine) pause(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	case <-e.stopping:
	}
}
