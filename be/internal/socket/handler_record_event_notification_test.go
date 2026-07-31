package socket

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestRecordEvent_Notification_IdleMessage_TriggersNudge verifies a
// "waiting for your input" Notification records exactly one row and fires
// TriggerIdleNudge with reason "idle".
func TestRecordEvent_Notification_IdleMessage_TriggersNudge(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "NOTIF-IDLE-1")
	wfiID := queryWFIID(t, env, "NOTIF-IDLE-1")
	sessionID := "sess-notif-idle-1"
	insertAgentSession(t, env, "NOTIF-IDLE-1", sessionID, wfiID)

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)
	req := buildRecordEventReq(t, "req-notif-idle", sessionID, map[string]interface{}{
		"hook_event_name": "Notification",
		"message":         "Claude is waiting for your input",
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "recorded" {
		t.Errorf("status = %q, want %q", result["status"], "recorded")
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}

	if len(sig.nudgeCalls) != 1 {
		t.Fatalf("TriggerIdleNudge call count = %d, want 1", len(sig.nudgeCalls))
	}
	if sig.nudgeCalls[0].sessionID != sessionID {
		t.Errorf("nudge sessionID = %q, want %q", sig.nudgeCalls[0].sessionID, sessionID)
	}
	if sig.nudgeCalls[0].reason != "idle" {
		t.Errorf("nudge reason = %q, want %q", sig.nudgeCalls[0].reason, "idle")
	}

	_, category := lastAgentMessage(t, env, sessionID)
	if category != model.MsgCategorySystemNotice {
		t.Errorf("category = %q, want %q", category, model.MsgCategorySystemNotice)
	}
}

// TestRecordEvent_Notification_IdleMessage_ConsoleChatSession_NoNudge verifies
// an idle Notification on a console_chat session (not workflow_agent) still
// records model.MsgCategorySystemNotice but does NOT fire TriggerIdleNudge —
// the nudge is scoped to workflow_agent sessions only.
func TestRecordEvent_Notification_IdleMessage_ConsoleChatSession_NoNudge(t *testing.T) {
	env := newHandlerTestEnv(t)
	sessionID := "sess-notif-idle-console-1"
	insertConsoleChatSessionForBroadcastTest(t, env, sessionID)

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)
	req := buildRecordEventReq(t, "req-notif-idle-console", sessionID, map[string]interface{}{
		"hook_event_name": "Notification",
		"message":         "Claude is waiting for your input",
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}
	_, category := lastAgentMessage(t, env, sessionID)
	if category != model.MsgCategorySystemNotice {
		t.Errorf("category = %q, want %q", category, model.MsgCategorySystemNotice)
	}
	if len(sig.nudgeCalls) != 0 {
		t.Errorf("TriggerIdleNudge call count = %d, want 0 (console_chat session must not nudge)", len(sig.nudgeCalls))
	}
}

// TestRecordEvent_Notification_PermissionMessage_TriggersNudgeWithMarker verifies a
// permission-prompt Notification records a distinct "[permission prompt] <tool>"
// marker row and fires TriggerIdleNudge with reason "permission".
func TestRecordEvent_Notification_PermissionMessage_TriggersNudgeWithMarker(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "NOTIF-PERM-1")
	wfiID := queryWFIID(t, env, "NOTIF-PERM-1")
	sessionID := "sess-notif-perm-1"
	insertAgentSession(t, env, "NOTIF-PERM-1", sessionID, wfiID)

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)
	req := buildRecordEventReq(t, "req-notif-perm", sessionID, map[string]interface{}{
		"hook_event_name": "Notification",
		"message":         "Claude needs your permission to use Bash",
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}
	content, category := lastAgentMessage(t, env, sessionID)
	if content != "[permission prompt] Bash" {
		t.Errorf("content = %q, want %q", content, "[permission prompt] Bash")
	}
	if category != "permission" {
		t.Errorf("category = %q, want %q", category, "permission")
	}

	if len(sig.nudgeCalls) != 1 {
		t.Fatalf("TriggerIdleNudge call count = %d, want 1", len(sig.nudgeCalls))
	}
	if sig.nudgeCalls[0].reason != "permission" {
		t.Errorf("nudge reason = %q, want %q", sig.nudgeCalls[0].reason, "permission")
	}
}

// TestRecordEvent_Notification_UnknownMessage_NoNudge verifies an
// unrecognized Notification message is recorded under the default
// model.MsgCategorySystemNotice category (content unchanged) and does NOT
// trigger a nudge — matches the pre-existing "permission requested" case
// covered by TestRecordEvent_VerboseEventsRecorded.
func TestRecordEvent_Notification_UnknownMessage_NoNudge(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "NOTIF-UNK-1")
	wfiID := queryWFIID(t, env, "NOTIF-UNK-1")
	sessionID := "sess-notif-unk-1"
	insertAgentSession(t, env, "NOTIF-UNK-1", sessionID, wfiID)

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)
	req := buildRecordEventReq(t, "req-notif-unk", sessionID, map[string]interface{}{
		"hook_event_name": "Notification",
		"message":         "permission requested",
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "recorded" {
		t.Errorf("status = %q, want %q", result["status"], "recorded")
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}
	content, category := lastAgentMessage(t, env, sessionID)
	if content != "permission requested" {
		t.Errorf("content = %q, want %q (recorded exactly as before)", content, "permission requested")
	}
	if category != model.MsgCategorySystemNotice {
		t.Errorf("category = %q, want %q", category, model.MsgCategorySystemNotice)
	}
	if len(sig.nudgeCalls) != 0 {
		t.Errorf("TriggerIdleNudge call count = %d, want 0 (unknown message must not nudge)", len(sig.nudgeCalls))
	}
}

// TestRecordEvent_Notification_StructuredFieldPrecedence verifies a
// structured "type" field wins over a contradictory message substring.
func TestRecordEvent_Notification_StructuredFieldPrecedence(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "NOTIF-STRUCT-1")
	wfiID := queryWFIID(t, env, "NOTIF-STRUCT-1")
	sessionID := "sess-notif-struct-1"
	insertAgentSession(t, env, "NOTIF-STRUCT-1", sessionID, wfiID)

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)
	req := buildRecordEventReq(t, "req-notif-struct", sessionID, map[string]interface{}{
		"hook_event_name": "Notification",
		"type":            "waiting_for_input",
		// Message text would otherwise classify as "unknown" via substring fallback.
		"message": "something arbitrary happened",
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}
	if len(sig.nudgeCalls) != 1 {
		t.Fatalf("TriggerIdleNudge call count = %d, want 1 (structured field must win)", len(sig.nudgeCalls))
	}
	if sig.nudgeCalls[0].reason != "idle" {
		t.Errorf("nudge reason = %q, want %q", sig.nudgeCalls[0].reason, "idle")
	}
}

// TestRecordEvent_Notification_NilSignaler_NoPanic verifies a nil signaler
// still records the event without panicking.
func TestRecordEvent_Notification_NilSignaler_NoPanic(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "NOTIF-NILSIG-1")
	wfiID := queryWFIID(t, env, "NOTIF-NILSIG-1")
	sessionID := "sess-notif-nilsig-1"
	insertAgentSession(t, env, "NOTIF-NILSIG-1", sessionID, wfiID)

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildRecordEventReq(t, "req-notif-nilsig", sessionID, map[string]interface{}{
		"hook_event_name": "Notification",
		"message":         "Claude is waiting for your input",
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Errorf("nil signaler should not cause error, got: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "recorded" {
		t.Errorf("status = %q, want %q", result["status"], "recorded")
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}
}
