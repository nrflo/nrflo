package integration

import (
	"testing"
	"time"
)

// insertTestSession creates a ticket, inits a workflow, and inserts an agent session.
func insertTestSession(t *testing.T, env *TestEnv, sessionID, ticketID string) {
	t.Helper()
	env.CreateTicket(t, ticketID, "ticket-"+ticketID)
	env.InitWorkflow(t, ticketID)
	wfiID := env.GetWorkflowInstanceID(t, ticketID, "test")
	env.InsertAgentSession(t, sessionID, ticketID, wfiID, "analyzer", "analyzer", "sonnet")
}

func TestSessionMessages_InsertAndRetrieve(t *testing.T) {
	env := NewTestEnv(t)

	insertTestSession(t, env, "sess-msg-1", "MSG-1")

	// Insert messages directly via pool
	now := time.Now().UTC().Format(time.RFC3339)
	for i, msg := range []string{"hello", "world", "done"} {
		_, err := env.Pool.Exec(
			`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`,
			"sess-msg-1", i, msg, now,
		)
		if err != nil {
			t.Fatalf("failed to insert message %d: %v", i, err)
		}
	}

	// Retrieve via service
	msgs, total, err := env.AgentSvc.GetSessionMessages("sess-msg-1", 0, 0)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0] != "hello" || msgs[1] != "world" || msgs[2] != "done" {
		t.Fatalf("unexpected messages: %v", msgs)
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
}

func TestSessionMessages_BatchInsert(t *testing.T) {
	env := NewTestEnv(t)

	insertTestSession(t, env, "sess-batch-1", "MSG-2")

	// Use a transaction to batch insert
	tx, err := env.Pool.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	msgTexts := []string{"msg-a", "msg-b", "msg-c", "msg-d", "msg-e"}
	for i, msg := range msgTexts {
		_, err := tx.Exec(
			`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`,
			"sess-batch-1", i, msg, now,
		)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to insert message %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Verify via service
	msgs, _, err := env.AgentSvc.GetSessionMessages("sess-batch-1", 0, 0)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
}

func TestSessionMessages_EmptySession(t *testing.T) {
	env := NewTestEnv(t)

	insertTestSession(t, env, "sess-empty-1", "MSG-3")

	// Retrieve messages for session with no messages
	msgs, total, err := env.AgentSvc.GetSessionMessages("sess-empty-1", 0, 0)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
	if total != 0 {
		t.Fatalf("expected total 0, got %d", total)
	}
}

func TestSessionMessages_Pagination(t *testing.T) {
	env := NewTestEnv(t)

	insertTestSession(t, env, "sess-page-1", "MSG-4")

	// Insert 10 messages
	now := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < 10; i++ {
		_, err := env.Pool.Exec(
			`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`,
			"sess-page-1", i, "message-"+string(rune('A'+i)), now,
		)
		if err != nil {
			t.Fatalf("failed to insert message %d: %v", i, err)
		}
	}

	// Get first page (limit=3, offset=0)
	msgs1, total1, err := env.AgentSvc.GetSessionMessages("sess-page-1", 3, 0)
	if err != nil {
		t.Fatalf("failed to get page 1: %v", err)
	}
	if len(msgs1) != 3 {
		t.Fatalf("expected 3 messages in page 1, got %d", len(msgs1))
	}
	if total1 != 10 {
		t.Fatalf("expected total 10, got %d", total1)
	}

	// Get second page (limit=3, offset=3)
	msgs2, _, err := env.AgentSvc.GetSessionMessages("sess-page-1", 3, 3)
	if err != nil {
		t.Fatalf("failed to get page 2: %v", err)
	}
	if len(msgs2) != 3 {
		t.Fatalf("expected 3 messages in page 2, got %d", len(msgs2))
	}

	// Verify pages have different content
	if msgs1[0] == msgs2[0] {
		t.Fatal("page 1 and page 2 have the same first message")
	}
}

func TestSessionMessages_CountBatch(t *testing.T) {
	env := NewTestEnv(t)

	// Create two sessions
	insertTestSession(t, env, "sess-count-1", "MSG-5a")
	insertTestSession(t, env, "sess-count-2", "MSG-5b")

	// Insert different numbers of messages
	now := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < 5; i++ {
		env.Pool.Exec(
			`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`,
			"sess-count-1", i, "msg", now,
		)
	}
	for i := 0; i < 3; i++ {
		env.Pool.Exec(
			`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`,
			"sess-count-2", i, "msg", now,
		)
	}

	// Verify counts via service
	_, total1, err := env.AgentSvc.GetSessionMessages("sess-count-1", 0, 0)
	if err != nil {
		t.Fatalf("failed to get sess-count-1 messages: %v", err)
	}
	if total1 != 5 {
		t.Fatalf("expected 5 messages for sess-count-1, got %d", total1)
	}

	_, total2, err := env.AgentSvc.GetSessionMessages("sess-count-2", 0, 0)
	if err != nil {
		t.Fatalf("failed to get sess-count-2 messages: %v", err)
	}
	if total2 != 3 {
		t.Fatalf("expected 3 messages for sess-count-2, got %d", total2)
	}
}

func TestSessionMessages_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	// Service should return error for non-existent session
	_, _, err := env.AgentSvc.GetSessionMessages("does-not-exist", 0, 0)
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
}
