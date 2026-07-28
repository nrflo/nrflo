package console

import (
	"context"
	"errors"
	"strings"

	"be/internal/logger"
	"be/internal/ws"
)

// setSeedContext stashes text to prepend to the first SendUserTurn call —
// used by OpenHandsSibling to seed the sibling's first turn with the
// origin's refinery digest before the caller ever sends a message.
func (c *chatSession) setSeedContext(text string) {
	c.mu.Lock()
	c.seedContext = text
	c.mu.Unlock()
}

// appendSeedContext appends text to the pending seed context, joined by
// "\n\n" (an empty slot behaves like a plain assign) — used to fold an
// inform_model invoke digest into the NEXT SendUserTurn without disturbing
// takeSeedContext's consume-once semantics or the skill-turn deferral
// (chat_service_turn.go).
func (c *chatSession) appendSeedContext(text string) {
	c.mu.Lock()
	if c.seedContext == "" {
		c.seedContext = text
	} else {
		c.seedContext += "\n\n" + text
	}
	c.mu.Unlock()
}

// takeSeedContext returns and clears the pending seed context — consumed
// exactly once, by the first SendUserTurn.
func (c *chatSession) takeSeedContext() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	text := c.seedContext
	c.seedContext = ""
	return text
}

// maxQueuedPrompts bounds the mid-turn prompt queue; beyond it SendMessage
// returns ErrPromptQueueFull (mapped to 409 by the REST handler).
const maxQueuedPrompts = 32

// ErrPromptQueueFull is returned by SendMessage when a mid-turn prompt cannot
// be queued because the session's queue is at capacity.
var ErrPromptQueueFull = errors.New("console_chat_prompt_queue_full")

// enqueuePrompt appends text for delivery when the current turn ends.
// Reports false when the queue is at capacity.
func (c *chatSession) enqueuePrompt(text string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.queued) >= maxQueuedPrompts {
		return false
	}
	c.queued = append(c.queued, text)
	return true
}

// takeQueuedPrompts returns and clears every queued prompt (nil when empty).
func (c *chatSession) takeQueuedPrompts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	q := c.queued
	c.queued = nil
	return q
}

// queuedPrompts returns a copy of the queue for snapshots.
func (c *chatSession) queuedPrompts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.queued))
	copy(out, c.queued)
	return out
}

// pushQueued pushes the session's full current queue on the session channel —
// sent whenever the queue changes (enqueue, fold into a turn, flush), always
// as the full list so consumers never merge deltas.
func pushQueued(wsHub *ws.Hub, sess *chatSession) {
	prompts := sess.queuedPrompts()
	pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatQueued, map[string]interface{}{
		"count":   len(prompts),
		"prompts": prompts,
	})
}

// flushQueuedPrompts delivers the prompts queued during the just-finished
// turn as the next turn. Called by pumpChatEvents on EventTurnCompleted, on
// its own goroutine — the pump must keep draining engine events, and a
// claude SendUserTurn briefly sleeps between the body write and the submit
// CR. Losing the beginTurn race to a concurrent SendMessage is fine: that
// message's dispatch folds the queue into its own turn.
func (s *ChatService) flushQueuedPrompts(sess *chatSession) {
	if err := sess.beginTurn(); err != nil {
		return
	}
	q := sess.takeQueuedPrompts()
	if len(q) == 0 {
		sess.endTurn()
		return
	}
	if err := s.dispatchTurn(sess, strings.Join(q, "\n\n")); err != nil {
		logger.Error(context.Background(), "console chat: flush queued prompts failed", "session_id", sess.id, "error", err)
		pushSessionEvent(s.deps.WSHub, sess.id, sess.projectID, ws.EventConsoleChatError, map[string]interface{}{
			"text":     "queued message failed to send: " + err.Error(),
			"is_error": true,
		})
	}
	pushQueued(s.deps.WSHub, sess)
}
