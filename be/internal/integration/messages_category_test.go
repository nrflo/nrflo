package integration

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

// insertMsgWithCategory inserts a single agent message row with an explicit category.
func insertMsgWithCategory(t *testing.T, env *TestEnv, sessionID string, seq int, content, category string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.Pool.Exec(
		`INSERT INTO agent_messages (session_id, seq, content, category, created_at) VALUES (?, ?, ?, ?, ?)`,
		sessionID, seq, content, category, now,
	)
	if err != nil {
		t.Fatalf("insertMsgWithCategory: %v", err)
	}
}

// === Category filtering via service ===

func TestSessionMessages_CategoryFilter_Subagent(t *testing.T) {
	env := NewTestEnv(t)
	insertTestSession(t, env, "sess-cf-sub1", "CF-SUB1")

	insertMsgWithCategory(t, env, "sess-cf-sub1", 0, "[Task] analyze code", "subagent")
	insertMsgWithCategory(t, env, "sess-cf-sub1", 1, "[Bash] git status", "tool")
	insertMsgWithCategory(t, env, "sess-cf-sub1", 2, "[TaskResult] general-purpose: analyze code", "subagent")
	insertMsgWithCategory(t, env, "sess-cf-sub1", 3, "plain text response", "text")
	insertMsgWithCategory(t, env, "sess-cf-sub1", 4, "[Skill] commit", "skill")

	msgs, total, err := env.AgentSvc.GetSessionMessages("sess-cf-sub1", 0, 0, "subagent")
	if err != nil {
		t.Fatalf("GetSessionMessages(subagent): %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2 for subagent filter, got %d", total)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	for _, msg := range msgs {
		if msg.Category != "subagent" {
			t.Errorf("expected category=subagent, got %q for %q", msg.Category, msg.Content)
		}
	}
	if msgs[0].Content != "[Task] analyze code" {
		t.Errorf("msgs[0].Content = %q, want [Task] analyze code", msgs[0].Content)
	}
	if msgs[1].Content != "[TaskResult] general-purpose: analyze code" {
		t.Errorf("msgs[1].Content = %q, want [TaskResult] ...", msgs[1].Content)
	}
}

func TestSessionMessages_CategoryFilter_Tool(t *testing.T) {
	env := NewTestEnv(t)
	insertTestSession(t, env, "sess-cf-tool1", "CF-TOOL1")

	insertMsgWithCategory(t, env, "sess-cf-tool1", 0, "[Bash] ls", "tool")
	insertMsgWithCategory(t, env, "sess-cf-tool1", 1, "[Read] main.go", "tool")
	insertMsgWithCategory(t, env, "sess-cf-tool1", 2, "[Task] work", "subagent")
	insertMsgWithCategory(t, env, "sess-cf-tool1", 3, "plain text", "text")

	msgs, total, err := env.AgentSvc.GetSessionMessages("sess-cf-tool1", 0, 0, "tool")
	if err != nil {
		t.Fatalf("GetSessionMessages(tool): %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2 for tool filter, got %d", total)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestSessionMessages_CategoryFilter_EmptyString_ReturnsAll(t *testing.T) {
	env := NewTestEnv(t)
	insertTestSession(t, env, "sess-cf-all1", "CF-ALL1")

	insertMsgWithCategory(t, env, "sess-cf-all1", 0, "[Task] work", "subagent")
	insertMsgWithCategory(t, env, "sess-cf-all1", 1, "[Bash] ls", "tool")
	insertMsgWithCategory(t, env, "sess-cf-all1", 2, "text", "text")

	msgs, total, err := env.AgentSvc.GetSessionMessages("sess-cf-all1", 0, 0, "")
	if err != nil {
		t.Fatalf("GetSessionMessages(no filter): %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total=3 (no filter), got %d", total)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
}

func TestSessionMessages_CategoryFilter_Pagination(t *testing.T) {
	env := NewTestEnv(t)
	insertTestSession(t, env, "sess-cf-pag1", "CF-PAG1")

	// Insert 6 tool messages and 4 subagent messages
	for i := 0; i < 6; i++ {
		insertMsgWithCategory(t, env, "sess-cf-pag1", i, "[Bash] cmd", "tool")
	}
	for i := 6; i < 10; i++ {
		insertMsgWithCategory(t, env, "sess-cf-pag1", i, "[Task] work", "subagent")
	}

	// First page of tool messages (limit=3)
	page1, total, err := env.AgentSvc.GetSessionMessages("sess-cf-pag1", 3, 0, "tool")
	if err != nil {
		t.Fatalf("GetSessionMessages page1: %v", err)
	}
	if total != 6 {
		t.Fatalf("expected total=6 for tool filter, got %d", total)
	}
	if len(page1) != 3 {
		t.Fatalf("expected 3 in page1, got %d", len(page1))
	}

	// Second page
	page2, _, err := env.AgentSvc.GetSessionMessages("sess-cf-pag1", 3, 3, "tool")
	if err != nil {
		t.Fatalf("GetSessionMessages page2: %v", err)
	}
	if len(page2) != 3 {
		t.Fatalf("expected 3 in page2, got %d", len(page2))
	}
}

// === InsertBatch with categories via repo ===

func TestSessionMessages_InsertBatch_CategoryPersisted(t *testing.T) {
	env := NewTestEnv(t)
	insertTestSession(t, env, "sess-ib-cat1", "IB-CAT1")

	msgRepo := repo.NewAgentMessagePoolRepo(env.Pool, clock.Real())
	err := msgRepo.InsertBatch("sess-ib-cat1", 0, []repo.MessageEntry{
		{Content: "[Task] run agent", Category: "subagent"},
		{Content: "[Bash] git diff", Category: "tool"},
		{Content: "[Skill] commit", Category: "skill"},
		{Content: "thinking...", Category: "text"},
	})
	if err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	msgs, err := msgRepo.GetBySessionPaginated("sess-ib-cat1", 100, 0)
	if err != nil {
		t.Fatalf("GetBySessionPaginated: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	expected := []struct {
		content  string
		category string
	}{
		{"[Task] run agent", "subagent"},
		{"[Bash] git diff", "tool"},
		{"[Skill] commit", "skill"},
		{"thinking...", "text"},
	}
	for i, msg := range msgs {
		if msg.Content != expected[i].content {
			t.Errorf("msgs[%d].Content = %q, want %q", i, msg.Content, expected[i].content)
		}
		if msg.Category != expected[i].category {
			t.Errorf("msgs[%d].Category = %q, want %q", i, msg.Category, expected[i].category)
		}
	}
}

func TestSessionMessages_InsertBatch_DefaultCategory_IsText(t *testing.T) {
	env := NewTestEnv(t)
	insertTestSession(t, env, "sess-ib-def1", "IB-DEF1")

	msgRepo := repo.NewAgentMessagePoolRepo(env.Pool, clock.Real())
	// Category is empty string — should default to "text"
	err := msgRepo.InsertBatch("sess-ib-def1", 0, []repo.MessageEntry{
		{Content: "no category specified"},
	})
	if err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	msgs, err := msgRepo.GetBySessionPaginated("sess-ib-def1", 100, 0)
	if err != nil {
		t.Fatalf("GetBySessionPaginated: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Category != "text" {
		t.Errorf("Category = %q, want %q (empty defaults to text)", msgs[0].Category, "text")
	}
}

func TestSessionMessages_CategoryInGetSessionMessages_Returned(t *testing.T) {
	env := NewTestEnv(t)
	insertTestSession(t, env, "sess-cat-ret1", "CATRET-1")

	msgRepo := repo.NewAgentMessagePoolRepo(env.Pool, clock.Real())
	err := msgRepo.InsertBatch("sess-cat-ret1", 0, []repo.MessageEntry{
		{Content: "[Task] analyze", Category: "subagent"},
		{Content: "hello", Category: "text"},
	})
	if err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	msgs, total, err := env.AgentSvc.GetSessionMessages("sess-cat-ret1", 0, 0, "")
	if err != nil {
		t.Fatalf("GetSessionMessages: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2, got %d", total)
	}
	if msgs[0].Category != "subagent" {
		t.Errorf("msgs[0].Category = %q, want subagent", msgs[0].Category)
	}
	if msgs[1].Category != "text" {
		t.Errorf("msgs[1].Category = %q, want text", msgs[1].Category)
	}
}
