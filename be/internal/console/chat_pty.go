package console

import (
	"errors"

	"be/internal/spawner"
)

// ErrChatNoPTY marks a console chat whose engine exposes no raw terminal
// (codex/api) — only the claude engine implements spawner.ConsolePTYTarget.
var ErrChatNoPTY = errors.New("console_chat_no_pty")

// AttachPTY registers sink as the chat engine's raw-terminal viewer and
// returns the engine's PTY target (input/resize) plus the viewer's detach.
// Detaching leaves the engine and its PTY untouched — this is a viewer, not
// an owner; Close remains the only teardown path.
func (s *ChatService) AttachPTY(sid string, sink func([]byte)) (spawner.ConsolePTYTarget, func(), error) {
	sess, ok := s.get(sid)
	if !ok {
		return nil, nil, ErrChatSessionNotFound
	}
	target, ok := sess.getEngine().(spawner.ConsolePTYTarget)
	if !ok {
		return nil, nil, ErrChatNoPTY
	}
	detach := target.AttachViewer(sink)
	return target, detach, nil
}
