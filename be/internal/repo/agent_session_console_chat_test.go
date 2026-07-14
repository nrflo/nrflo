package repo

import (
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
)

// insertConsoleChatSessionRow mirrors insertConsoleSession but with kind='console_chat'.
func insertConsoleChatSessionRow(t *testing.T, database *db.DB, id, token string, status model.AgentSessionStatus, updatedAt time.Time) {
	t.Helper()
	updated := updatedAt.UTC().Format(time.RFC3339Nano)
	_, err := database.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, kind, spawn_token, created_at, updated_at)
		VALUES (?, 'proj', '', NULL, 'console_chat', 'console_chat', 'sonnet', ?, 'console_chat', ?, ?, ?)`,
		id, status, token, updated, updated)
	if err != nil {
		t.Fatalf("insert console_chat session %s: %v", id, err)
	}
}

func TestConsoleChatSession_GetByTokenAndClose(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	insertConsoleChatSessionRow(t, database, "chat-1", "tok-chat-1", model.AgentSessionUserInteractive, time.Now().UTC())

	got, err := r.GetByToken("tok-chat-1")
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got == nil || got.ID != "chat-1" {
		t.Fatalf("GetByToken = %+v, want chat-1", got)
	}
	if got.Kind != model.AgentSessionKindConsoleChat {
		t.Errorf("Kind = %q, want console_chat", got.Kind)
	}

	n, err := r.CloseConsoleChat("chat-1")
	if err != nil {
		t.Fatalf("CloseConsoleChat: %v", err)
	}
	if n != 1 {
		t.Fatalf("CloseConsoleChat rows = %d, want 1", n)
	}

	closed, err := r.GetConsoleChat("chat-1")
	if err != nil {
		t.Fatalf("GetConsoleChat: %v", err)
	}
	if closed == nil {
		t.Fatal("GetConsoleChat after close = nil, want row")
	}
	if closed.Status != model.AgentSessionInteractiveCompleted {
		t.Errorf("Status = %q, want interactive_completed", closed.Status)
	}

	after, err := r.GetByToken("tok-chat-1")
	if err != nil {
		t.Fatalf("GetByToken after close: %v", err)
	}
	if after != nil {
		t.Errorf("GetByToken after close = %+v, want nil (token must die)", after)
	}
}

func TestConsoleChatSession_CloseConsoleChat_Idempotent(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	insertConsoleChatSessionRow(t, database, "chat-2", "tok-chat-2", model.AgentSessionUserInteractive, time.Now().UTC())

	n1, err := r.CloseConsoleChat("chat-2")
	if err != nil {
		t.Fatalf("first CloseConsoleChat: %v", err)
	}
	if n1 != 1 {
		t.Fatalf("first CloseConsoleChat rows = %d, want 1", n1)
	}

	n2, err := r.CloseConsoleChat("chat-2")
	if err != nil {
		t.Fatalf("second CloseConsoleChat: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second CloseConsoleChat rows = %d, want 0", n2)
	}
}

// TestConsoleChatSession_KindGuard_CrossKindIsolation is the security
// property from repo/CLAUDE.md's kind guard: CloseConsoleChat/GetConsoleChat
// must never touch a kind='console' or kind='workflow_agent' row, and
// CloseConsole/GetConsole must never touch a kind='console_chat' row.
func TestConsoleChatSession_KindGuard_CrossKindIsolation(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	insertConsoleSession(t, database, "console-only", "tok-console-only", model.AgentSessionUserInteractive, time.Now().UTC())
	insertConsoleChatSessionRow(t, database, "chat-only", "tok-chat-only", model.AgentSessionUserInteractive, time.Now().UTC())
	insertWorkflowAgentSession(t, database, "wf-agent-x", model.AgentSessionUserInteractive, time.Now().UTC())

	// GetConsoleChat must reject a console/workflow_agent id.
	if got, err := r.GetConsoleChat("console-only"); err != nil || got != nil {
		t.Errorf("GetConsoleChat(console id) = %+v, err=%v, want nil,nil", got, err)
	}
	if got, err := r.GetConsoleChat("wf-agent-x"); err != nil || got != nil {
		t.Errorf("GetConsoleChat(workflow_agent id) = %+v, err=%v, want nil,nil", got, err)
	}
	// GetConsole must reject a console_chat id.
	if got, err := r.GetConsole("chat-only"); err != nil || got != nil {
		t.Errorf("GetConsole(console_chat id) = %+v, err=%v, want nil,nil", got, err)
	}

	// CloseConsoleChat must not affect a console/workflow_agent row.
	if n, err := r.CloseConsoleChat("console-only"); err != nil || n != 0 {
		t.Errorf("CloseConsoleChat(console id) rows=%d err=%v, want 0,nil", n, err)
	}
	if n, err := r.CloseConsoleChat("wf-agent-x"); err != nil || n != 0 {
		t.Errorf("CloseConsoleChat(workflow_agent id) rows=%d err=%v, want 0,nil", n, err)
	}
	// CloseConsole must not affect a console_chat row.
	if n, err := r.CloseConsole("chat-only"); err != nil || n != 0 {
		t.Errorf("CloseConsole(console_chat id) rows=%d err=%v, want 0,nil", n, err)
	}

	// All three tokens must remain live (untouched by the wrong-kind close attempts).
	for _, tok := range []string{"tok-console-only", "tok-chat-only", "tok-wf-wf-agent-x"} {
		got, err := r.GetByToken(tok)
		if err != nil {
			t.Fatalf("GetByToken(%q): %v", tok, err)
		}
		if got == nil {
			t.Errorf("GetByToken(%q) = nil, want row untouched by cross-kind close", tok)
		}
	}
}

func TestConsoleChatSessions_ExcludedFromRunningQueriesAndProjectScope(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	insertConsoleChatSessionRow(t, database, "chat-x", "tok-x", model.AgentSessionUserInteractive, time.Now().UTC())

	count, err := r.CountRunning()
	if err != nil {
		t.Fatalf("CountRunning: %v", err)
	}
	if count != 0 {
		t.Errorf("CountRunning = %d, want 0 (console_chat rows are user_interactive, not running)", count)
	}

	running, err := r.GetRunning(10)
	if err != nil {
		t.Fatalf("GetRunning: %v", err)
	}
	if len(running) != 0 {
		t.Errorf("GetRunning = %d rows, want 0", len(running))
	}

	sessions, err := r.GetByProjectScope("proj", "")
	if err != nil {
		t.Fatalf("GetByProjectScope: %v", err)
	}
	for _, s := range sessions {
		if s.ID == "chat-x" {
			t.Errorf("GetByProjectScope returned console_chat row %q, want excluded", s.ID)
		}
	}

	recent, err := r.GetRecent(10)
	if err != nil {
		t.Fatalf("GetRecent: %v", err)
	}
	for _, s := range recent {
		if s.ID == "chat-x" {
			t.Errorf("GetRecent included console_chat row, want excluded")
		}
	}
}

// ExpireIdleConsoles must never touch a console_chat row — chat lifetime is
// owned by console.ChatService (close route / server shutdown), not the
// console idle sweep.
func TestExpireIdleConsoles_DoesNotTouchConsoleChatRows(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	now := time.Now().UTC()
	cutoff := now.Add(-1 * time.Hour)

	insertConsoleChatSessionRow(t, database, "stale-chat", "tok-stale-chat", model.AgentSessionUserInteractive, cutoff.Add(-time.Minute))

	n, err := r.ExpireIdleConsoles(cutoff.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("ExpireIdleConsoles: %v", err)
	}
	if n != 0 {
		t.Fatalf("ExpireIdleConsoles rows = %d, want 0 (must not touch console_chat)", n)
	}

	got, err := r.GetByToken("tok-stale-chat")
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got == nil {
		t.Error("stale console_chat session token was invalidated by ExpireIdleConsoles, want untouched")
	}
}
