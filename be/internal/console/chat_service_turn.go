package console

import (
	"context"
	"errors"
	"strings"

	"be/internal/spawner"
)

// SendMessage submits one user turn, or — when a turn is already in flight —
// queues the text for delivery as the next turn (queued=true; the flush
// happens on EventTurnCompleted, chat_queue.go). ErrPromptQueueFull when the
// mid-turn queue is at capacity. The idle path folds any queued leftovers
// (e.g. a turn that ended in EventError never flushes) ahead of the new text.
//
// A leading "/name" in the RAW text is matched against the session's
// project skills (resolveSkill); when matched, the resolved
// spawner.SkillMatch rides on UserTurn.Skill and the seed-context prepend is
// deferred (a skill turn already carries its own body — see
// spawner.expandSkillTurn) rather than applied to the persisted text. When
// unmatched, the existing takeSeedContext prepend behavior is unchanged.
// Pass-through-vs-expand for a matched skill is entirely an engine decision
// (Rule 6) — this method never inspects sess.EngineName().
func (s *ChatService) SendMessage(sid, text string) (queued bool, err error) {
	sess, ok := s.get(sid)
	if !ok {
		return false, ErrChatSessionNotFound
	}
	// Two attempts: steering can lose its race with the turn ending
	// (ErrNoActiveTurn), in which case the message belongs in a fresh turn.
	for attempt := 0; ; attempt++ {
		if err := sess.beginTurn(); err != nil {
			if !errors.Is(err, spawner.ErrTurnActive) {
				return false, err
			}
			steerErr := sess.getEngine().SteerUserTurn(context.Background(), text)
			if steerErr == nil {
				return false, nil // delivered into the running turn
			}
			if errors.Is(steerErr, spawner.ErrNoActiveTurn) && attempt == 0 {
				continue
			}
			// ErrSteeringUnsupported (codex) or any real steering failure:
			// fall back to the mid-turn queue, delivered on turn end.
			if !sess.enqueuePrompt(text) {
				return false, ErrPromptQueueFull
			}
			pushQueued(s.deps.WSHub, sess)
			return true, nil
		}
		break
	}
	if q := sess.takeQueuedPrompts(); len(q) > 0 {
		text = strings.Join(append(q, text), "\n\n")
		defer pushQueued(s.deps.WSHub, sess)
	}
	return false, s.dispatchTurn(sess, text)
}

// dispatchTurn hands one composed turn to the engine; the caller has already
// won beginTurn. Shared by SendMessage and flushQueuedPrompts.
func (s *ChatService) dispatchTurn(sess *chatSession, text string) error {
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

// AnswerQuestion resolves a pending AskUserQuestion approval with the user's
// free-form answer — the engine wires it back to the model and emits the
// EventApprovalResolved that pumpChatEvents turns into the WS resolution.
func (s *ChatService) AnswerQuestion(sid, approvalID, answer string) error {
	sess, ok := s.get(sid)
	if !ok {
		return ErrChatSessionNotFound
	}
	return sess.getEngine().AnswerQuestion(approvalID, answer)
}

// Interrupt cancels the active turn without closing the chat session.
func (s *ChatService) Interrupt(ctx context.Context, sid string) error {
	sess, ok := s.get(sid)
	if !ok {
		return ErrChatSessionNotFound
	}
	return sess.getEngine().InterruptTurn(ctx)
}
