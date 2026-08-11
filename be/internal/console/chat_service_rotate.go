package console

import (
	"context"

	"github.com/google/uuid"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
	"be/internal/ws"
)

// maybeRotate checks the shared proactive-restart policy
// (spawner.ProactiveRestartDecision) at an idle task boundary
// (EventTurnCompleted) and, when it fires, rotates sess's engine in place.
// Returns true when a rotation was performed — the caller (pumpChatEvents)
// must skip its normal channel-close teardown in that case, since rotate
// starts a fresh pump for the swapped-in engine.
//
// No-op (false) when: no refinery digest exists yet for this session (there
// is nothing to carry the conversation forward with), the session has no
// context signal yet, or the policy does not fire (disabled, under
// threshold, not idle, min-interval/max-per-session/boundary-window gates).
func (s *ChatService) maybeRotate(sess *chatSession) bool {
	if s.deps.Pool == nil {
		return false
	}
	digest, err := repo.NewRefineryDigestRepo(s.deps.Pool, s.deps.Clock).Get(sess.id)
	if err != nil || digest == nil || digest.Content == "" {
		return false
	}

	currentTokens, ok := sess.currentTokens()
	if !ok {
		return false
	}

	profile, _ := ProfileByName(sess.Profile())
	threshold := spawner.ProactiveRestartConsoleThreshold(s.deps.Pool, sess.MaxContext(), profile.ContextBudgetTokens)
	fire, tokensBefore := spawner.ProactiveRestartDecision(s.deps.Pool, s.deps.Clock, sess.id, currentTokens, threshold, 0, true, false)
	if !fire {
		return false
	}

	return s.rotate(sess, tokensBefore, digest.Content)
}

// rotate stops sess's current engine, starts a fresh engine of the same
// kind/model under a new CLISessionID (same stable console agent_sessions.id
// — chat history and the row stay untouched), swaps it onto sess, restarts
// the event pump, notes the restart against the shared policy store, and
// emits console.context_rotated. Returns true only after the new engine is
// started, swapped in, and its pump launched.
//
// Failures before oldEngine.Stop() (registry/spec/engine build) return false
// with the OLD engine still live and running — the session simply continues
// un-rotated. A failure AFTER oldEngine.Stop() (new engine Start error) also
// returns false: the old engine is now dead, so its pump's channel-close
// teardown (which the caller runs precisely because rotate returned false)
// closes the session cleanly instead of orphaning it.
func (s *ChatService) rotate(sess *chatSession, tokensBefore int, seedDigest string) bool {
	ctx := context.Background()
	oldEngine := sess.getEngine()

	profile, _ := ProfileByName(sess.Profile())

	reg, err := BuildRegistry(s.deps.Tools, profile.Catalogue)
	if err != nil {
		logger.Error(ctx, "console rotation: build tool registry failed", "session_id", sess.id, "error", err)
		return false
	}

	// The session's bearer token is durable on the row; without it the
	// rotated engine's mcp-external bridge cannot adopt the session and every
	// nrflo tool silently vanishes from the fresh CLI conversation.
	row, err := repo.NewAgentSessionRepo(s.deps.Pool, s.deps.Clock).Get(sess.id)
	if err != nil || !row.SpawnToken.Valid || row.SpawnToken.String == "" {
		logger.Error(ctx, "console rotation: resolve session bearer failed", "session_id", sess.id, "error", err)
		return false
	}

	spec, err := buildChatEngineSpec(s.deps.Pool, s.deps.Clock, chatSpecParams{
		SessionID:           sess.id,
		ProjectID:           sess.ProjectID(),
		Engine:              sess.EngineName(),
		ModelID:             sess.ModelID(),
		ReasoningEffort:     sess.ReasoningEffort(),
		SpawnToken:          row.SpawnToken.String,
		ServerURL:           s.deps.ServerURL,
		SystemTemplateID:    sess.SystemTemplateID(),
		Catalogue:           profile.Catalogue,
		NativeToolPolicy:    profile.NativeToolPolicy,
		ContextBudgetTokens: profile.ContextBudgetTokens,
		Yolo:                s.resolveSessionYolo(sess.id),
	})
	if err != nil {
		logger.Error(ctx, "console rotation: build engine spec failed", "session_id", sess.id, "error", err)
		return false
	}
	spec.CLISessionID = uuid.New().String()
	spec.SeededContext = seedDigest

	newEngine, err := s.engineFactory(sess.EngineName(), spawner.EngineDeps{
		Sink: &chatSink{
			pool: s.deps.Pool, clock: s.deps.Clock, wsHub: s.deps.WSHub, errorSvc: s.deps.ErrorSvc,
			sessionID: sess.id, projectID: sess.ProjectID(), refinery: s.deps.RefineryMgr,
		},
		PTY:       s.deps.PTY,
		Hub:       s.deps.Hub,
		NrfloPath: resolveNrfloPath(),
		API: spawner.APIEngineDeps{
			Pool:     s.deps.Pool,
			Clock:    s.deps.Clock,
			Tools:    Specs(reg),
			Handlers: reg,
			ToolEnv:  NewToolEnv(s.deps.Tools, sess.id, sess.ProjectID(), model.AgentSessionKindConsoleChat),
		},
	})
	if err != nil {
		logger.Error(ctx, "console rotation: build engine failed", "session_id", sess.id, "error", err)
		return false
	}

	oldEngine.Stop()
	if s.deps.PTY != nil {
		s.deps.PTY.Remove(sess.id)
	}

	if err := newEngine.Start(ctx, spec); err != nil {
		logger.Error(ctx, "console rotation: start engine failed; session will close", "session_id", sess.id, "error", err)
		return false
	}

	sess.setEngine(newEngine)
	sess.resetContext()
	// The swapped-in engine's cumulative usage restarts at zero on its own
	// provider thread; without this, codex's next tokenUsage report would
	// read as a downward correction and setUsage would bill nothing until
	// the new thread's totals climb back past the old high water. No-op for
	// claude/api, which never populate the reported high water.
	spawner.ResetSessionCostThread(sess.id)
	spawner.NoteProactiveRestart(sess.id, s.deps.Clock)

	tokensAfter, _ := sess.currentTokens()
	logger.Info(ctx, "console context rotated", "session_id", sess.id, "tokens_before", tokensBefore, "tokens_after", tokensAfter)

	pushSessionEvent(s.deps.WSHub, sess.id, sess.ProjectID(), ws.EventConsoleContextRotated, map[string]interface{}{
		"session_id":    sess.id,
		"tokens_before": tokensBefore,
		"tokens_after":  tokensAfter,
	})

	go pumpChatEvents(s.deps.Pool, s.deps.Clock, s.deps.WSHub, sess, func() { s.engineExited(sess.id) }, s.maybeRotate, s.flushQueuedPrompts)
	return true
}
