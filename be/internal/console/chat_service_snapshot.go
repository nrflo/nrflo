package console

import (
	"be/internal/repo"
	"be/internal/spawner"
)

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
	SessionApprovals []string
	LiveItems        []ChatLiveItem
	Thinking         ChatLiveItem
	Yolo             bool
	QueuedPrompts    []string
	GitBranch        string
	GitAdded         int
	GitDeleted       int
	// RotateAtPct is the proactive-rotation ceiling as a percentage of the
	// context window (profile budget capped — see maybeRotate); 0 = disabled
	// or unknown window.
	RotateAtPct int
}

type ChatLiveItem struct {
	ID   string
	Text string
}

// Snapshot returns sid's live in-memory state. ok=false means no live engine
// is registered for sid — a hard-killed server leaves user_interactive rows
// with no engine, so callers must not assume a DB row implies a snapshot.
func (s *ChatService) Snapshot(sid string) (ChatSnapshot, bool) {
	sess, ok := s.get(sid)
	if !ok {
		return ChatSnapshot{}, false
	}
	state := sess.snapshot()
	branch, added, deleted, _ := gitWorkdirStatus(sess.WorkDir())
	rotateAtPct := 0
	if maxContext := sess.MaxContext(); maxContext > 0 {
		profile, _ := ProfileByName(sess.Profile())
		threshold := spawner.ProactiveRestartConsoleThreshold(s.deps.Pool, maxContext, profile.ContextBudgetTokens)
		rotateAtPct = threshold * 100 / maxContext
	}
	return ChatSnapshot{
		Engine:           sess.EngineName(),
		ModelID:          sess.ModelID(),
		WorkDir:          sess.WorkDir(),
		Turn:             string(state.Turn),
		PendingApprovals: state.Pending,
		SessionApprovals: sess.getEngine().SessionApprovals(),
		LiveItems:        state.Live,
		Thinking:         state.Thinking,
		Yolo:             sess.getEngine().Yolo(),
		QueuedPrompts:    sess.queuedPrompts(),
		GitBranch:        branch,
		GitAdded:         added,
		GitDeleted:       deleted,
		RotateAtPct:      rotateAtPct,
	}, true
}

// RevokeSessionApproval removes one tool from sid's session allowlist (so its
// next use asks the human again) and pushes the updated list on the session
// channel — the additive counterpart is pushed by pumpChatEvents when an
// approve_for_session decision resolves.
func (s *ChatService) RevokeSessionApproval(sid, tool string) error {
	sess, ok := s.get(sid)
	if !ok {
		return ErrChatSessionNotFound
	}
	if err := sess.getEngine().RevokeSessionApproval(tool); err != nil {
		return err
	}
	pushSessionApprovals(s.deps.WSHub, sess)
	return nil
}

// SetYolo toggles sid's auto-approve-all-tools state: the engine (immediate
// for claude/api, deferred to next rotation for codex), then the persisted
// per-session column so an override survives rotate/reconnect, then a
// console_chat.yolo push on the session channel.
func (s *ChatService) SetYolo(sid string, on bool) error {
	sess, ok := s.get(sid)
	if !ok {
		return ErrChatSessionNotFound
	}
	if err := sess.getEngine().SetYolo(on); err != nil {
		return err
	}
	if err := repo.NewAgentSessionRepo(s.deps.Pool, s.deps.Clock).SetConsoleYolo(sid, on); err != nil {
		return err
	}
	pushYolo(s.deps.WSHub, sess)
	return nil
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
		sess.getEngine().Stop()
	}
}
