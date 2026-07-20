package console

import (
	"context"

	"be/internal/spawner"
)

// SendMessage submits one user turn. Returns spawner.ErrTurnActive when a
// turn is already in flight (the REST handler maps this to 409) — rejected
// locally via chatSession.beginTurn before ever reaching the engine, so the
// reject is deterministic without a round trip.
//
// A leading "/name" in the RAW text is matched against the session's
// project skills (resolveSkill); when matched, the resolved
// spawner.SkillMatch rides on UserTurn.Skill and the seed-context prepend is
// deferred (a skill turn already carries its own body — see
// spawner.expandSkillTurn) rather than applied to the persisted text. When
// unmatched, the existing takeSeedContext prepend behavior is unchanged.
// Pass-through-vs-expand for a matched skill is entirely an engine decision
// (Rule 6) — this method never inspects sess.EngineName().
func (s *ChatService) SendMessage(sid, text string) error {
	sess, ok := s.get(sid)
	if !ok {
		return ErrChatSessionNotFound
	}
	if err := sess.beginTurn(); err != nil {
		return err
	}
	turn := spawner.UserTurn{Text: text}
	if match := s.resolveSkill(sess.WorkDir(), text); match != nil {
		turn.Skill = match
	} else if seed := sess.takeSeedContext(); seed != "" {
		turn.Text = seed + "\n\n" + text
	}
	if err := sess.getEngine().SendUserTurn(context.Background(), turn); err != nil {
		sess.endTurn()
		return err
	}
	return nil
}

// ReplyApproval forwards an already-mapped decision to the engine. The
// engine's own EventApprovalResolved (handled once, by pumpChatEvents) is
// what resolves the pending approval and pushes console_chat.approval_resolved
// — this method does not duplicate that, since the same resolution can also
// arrive via a timeout or engine stop that never goes through here. The REST
// layer maps allow|deny to the spawner.ApprovalDecision wire vocabulary; this
// method never touches that mapping itself.
func (s *ChatService) ReplyApproval(sid, approvalID string, decision spawner.ApprovalDecision) error {
	sess, ok := s.get(sid)
	if !ok {
		return ErrChatSessionNotFound
	}
	return sess.getEngine().ReplyApproval(approvalID, decision)
}

// Interrupt cancels the active turn without closing the chat session.
func (s *ChatService) Interrupt(ctx context.Context, sid string) error {
	sess, ok := s.get(sid)
	if !ok {
		return ErrChatSessionNotFound
	}
	return sess.getEngine().InterruptTurn(ctx)
}
