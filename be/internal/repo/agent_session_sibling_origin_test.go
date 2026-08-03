package repo

import (
	"testing"
	"time"

	"be/internal/clock"
)

// Fixtures below seed rows via raw SQL rather than AgentSessionRepo.Create —
// see be_production_bugs (Create's INSERT is column/placeholder-mismatched
// after migration 000230 and errors on every call).

func TestSetSiblingOrigin_PersistsAndSurvivesFreshRead(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-sib', 'P', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, created_at, updated_at)
		VALUES ('origin-sess', 'proj-sib', '', 'p', 'a', 'user_interactive', 'console_chat', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, created_at, updated_at)
		VALUES ('sib-sess', 'proj-sib', '', 'p', 'a', 'user_interactive', 'console_chat', ?, ?)`, now, now)

	r := NewAgentSessionRepo(pool, clock.Real())
	if err := r.SetSiblingOrigin("sib-sess", "origin-sess"); err != nil {
		t.Fatalf("SetSiblingOrigin: %v", err)
	}

	// Read back via a fresh Get — the durability regression this guards
	// against is a transient-only WS push with nothing persisted.
	got, err := r.Get("sib-sess")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.SiblingOriginSessionID != "origin-sess" {
		t.Errorf("SiblingOriginSessionID = %q, want origin-sess", got.SiblingOriginSessionID)
	}
}

func TestListSiblingsByOrigin_ScopedAndOrderedByStartedAtAsc(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-sib2', 'P', ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, created_at, updated_at)
		VALUES ('origin-sess-2', 'proj-sib2', '', 'p', 'a', 'user_interactive', 'console_chat', ?, ?)`, now, now)

	t0, t1 := "2025-01-01T00:00:00Z", "2025-01-01T00:01:00Z"
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, started_at, created_at, updated_at)
		VALUES ('sib-first', 'proj-sib2', '', 'p', 'a', 'user_interactive', 'console_chat', ?, ?, ?)`, t0, now, now)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, started_at, created_at, updated_at)
		VALUES ('sib-second', 'proj-sib2', '', 'p', 'a', 'user_interactive', 'console_chat', ?, ?, ?)`, t1, now, now)
	// An ordinary (non-sibling) chat must never show up.
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, created_at, updated_at)
		VALUES ('not-a-sibling', 'proj-sib2', '', 'p', 'a', 'user_interactive', 'console_chat', ?, ?)`, now, now)

	r := NewAgentSessionRepo(pool, clock.Real())
	if err := r.SetSiblingOrigin("sib-first", "origin-sess-2"); err != nil {
		t.Fatalf("SetSiblingOrigin(sib-first): %v", err)
	}
	if err := r.SetSiblingOrigin("sib-second", "origin-sess-2"); err != nil {
		t.Fatalf("SetSiblingOrigin(sib-second): %v", err)
	}

	list, err := r.ListSiblingsByOrigin("origin-sess-2")
	if err != nil {
		t.Fatalf("ListSiblingsByOrigin: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].ID != "sib-first" || list[1].ID != "sib-second" {
		t.Errorf("order = %s,%s, want sib-first,sib-second (started_at ASC)", list[0].ID, list[1].ID)
	}
}

func TestListSiblingsByOrigin_NoSiblings_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewAgentSessionRepo(pool, clock.Real())
	list, err := r.ListSiblingsByOrigin("no-such-origin")
	if err != nil {
		t.Fatalf("ListSiblingsByOrigin: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("len(list) = %d, want 0", len(list))
	}
}
