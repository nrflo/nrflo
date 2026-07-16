package repo

import (
	"database/sql"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
)

// insertConsoleChatSessionRowWithStartedAt is insertConsoleChatSessionRow plus
// an explicit project and started_at, so ordering/scope can be asserted
// independently of insert order.
func insertConsoleChatSessionRowWithStartedAt(t *testing.T, database *db.DB, id, projectID, token, engine string, startedAt time.Time) {
	t.Helper()
	started := startedAt.UTC().Format(time.RFC3339Nano)
	_, err := database.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, kind, spawn_token, console_engine, started_at, created_at, updated_at)
		VALUES (?, ?, '', NULL, 'console_chat', 'console_chat', 'sonnet-5', ?, 'console_chat', ?, ?, ?, ?, ?)`,
		id, projectID, model.AgentSessionUserInteractive, token, engine, started, started, started)
	if err != nil {
		t.Fatalf("insert console_chat session %s: %v", id, err)
	}
}

// TestListConsoleChats_KindAndProjectFilteredOrderedByStartedAtDesc is the
// security boundary from repo/CLAUDE.md's kind guard applied to a listing
// query: ListConsoleChats must return only kind='console_chat' rows scoped to
// the requested project, most recently started first.
func TestListConsoleChats_KindAndProjectFilteredOrderedByStartedAtDesc(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	if _, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-b', 'Other', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed proj-b: %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	insertConsoleChatSessionRowWithStartedAt(t, database, "chat-older", "proj", "tok-older", "codex", base)
	insertConsoleChatSessionRowWithStartedAt(t, database, "chat-newer", "proj", "tok-newer", "claude", base.Add(time.Hour))
	insertConsoleChatSessionRowWithStartedAt(t, database, "chat-other-proj", "proj-b", "tok-other-proj", "codex", base.Add(2*time.Hour))
	insertConsoleSession(t, database, "console-only-list", "tok-console-only-list", model.AgentSessionUserInteractive, base)
	insertWorkflowAgentSession(t, database, "wf-agent-list", model.AgentSessionUserInteractive, base)

	got, err := r.ListConsoleChats("proj", 0)
	if err != nil {
		t.Fatalf("ListConsoleChats: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListConsoleChats(proj) = %d rows, want 2; got %+v", len(got), got)
	}
	if got[0].ID != "chat-newer" || got[1].ID != "chat-older" {
		t.Errorf("ListConsoleChats order = [%s, %s], want [chat-newer, chat-older] (started_at DESC)", got[0].ID, got[1].ID)
	}
	for _, s := range got {
		if s.Kind != model.AgentSessionKindConsoleChat {
			t.Errorf("row %s Kind = %q, want console_chat", s.ID, s.Kind)
		}
		if s.ProjectID != "proj" {
			t.Errorf("row %s ProjectID = %q, want proj", s.ID, s.ProjectID)
		}
	}
}

// TestListConsoleChats_LimitDefaultsAndCaps asserts limit<=0 defaults to 50
// and a positive limit is respected.
func TestListConsoleChats_LimitDefaultsAndCaps(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	base := time.Now().UTC()
	for i := 0; i < 3; i++ {
		insertConsoleChatSessionRowWithStartedAt(t, database, "chat-lim-"+string(rune('a'+i)), "proj", "tok-lim-"+string(rune('a'+i)), "codex", base.Add(time.Duration(i)*time.Minute))
	}

	all, err := r.ListConsoleChats("proj", 0)
	if err != nil {
		t.Fatalf("ListConsoleChats(limit=0): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListConsoleChats(limit=0) = %d rows, want 3 (default cap 50)", len(all))
	}

	limited, err := r.ListConsoleChats("proj", 2)
	if err != nil {
		t.Fatalf("ListConsoleChats(limit=2): %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("ListConsoleChats(limit=2) = %d rows, want 2", len(limited))
	}
}

// TestAgentSessionRepo_Create_PersistsConsoleEngine_RoundtripsThroughGetConsoleChat
// is the console_engine write/read roundtrip: the engine name a chat row was
// started with must survive a Create + GetConsoleChat cycle, since
// GET /api/v1/console/chats depends on it without an in-memory session.
func TestAgentSessionRepo_Create_PersistsConsoleEngine_RoundtripsThroughGetConsoleChat(t *testing.T) {
	t.Parallel()
	_, r := setupConsoleTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	row := &model.AgentSession{
		ID:            "chat-roundtrip",
		ProjectID:     "proj",
		TicketID:      "",
		Phase:         "console_chat",
		NodeID:        "console_chat",
		AgentType:     "console_chat",
		Status:        model.AgentSessionUserInteractive,
		Kind:          model.AgentSessionKindConsoleChat,
		SpawnToken:    sql.NullString{String: "tok-roundtrip", Valid: true},
		StartedAt:     sql.NullString{String: now, Valid: true},
		ConsoleEngine: sql.NullString{String: "claude", Valid: true},
	}
	if err := r.Create(row); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.GetConsoleChat("chat-roundtrip")
	if err != nil {
		t.Fatalf("GetConsoleChat: %v", err)
	}
	if got == nil {
		t.Fatal("GetConsoleChat = nil, want row")
	}
	if !got.ConsoleEngine.Valid || got.ConsoleEngine.String != "claude" {
		t.Errorf("ConsoleEngine = %+v, want valid %q", got.ConsoleEngine, "claude")
	}

	listed, err := r.ListConsoleChats("proj", 0)
	if err != nil {
		t.Fatalf("ListConsoleChats: %v", err)
	}
	if len(listed) != 1 || listed[0].ConsoleEngine.String != "claude" {
		t.Fatalf("ListConsoleChats = %+v, want one row with ConsoleEngine=claude", listed)
	}
}
