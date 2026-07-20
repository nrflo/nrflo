package repo

import (
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// setupMessageFixture creates minimal DB rows for agent_message tests: project → workflow → wfi → session.
func setupMessageFixture(t *testing.T, d *db.DB, sessionID string) {
	t.Helper()
	if _, err := d.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('msg-proj', 'P', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('msg-proj', 'msg-wf', '', 'project', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES ('msg-wfi', 'msg-proj', '', 'msg-wf', 'active', 'project', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert wfi: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at) VALUES (?, 'msg-proj', '', 'msg-wfi', 'ph', 'ag', 'm', 'running', datetime('now'), datetime('now'))`, sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

// TestAgentMessageRepo_InsertBatch_PayloadRoundTrip verifies payload survives the write/read cycle.
func TestAgentMessageRepo_InsertBatch_PayloadRoundTrip(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	const sessionID = "msg-sess-payload"
	setupMessageFixture(t, d, sessionID)

	r := NewAgentMessageRepo(d, clock.Real())
	entries := []MessageEntry{
		{Content: "msg-with-payload", Category: "tool", Payload: `{"cmd":"ls"}`},
		{Content: "msg-no-payload", Category: "text", Payload: ""},
	}
	if err := r.InsertBatch(sessionID, entries); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	msgs, err := r.GetBySessionPaginated(sessionID, 10, 0)
	if err != nil {
		t.Fatalf("GetBySessionPaginated: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}

	// First message: payload should round-trip as-is.
	if msgs[0].Payload != `{"cmd":"ls"}` {
		t.Errorf("msgs[0].Payload = %q, want %q", msgs[0].Payload, `{"cmd":"ls"}`)
	}
	if msgs[0].Category != "tool" {
		t.Errorf("msgs[0].Category = %q, want %q", msgs[0].Category, "tool")
	}

	// Second message: empty payload stores as NULL, COALESCE returns "".
	if msgs[1].Payload != "" {
		t.Errorf("msgs[1].Payload = %q, want empty string (COALESCE of NULL)", msgs[1].Payload)
	}
}

// TestAgentMessageRepo_EmptyPayload_StoredAsNull verifies NULLIF(?, "") stores NULL for empty payload.
func TestAgentMessageRepo_EmptyPayload_StoredAsNull(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	const sessionID = "msg-sess-nullpay"
	setupMessageFixture(t, d, sessionID)

	r := NewAgentMessageRepo(d, clock.Real())
	if err := r.InsertBatch(sessionID, []MessageEntry{
		{Content: "no-payload", Category: "text", Payload: ""},
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	var isNull bool
	if err := d.QueryRow(
		`SELECT payload IS NULL FROM agent_messages WHERE session_id = ? ORDER BY seq DESC LIMIT 1`,
		sessionID,
	).Scan(&isNull); err != nil {
		t.Fatalf("query payload IS NULL: %v", err)
	}
	if !isNull {
		t.Errorf("payload should be NULL when empty string is inserted via NULLIF")
	}
}

// TestAgentMessageRepo_SetToolEnded_PreservesNestedInput verifies the
// json_set SQL used by SetToolEnded tolerates the widened payload shape
// (a nested "input" object alongside tool_use_id): ended_at is added AND the
// input object survives untouched.
func TestAgentMessageRepo_SetToolEnded_PreservesNestedInput(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	const sessionID = "msg-sess-toolend-input"
	setupMessageFixture(t, d, sessionID)

	r := NewAgentMessageRepo(d, clock.Real())
	if err := r.InsertBatch(sessionID, []MessageEntry{
		{Content: "[Bash] ls", Category: "tool", Payload: `{"tool_use_id":"x","input":{"a":1}}`},
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	updated, err := r.SetToolEnded(sessionID, "x", "2025-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("SetToolEnded: %v", err)
	}
	if !updated {
		t.Fatal("SetToolEnded reported no row updated")
	}

	msgs, err := r.GetBySessionPaginated(sessionID, 10, 0)
	if err != nil {
		t.Fatalf("GetBySessionPaginated: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if !strings.Contains(msgs[0].Payload, `"ended_at":"2025-01-01T00:00:00Z"`) {
		t.Errorf("Payload = %q, want ended_at stamped", msgs[0].Payload)
	}
	if !strings.Contains(msgs[0].Payload, `"input":{"a":1}`) {
		t.Errorf("Payload = %q, want nested input object preserved", msgs[0].Payload)
	}
}

// TestAgentMessageRepo_GetBySessionPaginatedFiltered_IncludesPayload verifies filtered query returns payload.
func TestAgentMessageRepo_GetBySessionPaginatedFiltered_IncludesPayload(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	const sessionID = "msg-sess-filtered"
	setupMessageFixture(t, d, sessionID)

	r := NewAgentMessageRepo(d, clock.Real())
	entries := []MessageEntry{
		{Content: "tool-msg", Category: "tool", Payload: `{"tool":"Bash"}`},
		{Content: "text-msg", Category: "text", Payload: ""},
	}
	if err := r.InsertBatch(sessionID, entries); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	msgs, err := r.GetBySessionPaginatedFiltered(sessionID, "tool", 10, 0)
	if err != nil {
		t.Fatalf("GetBySessionPaginatedFiltered: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d filtered messages, want 1", len(msgs))
	}
	if msgs[0].Payload != `{"tool":"Bash"}` {
		t.Errorf("Payload = %q, want %q", msgs[0].Payload, `{"tool":"Bash"}`)
	}
	if msgs[0].Content != "tool-msg" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "tool-msg")
	}
}
