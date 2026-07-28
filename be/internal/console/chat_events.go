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
// row for turn/approval events. EventText/tool events are already persisted by
// chatSink (the engine's own Sink) and broadcast via
// chatSink.BroadcastMessagesUpdated, so this pump does not duplicate them.
// This pump IS the single writer of the thinking/token-usage/approval-resolved
// session pushes for both engines: EventThinking has no other sink (codex
// thinking is never persisted; claude thinking is event-only), EventTokenUsage
// covers claude (whose context update otherwise only reaches the socket path)
// and codex alike, and EventApprovalResolved is how a human reply, a timeout,
// or an engine stopping all resolve the same pending-approval id exactly once.
//
// The channel closing means the engine's run loop is gone (Stop, or the engine
// dying on its own — app-server EOF), so the pump ends the turn and calls
// onEngineExit, which tears the session down. Without that, an engine that
// died mid-turn would leave the turn pinned "running" and every later
// SendMessage rejected with ErrTurnActive against a process that no longer
// exists.
//
// maybeRotate is consulted on every EventTurnCompleted (the idle task
// boundary): when it performs a proactive-restart rotation it returns true,
// and this pump's normal channel-close teardown is skipped for the rest of
// its lifetime — rotate already started a fresh engine + a new pump, which
// owns teardown/idle-push from here on. maybeRotate may be nil (no
// rotation support wired, e.g. tests) — nil is a no-op, never a rotation.
func pumpChatEvents(pool *db.Pool, clk clock.Clock, wsHub *ws.Hub, sess *chatSession, onEngineExit func(), maybeRotate func(*chatSession) bool) {
	auditRepo := repo.NewAuditRepo(pool, clk)
	rotated := false
	// The turn-idle push comes LAST, after the session is torn down: a
	// subscriber that sees it knows the row is already closed.
	defer func() {
		sess.endTurn()
		if rotated {
			return
		}
		if onEngineExit != nil {
			onEngineExit()
		}
		pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatTurn, map[string]interface{}{"state": "idle"})
	}()

	for ev := range sess.getEngine().Events() {
		switch ev.Type {
		case spawner.EventTextDelta:
			sess.appendLive(ev.ItemID, ev.Text)
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatDelta, map[string]interface{}{
				"item_id": ev.ItemID,
				"text":    ev.Text,
			})

		case spawner.EventTurnStarted:
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatTurn, map[string]interface{}{"state": "running"})

		case spawner.EventTurnCompleted:
			sess.endTurn()
			sess.clearLive()
			if maybeRotate != nil && maybeRotate(sess) {
				rotated = true
				appendChatAudit(auditRepo, sess.id, "console_chat.turn_completed", nil)
				continue
			}
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatTurn, map[string]interface{}{"state": "idle"})
			appendChatAudit(auditRepo, sess.id, "console_chat.turn_completed", nil)

		case spawner.EventApprovalRequest:
			if ev.Approval == nil {
				continue
			}
			sess.addPendingApproval(ev.Approval)
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatApprovalRequest, map[string]interface{}{
				"approval_id": ev.Approval.ID,
				"kind":        ev.Approval.Kind,
				"command":     ev.Approval.Command,
				"cwd":         ev.Approval.Cwd,
				"reason":      ev.Approval.Reason,
			})
			meta, _ := json.Marshal(map[string]interface{}{"approval_id": ev.Approval.ID, "command": ev.Approval.Command})
			appendChatAudit(auditRepo, sess.id, "console_chat.approval_request", meta)

		case spawner.EventApprovalResolved:
			sess.resolvePendingApproval(ev.ApprovalID)
			decision := clientDecision(ev.Decision)
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatApprovalResolved, map[string]interface{}{
				"approval_id": ev.ApprovalID,
				"decision":    decision,
				"reason":      ev.Text,
			})
			if ev.Decision == spawner.ApprovalApproveForSession {
				pushSessionApprovals(wsHub, sess)
			}
			meta, _ := json.Marshal(map[string]interface{}{"approval_id": ev.ApprovalID, "decision": decision})
			appendChatAudit(auditRepo, sess.id, "console_chat.approval_resolved", meta)

		case spawner.EventToolInvoke:
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatToolStarted, map[string]interface{}{
				"tool":   ev.ToolName,
				"detail": spawner.FormatToolDetail(ev.ToolName, ev.ToolInput),
			})

		case spawner.EventToolResult:
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatToolFinished, map[string]interface{}{
				"tool":     ev.ToolName,
				"is_error": ev.IsError,
			})

		case spawner.EventThinking:
			sess.appendThinking(ev.ItemID, ev.Text)
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatThinking, map[string]interface{}{
				"item_id": ev.ItemID,
				"text":    ev.Text,
			})

		case spawner.EventTokenUsage:
			sess.noteContextLeft(ev.ContextLeftPct)
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventAgentContextUpdated, map[string]interface{}{
				"session_id":   sess.id,
				"context_left": ev.ContextLeftPct,
			})

		case spawner.EventError:
			// Every engine EventError is turn-terminal (codex: turn/completed
			// carrying an error, an `error` notification, or the app-server
			// connection dropping; claude: the CLI process dying). End the turn
			// so the state machine cannot stay pinned "running" against an engine
			// that will never report turn/completed.
			sess.endTurn()
			sess.clearLive()
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatError, map[string]interface{}{
				"text":     ev.Text,
				"is_error": ev.IsError,
			})
			pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatTurn, map[string]interface{}{"state": "idle"})
		}
	}
}

// clientDecision maps the spawner's engine-facing approval vocabulary onto the
// two values the REST/WS contract speaks. POST .../approvals/{aid} accepts only
// "allow"/"deny" (handlers_console_chat.go), so console_chat.approval_resolved
// must answer in the same vocabulary — pushing the raw spawner value would make
// an approved tool ("approve") arrive as neither, and render as denied.
func clientDecision(d spawner.ApprovalDecision) string {
	switch d {
	case spawner.ApprovalApprove, spawner.ApprovalApproveForSession:
		return "allow"
	default:
		return "deny"
	}
}

// pushSessionApprovals pushes the engine's current session-approved tool list
// — sent whenever the list changes (approve_for_session resolution, revoke),
// always as the full list so consumers never have to merge deltas.
func pushSessionApprovals(wsHub *ws.Hub, sess *chatSession) {
	tools := sess.getEngine().SessionApprovals()
	if tools == nil {
		tools = []string{}
	}
	pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatSessionApprovals, map[string]interface{}{
		"tools": tools,
	})
}

// pushYolo pushes the engine's current yolo state — sent whenever it changes,
// mirroring pushSessionApprovals.
func pushYolo(wsHub *ws.Hub, sess *chatSession) {
	pushSessionEvent(wsHub, sess.id, sess.projectID, ws.EventConsoleChatYolo, map[string]interface{}{
		"yolo": sess.getEngine().Yolo(),
	})
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
