package repo

import (
	"testing"

	"be/internal/clock"
)

// TestAgentMessageRepo_GetBySessionCategorizedFromSeq_PreservesContentCategorySeqOrder
// verifies the lean fold-input projection: seq+content+category, ordered by
// seq, for a mix of categories (text, user_input, tool), with fromSeq=0
// returning every row and each row's Seq matching its insertion order.
func TestAgentMessageRepo_GetBySessionCategorizedFromSeq_PreservesContentCategorySeqOrder(t *testing.T) {
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

	got, err := r.GetBySessionCategorizedFromSeq(sessionID, 0)
	if err != nil {
		t.Fatalf("GetBySessionCategorizedFromSeq(0): %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("got %d messages, want %d", len(got), len(entries))
	}
	for i, e := range entries {
		if got[i].Seq != i {
			t.Errorf("messages[%d].Seq = %d, want %d", i, got[i].Seq, i)
		}
		if got[i].Content != e.Content {
			t.Errorf("messages[%d].Content = %q, want %q", i, got[i].Content, e.Content)
		}
		if got[i].Category != e.Category {
			t.Errorf("messages[%d].Category = %q, want %q", i, got[i].Category, e.Category)
		}
	}

	// fromSeq=2 of 3 rows (seq 0,1,2) must return only the tail row (seq 2).
	tail, err := r.GetBySessionCategorizedFromSeq(sessionID, 2)
	if err != nil {
		t.Fatalf("GetBySessionCategorizedFromSeq(2): %v", err)
	}
	if len(tail) != 1 {
		t.Fatalf("GetBySessionCategorizedFromSeq(2) len = %d, want 1", len(tail))
	}
	if tail[0].Seq != 2 || tail[0].Content != "assistant reply" {
		t.Errorf("GetBySessionCategorizedFromSeq(2)[0] = %+v, want {Seq:2 Content:%q}", tail[0], "assistant reply")
	}

	// fromSeq past the tail (3, one past the last seq 2) must return no rows.
	past, err := r.GetBySessionCategorizedFromSeq(sessionID, 3)
	if err != nil {
		t.Fatalf("GetBySessionCategorizedFromSeq(3): %v", err)
	}
	if len(past) != 0 {
		t.Errorf("GetBySessionCategorizedFromSeq(3) len = %d, want 0", len(past))
	}
}

// TestAgentMessageRepo_GetBySessionCategorizedFromSeq_UnknownSession verifies
// an unknown session id yields an empty (not nil-panicking) slice regardless
// of fromSeq.
func TestAgentMessageRepo_GetBySessionCategorizedFromSeq_UnknownSession(t *testing.T) {
	pool := newTestPool(t)
	r := NewAgentMessageRepo(pool, clock.Real())

	got, err := r.GetBySessionCategorizedFromSeq("no-such-session", 0)
	if err != nil {
		t.Fatalf("GetBySessionCategorizedFromSeq: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d messages for nonexistent session, want 0", len(got))
	}
}
