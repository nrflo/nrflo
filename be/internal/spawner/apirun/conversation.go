package apirun

import (
	"context"
	"sync"

	"be/internal/spawner/apirun/provider"
)

// StreamHook receives raw text/thinking deltas as they arrive from the
// provider, before the runner sink's chunked buffering — used by a live
// console consumer that wants deltas as they stream rather than in ~4KB
// chunks. Autonomous Runner.Run passes nil (Config.Stream unset).
//
// itemID names the segment the delta belongs to and rotates every time the
// sink flushes that buffer to a persisted row, so one id covers exactly one
// row: a consumer accumulating deltas per id can drop its buffer when the
// matching row arrives, instead of concatenating the whole session.
type StreamHook interface {
	OnTextDelta(itemID, text string)
	OnThinkingDelta(itemID, text string)
}

// Conversation drives a multi-turn API-mode session: unlike Runner (single-
// shot, discards history after Run returns), it keeps the provider.Message
// history across SendTurn calls so a console chat engine can hold a
// multi-turn conversation over the same tool-use loop Run uses. MaxIterations
// applies per SendTurn, never across the whole session — a turn ending in
// end_turn is a turn boundary, not a session end, so SendTurn never sets a
// session-final status itself (whatever runTurns sets on proc is the calling
// engine's concern, not Conversation's).
type Conversation struct {
	cfg Config

	mu   sync.Mutex
	msgs []provider.Message
}

// NewConversation constructs a Conversation from cfg, applying the same
// defaults as NewRunner (MaxIterations/MaxTokens/MaxContext).
func NewConversation(cfg Config) *Conversation {
	r := NewRunner(cfg)
	return &Conversation{cfg: r.cfg}
}

// SendTurn appends text as a user message, runs the shared tool-use loop
// (outside of any lock, so a concurrent Stop can still proceed), stores the
// resulting history, and returns the terminal status of THIS turn (PASS on
// end_turn / FAIL / RATE_LIMITED / CANCELLED).
func (c *Conversation) SendTurn(ctx context.Context, proc ProcState, text string) string {
	c.mu.Lock()
	msgs := append(append([]provider.Message{}, c.msgs...), provider.Message{
		Role:    "user",
		Content: []provider.ContentBlock{{Type: "text", Text: text}},
	})
	c.mu.Unlock()

	r := &Runner{cfg: c.cfg}
	newMsgs, status := r.runTurns(ctx, proc, msgs)

	c.mu.Lock()
	c.msgs = newMsgs
	c.mu.Unlock()

	return status
}
