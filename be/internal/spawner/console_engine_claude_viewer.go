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

// NotifyUserPrompt reports whether a UserPromptSubmit hook echo is the
// engine's own SendUserTurn (suppress persisting — SendUserTurn already
// wrote the user_input row) or a human-typed prompt from an attached
// terminal (persist it — nothing else records it). Matched at most once per
// SendUserTurn so a human later typing the identical text still persists.
func (e *claudeEngine) NotifyUserPrompt(prompt string) (own bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pendingEcho != "" && prompt == e.pendingEcho {
		e.pendingEcho = ""
		return true
	}
	return false
}
