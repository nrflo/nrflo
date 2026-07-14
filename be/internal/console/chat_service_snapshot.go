package console

import "be/internal/spawner"

// ChatSnapshot is a live chat session's in-memory state: engine identity plus
// the turn/pending-approval state machine. Returned by Snapshot for the
// GET .../{sid} detail route so a page reload can restore an in-flight
// approval card and the turn spinner instead of losing them.
type ChatSnapshot struct {
	Engine           string
	ModelID          string
	WorkDir          string
	Turn             string
	PendingApprovals []*spawner.ApprovalRequest
}

// Snapshot returns sid's live in-memory state. ok=false means no live engine
// is registered for sid — a hard-killed server leaves user_interactive rows
// with no engine, so callers must not assume a DB row implies a snapshot.
func (s *ChatService) Snapshot(sid string) (ChatSnapshot, bool) {
	sess, ok := s.get(sid)
	if !ok {
		return ChatSnapshot{}, false
	}
	turn, pending := sess.snapshot()
	return ChatSnapshot{
		Engine:           sess.EngineName(),
		ModelID:          sess.ModelID(),
		WorkDir:          sess.WorkDir(),
		Turn:             string(turn),
		PendingApprovals: pending,
	}, true
}

// Live reports whether sid has a live engine registered with this service.
func (s *ChatService) Live(sid string) bool {
	_, ok := s.get(sid)
	return ok
}

// StopAll stops every live chat engine. Called during graceful shutdown,
// before repo.FailAllRunning marks running/user_interactive rows failed —
// otherwise an engine process (PTY child, app-server child) would outlive
// the server.
func (s *ChatService) StopAll() {
	s.mu.Lock()
	sessions := make([]*chatSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessions = make(map[string]*chatSession)
	s.mu.Unlock()

	for _, sess := range sessions {
		sess.engine.Stop()
	}
}
