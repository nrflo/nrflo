package console

import (
	"encoding/json"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
	"be/internal/ws"

	"github.com/google/uuid"
)

// pumpChatEvents ranges over engine.Events() until the channel closes, mapping
// each spawner.EngineEvent onto a WS push on the session channel plus an audit
// row for turn/approval events. EventText/EventThinking/tool events are already
// persisted by chatSink (the engine's own Sink) and broadcast via
// chatSink.BroadcastMessagesUpdated, so this pump does not duplicate them —
// it only reacts to delta/turn/approval/error.
//
// The channel closing means the engine's run loop is gone (Stop, or the engine
// dying on its own — app-server EOF), so the pump ends the turn and calls
// onEngineExit, which tears the session down. Without that, an engine that
// died mid-turn would leave the turn pinned "running" and every later
// SendMessage rejected with ErrTurnActive against a process that no longer
// exists.
func pumpChatEvents(pool *db.Pool, clk clock.Clock, wsHub *ws.Hub, sess *chatSession, onEngineExit func()) {
	auditRepo := repo.NewAuditRepo(pool, clk)
	// The turn-idle push comes LAST, after the session is torn down: a
	// subscriber that sees it knows the row is already closed.
	defer func() {
		sess.endTurn()
		if onEngineExit != nil {
			onEngineExit()
		}
		pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatTurn, map[string]interface{}{"state": "idle"})
	}()

	for ev := range sess.engine.Events() {
		switch ev.Type {
		case spawner.EventTextDelta:
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatDelta, map[string]interface{}{
				"item_id": ev.ItemID,
				"text":    ev.Text,
			})

		case spawner.EventTurnStarted:
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatTurn, map[string]interface{}{"state": "running"})

		case spawner.EventTurnCompleted:
			sess.endTurn()
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatTurn, map[string]interface{}{"state": "idle"})
			appendChatAudit(auditRepo, sess.id, "console_chat.turn_completed", nil)

		case spawner.EventApprovalRequest:
			if ev.Approval == nil {
				continue
			}
			sess.addPendingApproval(ev.Approval.ID)
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatApprovalRequest, map[string]interface{}{
				"approval_id": ev.Approval.ID,
				"kind":        ev.Approval.Kind,
				"command":     ev.Approval.Command,
				"cwd":         ev.Approval.Cwd,
				"reason":      ev.Approval.Reason,
			})
			meta, _ := json.Marshal(map[string]interface{}{"approval_id": ev.Approval.ID, "command": ev.Approval.Command})
			appendChatAudit(auditRepo, sess.id, "console_chat.approval_request", meta)

		case spawner.EventError:
			// Every engine EventError is turn-terminal (codex: turn/completed
			// carrying an error, an `error` notification, or the app-server
			// connection dropping; claude: the CLI process dying). End the turn
			// so the state machine cannot stay pinned "running" against an engine
			// that will never report turn/completed.
			sess.endTurn()
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatError, map[string]interface{}{
				"text":     ev.Text,
				"is_error": ev.IsError,
			})
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatTurn, map[string]interface{}{"state": "idle"})
		}
	}
}

// pushSessionEvent is a nil-safe helper for a session-channel WS push.
func pushSessionEvent(wsHub *ws.Hub, sessionID, projectID, eventType string, data map[string]interface{}) {
	if wsHub == nil {
		return
	}
	wsHub.BroadcastSession(&ws.Event{
		Type:      eventType,
		ProjectID: projectID,
		SessionID: sessionID,
		Data:      data,
	})
}

// appendChatAudit records one console-chat lifecycle event keyed to the
// session (resource_type=agent_session, resource_id=<session id>) — same
// shape as api/console_audit.go's appendConsoleToolAudit.
func appendChatAudit(auditRepo *repo.AuditRepo, sessionID, action string, metadata json.RawMessage) {
	m := string(metadata)
	if m == "" {
		m = "{}"
	}
	_ = auditRepo.Append(&model.AuditEntry{
		ID:           uuid.New().String(),
		Action:       action,
		ResourceType: "agent_session",
		ResourceID:   sessionID,
		Metadata:     m,
	})
}
