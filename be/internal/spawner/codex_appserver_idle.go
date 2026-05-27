package spawner

import (
	"context"
	"time"

	"be/internal/logger"
)

// Idle/nudge handling for the codex app-server backend. The app-server only nudges
// between turns: a completed turn with no `agent_finished` is an unambiguous stop
// (codex never self-continues), so the wait is the short between-turns grace rather
// than the full PTY-oriented idle window. Mid-turn silence is covered by the stall
// detector in monitorAll.

// armIdleTimer returns a channel that fires once the agent has sat idle between
// turns long enough to nudge. Only armed when a turn is NOT active (the agent
// completed a turn and may have stopped without calling `nrflo agent finished`).
// The wait is the short between-turns grace, not the full idle window — see
// betweenTurnsDelay.
func (b *codexAppServerBackend) armIdleTimer(proc *processInfo, turnActive bool) <-chan time.Time {
	if turnActive || proc.nudgeMax == 0 {
		return nil
	}
	window := b.betweenTurnsDelay(proc)
	if window <= 0 {
		return nil
	}
	proc.messagesMutex.Lock()
	last := proc.lastMessageTime
	proc.messagesMutex.Unlock()
	remaining := window - b.s.config.Clock.Now().Sub(last)
	if remaining < 0 {
		remaining = 0
	}
	return b.s.config.Clock.After(remaining)
}

func (b *codexAppServerBackend) idleWindow(proc *processInfo) time.Duration {
	proc.messagesMutex.Lock()
	hasMsg := proc.hasReceivedMessage
	proc.messagesMutex.Unlock()
	if hasMsg {
		return proc.idleAfterMessageTimeout
	}
	return proc.idleStartTimeout
}

// betweenTurnsDelay is how long to wait after a completed turn (no finish call)
// before nudging. Capped at codexBetweenTurnsNudgeDelay so a stalled codex agent
// is re-prompted within seconds instead of the full PTY-oriented idle window,
// while a shorter configured window still wins (and a 0/disabled window propagates
// through to suppress the nudge entirely).
func (b *codexAppServerBackend) betweenTurnsDelay(proc *processInfo) time.Duration {
	return min(b.idleWindow(proc), codexBetweenTurnsNudgeDelay)
}

// nudge re-issues a turn carrying the finish-reminder injectable, then records
// the nudge via the shared helper.
func (b *codexAppServerBackend) nudge(runCtx, logCtx context.Context, proc *processInfo, req SpawnRequest, client *appServerClient, threadID, effort string) {
	body := b.s.nudgeBody(proc)
	if _, err := client.call(runCtx, "turn/start", turnStartParams(threadID, body, effort, "")); err != nil {
		logger.Warn(logCtx, "codex app-server: nudge turn/start error", "session_id", proc.sessionID, "error", err)
	}
	b.s.recordNudgeSent(logCtx, proc, req)
}
