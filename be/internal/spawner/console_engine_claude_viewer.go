package spawner

// ConsolePTYTarget is the raw-terminal attach surface a console engine may
// expose (claude only — codex has no PTY, api has no process). The engine
// keeps sole ownership of PTY reads (two concurrent readers on one ptmx
// steal bytes from each other) and forwards output to at most one attached
// viewer; input/resize go through the engine for the same reason.
type ConsolePTYTarget interface {
	// AttachViewer registers sink as the single live viewer, replacing any
	// prior one, and returns its detach func. Bytes arrive from the engine's
	// ferry goroutine — the sink must not block.
	AttachViewer(sink func([]byte)) (detach func())
	ViewerWrite(data []byte) error
	ViewerResize(rows, cols uint16) error
}

var _ ConsolePTYTarget = (*claudeEngine)(nil)

// consoleViewer wraps the sink so detach can use pointer identity: a stale
// detach (viewer already replaced by a newer attach) must be a no-op.
type consoleViewer struct{ sink func([]byte) }

func (e *claudeEngine) AttachViewer(sink func([]byte)) func() {
	v := &consoleViewer{sink: sink}
	e.viewerMu.Lock()
	e.viewer = v
	e.viewerMu.Unlock()
	return func() {
		e.viewerMu.Lock()
		if e.viewer == v {
			e.viewer = nil
		}
		e.viewerMu.Unlock()
	}
}

// forwardToViewer hands one PTY output chunk to the attached viewer (copying
// — the ferry reuses its buffer). No viewer, no work: the ferry's
// read-and-drop stays free for the common unattached case.
func (e *claudeEngine) forwardToViewer(data []byte) {
	e.viewerMu.Lock()
	v := e.viewer
	e.viewerMu.Unlock()
	if v == nil {
		return
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	v.sink(cp)
}

func (e *claudeEngine) ViewerWrite(data []byte) error {
	e.mu.Lock()
	sess := e.ptySession
	e.mu.Unlock()
	if sess == nil {
		return ErrEngineStopped
	}
	_, err := sess.Write(data)
	return err
}

func (e *claudeEngine) ViewerResize(rows, cols uint16) error {
	e.mu.Lock()
	sess := e.ptySession
	e.mu.Unlock()
	if sess == nil {
		return ErrEngineStopped
	}
	return sess.Resize(rows, cols)
}

// NotifyUserPrompt acknowledges and persists the engine's own SendUserTurn,
// or reports a human-typed attached-terminal prompt for the socket handler to
// persist. An own echo is matched at most once so a human later typing the
// identical text still persists.
func (e *claudeEngine) NotifyUserPrompt(prompt string) (own bool) {
	e.mu.Lock()
	if e.pendingEcho == "" || prompt != e.pendingEcho {
		e.mu.Unlock()
		return false
	}
	ack := e.promptAck
	category := e.pendingEchoCategory
	sessionID := e.spec.SessionID
	e.pendingEcho = ""
	e.pendingEchoCategory = ""
	e.promptAck = nil
	e.turnAcknowledged = true
	e.mu.Unlock()

	// A nil ack is the zero-timeout test seam: SendUserTurn already persisted
	// and emitted synchronously. Production reaches this branch with ack set.
	if ack != nil {
		if e.sink != nil {
			emitMessage(sessionID, prompt, category, e.sink)
		}
		e.emit(EngineEvent{Type: EventTurnStarted, SessionID: sessionID})
		close(ack)
	}
	return true
}

// ferry reads and drops PTY output until the session closes — claude's
// heartbeat and turn boundaries come from hooks, not PTY bytes. A read error
// while Stop has NOT been requested means the CLI process died on its own:
// no Stop hook will ever arrive, so a turn in flight would stay pinned
// forever. Emit an EventError so the consumer can end the turn and surface
// the death. (It does not close Events: tailLoop is still emitting on its own
// goroutine; Stop owns that close.)
func (e *claudeEngine) ferry(sess ptySessionIface) {
	defer e.ferryOnce.Do(func() { close(e.ferryDone) })
	buf := make([]byte, 4096)
	var carry []byte
	for {
		n, err := sess.Read(buf)
		if n > 0 {
			if mode, ok := scanBracketedPasteMode(carry, buf[:n]); ok {
				e.mu.Lock()
				e.bracketedPaste = mode
				e.mu.Unlock()
			}
			carry = carryTail(buf[:n])
			e.forwardToViewer(buf[:n])
		}
		if err != nil {
			select {
			case <-e.stopping:
			default:
				e.emit(EngineEvent{
					Type:      EventError,
					SessionID: e.sessionID(),
					Text:      "claude console session ended unexpectedly",
					IsError:   true,
				})
			}
			return
		}
	}
}
