package console

import (
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// chatSink implements spawner.Sink for one console-chat session
// (be/internal/spawner/cli_adapter.go:167). It cannot reuse
// service.AgentService.RecordHookMessage/UpdateContextLeft as-is: their
// broadcast-id lookup INNER JOINs workflow_instances (service/agent.go), and
// a chat session has none, so the lookup silently returns empty ids and the
// broadcast never fires. chatSink instead knows its own projectID and pushes
// straight onto the session WS channel.
//
// BumpLastMessage/SetLastMessage/OnTurnComplete are no-ops: those exist to
// drive processInfo-based stall/idle timers, and a chat session's engine
// holds no processInfo — this IS the structural stall/nudge exemption.
type chatSink struct {
	pool      *db.Pool
	clock     clock.Clock
	wsHub     *ws.Hub
	errorSvc  *service.ErrorService
	sessionID string
	projectID string
	// refinery signals the session's refinery sidecar after a message row
	// lands, so a fold trigger sees fresh conversation content. Nil-safe —
	// unset when no RefineryLifecycle is wired.
	refinery RefineryLifecycle
}

// RecordHookMessage inserts one agent_messages row directly and touches the
// refinery sidecar (if wired) — the single choke point for engine-originated
// conversation rows across all three console engines. projectID is this
// session's own project; ticket/workflow stay empty (a chat session is bound
// to neither).
func (s *chatSink) RecordHookMessage(sessionID, content, category, payload string) (projectID, ticketID, workflowName string, err error) {
	msgRepo := repo.NewAgentMessageRepo(s.pool, s.clock)
	if err := msgRepo.InsertBatch(sessionID, []repo.MessageEntry{{Content: content, Category: category, Payload: payload}}); err != nil {
		return "", "", "", err
	}
	if s.refinery != nil {
		s.refinery.Touch(sessionID)
	}
	return s.projectID, "", "", nil
}

// UpdateContextLeft persists context_left and pushes nothing: the sink only
// ever sees a codex context update, and pumpChatEvents already pushes that one
// (spawner.EventTokenUsage), so a push here would double-fire. A claude context
// update instead arrives over the unix socket, where handler_context.go both
// fans out on the session channel and forwards to the engine — which the pump
// then pushes too, so claude's agent.context_updated legitimately arrives twice.
// Harmless (the value is absolute, and the FE reducer is idempotent), but the
// pump is NOT the single writer of the context push the way it is for
// approval_resolved/thinking.
func (s *chatSink) UpdateContextLeft(sessionID string, pct int) (projectID, ticketID, workflowName string, err error) {
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(`UPDATE agent_sessions SET context_left = ?, updated_at = ? WHERE id = ?`, pct, now, sessionID); err != nil {
		return "", "", "", err
	}
	return s.projectID, "", "", nil
}

func (s *chatSink) BumpLastMessage(sessionID string)         {}
func (s *chatSink) SetLastMessage(sessionID, content string) {}
func (s *chatSink) OnTurnComplete(sessionID string)          {}

// BroadcastMessagesUpdated pushes messages.updated on the session channel
// only — a chat session has no project/ticket subscription scope to fan out
// to, and the event is ephemeral like every other chat push.
func (s *chatSink) BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID string) {
	if s.wsHub == nil {
		return
	}
	s.wsHub.BroadcastSession(&ws.Event{
		Type:      ws.EventMessagesUpdated,
		ProjectID: s.projectID,
		SessionID: sessionID,
		Data:      map[string]interface{}{"session_id": sessionID},
	})
}

// RecordError forwards to the shared ErrorService when one is configured.
func (s *chatSink) RecordError(projectID, errType, sessionID, msg string) {
	if s.errorSvc == nil {
		return
	}
	_ = s.errorSvc.RecordError(s.projectID, errType, sessionID, msg)
}
