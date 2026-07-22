package repo

import (
	"testing"

	"be/internal/clock"
)

// TestAgentMessageRepo_GetBySessionCategorized_PreservesContentCategorySeqOrder
// verifies the lean fold-input projection: content+category only, ordered by
// seq, for a mix of categories (text, user_input, tool).
func TestAgentMessageRepo_GetBySessionCategorized_PreservesContentCategorySeqOrder(t *testing.T) {
	pool := newTestPool(t)
	const projectID, sessionID = "proj-cat", "sess-cat"
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO agent_sessions (id, project_id, ticket_id, phase, node_id, agent_type, status, kind, created_at, updated_at)
		 VALUES (?, ?, '', 'ph', 'node', 'ag', 'running', 'workflow_agent', datetime('now'), datetime('now'))`,
		sessionID, projectID,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	r := NewAgentMessageRepo(pool, clock.Real())
	entries := []MessageEntry{
		{Content: "please add a widget", Category: "user_input"},
		{Content: "ran ls -la", Category: "tool"},
		{Content: "assistant reply", Category: "text"},
	}
	if err := r.InsertBatch(sessionID, entries); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	got, err := r.GetBySessionCategorized(sessionID)
	if err != nil {
		t.Fatalf("GetBySessionCategorized: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("got %d messages, want %d", len(got), len(entries))
	}
	for i, e := range entries {
		if got[i].Content != e.Content {
			t.Errorf("messages[%d].Content = %q, want %q", i, got[i].Content, e.Content)
		}
		if got[i].Category != e.Category {
			t.Errorf("messages[%d].Category = %q, want %q", i, got[i].Category, e.Category)
		}
	}
}

// TestAgentMessageRepo_GetBySessionCategorized_EmptySession verifies no rows
// yields an empty (not nil-panicking) slice.
func TestAgentMessageRepo_GetBySessionCategorized_EmptySession(t *testing.T) {
	pool := newTestPool(t)
	r := NewAgentMessageRepo(pool, clock.Real())

	got, err := r.GetBySessionCategorized("no-such-session")
	if err != nil {
		t.Fatalf("GetBySessionCategorized: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d messages for nonexistent session, want 0", len(got))
	}
}
