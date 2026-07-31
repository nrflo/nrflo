package socket

import (
	"testing"

	"be/internal/model"
)

// TestRecordEvent_UserPromptSubmit_TaskNotificationEnvelope_RecordsCategory
// verifies a UserPromptSubmit prompt that is a Claude Code CLI harness
// <task-notification> envelope records under model.MsgCategoryTaskNotification
// instead of "user_input".
func TestRecordEvent_UserPromptSubmit_TaskNotificationEnvelope_RecordsCategory(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TASKNOTIFY-1")
	wfiID := queryWFIID(t, env, "TASKNOTIFY-1")
	sessionID := "sess-tasknotify-1"
	insertAgentSession(t, env, "TASKNOTIFY-1", sessionID, wfiID)

	prompt := "<task-notification><task_id>t-1</task_id><status>done</status></task-notification>"
	req := buildRecordEventReq(t, "req-tasknotify-1", sessionID, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          prompt,
	})
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}
	content, category := lastAgentMessage(t, env, sessionID)
	if category != model.MsgCategoryTaskNotification {
		t.Errorf("category = %q, want %q", category, model.MsgCategoryTaskNotification)
	}
	if content != prompt {
		t.Errorf("content = %q, want unchanged %q", content, prompt)
	}
}

// TestRecordEvent_UserPromptSubmit_OrdinaryPrompt_StillRecordsUserInput
// verifies a normal human-typed prompt is unaffected by the new envelope
// classification and still records as "user_input".
func TestRecordEvent_UserPromptSubmit_OrdinaryPrompt_StillRecordsUserInput(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TASKNOTIFY-2")
	wfiID := queryWFIID(t, env, "TASKNOTIFY-2")
	sessionID := "sess-tasknotify-2"
	insertAgentSession(t, env, "TASKNOTIFY-2", sessionID, wfiID)

	req := buildRecordEventReq(t, "req-tasknotify-2", sessionID, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "please fix the bug",
	})
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	_, category := lastAgentMessage(t, env, sessionID)
	if category != "user_input" {
		t.Errorf("category = %q, want %q", category, "user_input")
	}
}

// TestRecordEvent_UserPromptSubmit_TaskNotification_EngineOwnedEcho_RecordsNothing
// verifies that when a live console engine owns the turn (the echo case),
// a task-notification-shaped prompt still takes the recorded:false branch
// and writes no row — only claudeEngine.NotifyUserPrompt's own pendingEcho
// causes ConsoleUserPrompt to report ownership, so this exercises the same
// "engine already wrote it" guard regardless of envelope shape.
func TestRecordEvent_UserPromptSubmit_TaskNotification_EngineOwnedEcho_RecordsNothing(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TASKNOTIFY-3")
	wfiID := queryWFIID(t, env, "TASKNOTIFY-3")
	sessionID := "sess-tasknotify-3"
	insertAgentSession(t, env, "TASKNOTIFY-3", sessionID, wfiID)

	env.handler.consoleHooks = &fakeConsoleHooks{userPromptOwn: true}

	req := buildRecordEventReq(t, "req-tasknotify-3", sessionID, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "<task-notification><task_id>t-2</task_id></task-notification>",
	})
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if n := countAgentMessages(t, env, sessionID); n != 0 {
		t.Errorf("agent_messages count = %d, want 0 (engine owns the row)", n)
	}
}

// TestIsTaskNotification covers the classification helper directly: leading
// whitespace before the envelope tag still classifies as a task notification,
// while a similarly-shaped but different tag, or plain text, does not.
func TestIsTaskNotification(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   bool
	}{
		{"exact prefix", "<task-notification>foo</task-notification>", true},
		{"leading whitespace", "   \n\t<task-notification>foo</task-notification>", true},
		{"different tag", "<task-update>foo</task-update>", false},
		{"plain text", "please fix the bug", false},
		{"empty", "", false},
		{"tag-like but not envelope prefix", "some text <task-notification>embedded</task-notification>", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTaskNotification(tt.prompt); got != tt.want {
				t.Errorf("isTaskNotification(%q) = %v, want %v", tt.prompt, got, tt.want)
			}
		})
	}
}
