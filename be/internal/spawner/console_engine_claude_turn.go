package spawner

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SendUserTurn waits for the TUI to be ready, writes the body + a submit CR,
// and waits for Claude's UserPromptSubmit hook to acknowledge it. The hook
// persists the user row and emits turn_started before returning to Claude, so
// assistant transcript rows cannot land first and an unaccepted write cannot
// leave a false user row or a permanently active turn.
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
	e.turnAcknowledged = false
	e.turnTextSeen = false
	ack := make(chan struct{})
	e.promptAck = ack
	e.pendingEcho = text
	e.pendingEchoCategory = turn.MessageCategory()
	e.mu.Unlock()

	e.waitUntilReady(ctx)

	for attempt := 1; attempt <= maxPromptSubmits; attempt++ {
		if attempt > 1 {
			select {
			case <-ack:
				return nil
			default:
			}
			// Close any palette and clear text left by a swallowed submit before
			// retyping; UserPromptSubmit has not fired, so no model turn exists.
			if _, err := sess.Write([]byte{0x1b, 0x15}); err != nil {
				e.resetUnackedTurn(ack)
				return fmt.Errorf("console engine: reset unaccepted turn: %w", err)
			}
			e.pause(ctx, e.submitDelay)
		}
		if err := e.writeTurnAttempt(ctx, sess, text); err != nil {
			e.resetUnackedTurn(ack)
			return err
		}

		// Tests collapse the ack wait to zero; preserve their synchronous
		// engine seam while production always confirms through the hook.
		if e.turnAckTimeout <= 0 {
			e.mu.Lock()
			if e.promptAck == ack {
				e.promptAck = nil
				e.turnAcknowledged = true
			}
			e.mu.Unlock()
			if e.sink != nil {
				emitMessage(spec.SessionID, text, turn.MessageCategory(), e.sink)
			}
			e.emit(EngineEvent{Type: EventTurnStarted, SessionID: spec.SessionID})
			return nil
		}
		if e.waitTurnAck(ctx, ack) {
			return nil
		}
		select {
		case <-ctx.Done():
			e.resetUnackedTurn(ack)
			return fmt.Errorf("console engine: wait for claude acknowledgement: %w", ctx.Err())
		case <-e.stopping:
			e.resetUnackedTurn(ack)
			return ErrEngineStopped
		default:
		}
	}

	select {
	case <-ack:
		return nil
	default:
	}
	e.resetUnackedTurn(ack)
	return fmt.Errorf("console engine: claude did not acknowledge turn after %d submits", maxPromptSubmits)
}

func (e *claudeEngine) writeTurnAttempt(ctx context.Context, sess ptySessionIface, text string) error {
	if err := e.writeTurnText(sess, text); err != nil {
		return fmt.Errorf("console engine: write turn: %w", err)
	}
	if strings.HasPrefix(text, "/") {
		if _, err := sess.Write([]byte(" ")); err != nil {
			return fmt.Errorf("console engine: write turn: %w", err)
		}
	}
	e.pause(ctx, e.submitDelay)
	if _, err := sess.Write([]byte("\r")); err != nil {
		return fmt.Errorf("console engine: submit turn: %w", err)
	}
	return nil
}

func (e *claudeEngine) waitTurnAck(ctx context.Context, ack <-chan struct{}) bool {
	t := time.NewTimer(e.turnAckTimeout)
	defer t.Stop()
	select {
	case <-ack:
		return true
	case <-t.C:
		return false
	case <-ctx.Done():
		return false
	case <-e.stopping:
		return false
	}
}

func (e *claudeEngine) resetUnackedTurn(ack chan struct{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.promptAck != ack {
		return
	}
	e.promptAck = nil
	e.pendingEcho = ""
	e.pendingEchoCategory = ""
	e.turnActive = false
	e.turnAcknowledged = false
}

// SteerUserTurn types text into the BUSY TUI: claude natively queues input
// submitted mid-turn and steers it to the model at the next tool boundary
// (or auto-submits it as the next turn when the turn ends first — either
// way it is delivered, so this method persists the user row itself).
// turnActive is re-checked after the submit-delay pause: if the turn ended
// in that window, submitting would start a turn behind the server's back,
// so the typed line is cleared (Ctrl+U) and ErrNoActiveTurn tells the
// caller to send a normal turn instead.
func (e *claudeEngine) SteerUserTurn(ctx context.Context, turn UserTurn) error {
	text := turn.Text
	e.mu.Lock()
	if !e.turnActive || !e.turnAcknowledged {
		e.mu.Unlock()
		return ErrNoActiveTurn
	}
	sess, spec := e.ptySession, e.spec
	if sess == nil {
		e.mu.Unlock()
		return fmt.Errorf("console engine: not started")
	}
	e.mu.Unlock()

	if err := e.writeTurnText(sess, text); err != nil {
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
		emitMessage(spec.SessionID, text, turn.MessageCategory(), e.sink)
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

// writeTurnText writes turn text into the TUI, wrapped in bracketed-paste
// markers while the TUI has DECSET ?2004 on (it does, for its whole input
// loop). Unmarked, a multi-KB body goes through the per-keystroke path and the
// TUI keeps only its last ~100 bytes — the write reports full length and the
// submit CR is still accepted, so the truncation is silent. The submit CR and
// the slash-command dismissal space stay outside the markers: pasted, they
// would be inserted as text instead of acted on.
func (e *claudeEngine) writeTurnText(sess ptySessionIface, text string) error {
	e.mu.Lock()
	// Claude 2.1.247 consumes a bracketed-pasted slash command in its command
	// palette without submitting it. Slash turns are short and must follow the
	// native per-keystroke path; ordinary/large turns retain atomic paste mode.
	paste := e.bracketedPaste && !strings.HasPrefix(text, "/")
	e.mu.Unlock()
	if paste {
		text = bracketedPasteSet2004 + stripPasteEnd(text) + bracketedPasteEnd
	}
	_, err := sess.Write([]byte(text))
	return err
}
