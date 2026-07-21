package repo

import (
	"fmt"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func label(i int) string {
	return fmt.Sprintf("msg-%d", i)
}

// insertConsoleChatSession inserts a kind='console_chat' agent_sessions row
// scoped to projectID (the project must already exist).
func insertConsoleChatSession(t *testing.T, database *db.DB, id, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := database.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, kind, spawn_token, created_at, updated_at)
		VALUES (?, ?, '', NULL, 'console', 'console', 'sonnet-5', ?, ?, ?, ?, ?)`,
		id, projectID, model.AgentSessionUserInteractive, model.AgentSessionKindConsoleChat, "tok-"+id, now, now)
	if err != nil {
		t.Fatalf("insert console_chat session %s: %v", id, err)
	}
}

// insertConsoleMessage inserts one agent_messages row directly with an
// explicit seq/category/created_at, bypassing InsertBatch's auto-seq so
// cross-session created_at ordering is fully test-controlled.
func insertConsoleMessage(t *testing.T, database *db.DB, sessionID string, seq int, category, content string, createdAt time.Time) {
	t.Helper()
	_, err := database.Exec(`INSERT INTO agent_messages (session_id, seq, content, category, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		sessionID, seq, content, category, createdAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert message on %s: %v", sessionID, err)
	}
}

// setupProjectConsoleUserInputsDB creates a DB with a project 'proj-hist'.
func setupProjectConsoleUserInputsDB(t *testing.T) (*db.DB, *AgentMessageRepo) {
	t.Helper()
	database := newTestDB(t)
	if _, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-hist', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("project: %v", err)
	}
	return database, NewAgentMessageRepo(database, clock.Real())
}

func TestProjectConsoleUserInputs_AggregatesAcrossSessionsOldestToNewest(t *testing.T) {
	t.Parallel()
	database, r := setupProjectConsoleUserInputsDB(t)
	insertConsoleChatSession(t, database, "cc-1", "proj-hist")
	insertConsoleChatSession(t, database, "cc-2", "proj-hist")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	insertConsoleMessage(t, database, "cc-1", 0, "user_input", "first", base)
	insertConsoleMessage(t, database, "cc-2", 0, "user_input", "second", base.Add(time.Minute))
	insertConsoleMessage(t, database, "cc-1", 1, "user_input", "third", base.Add(2*time.Minute))

	got, err := r.ProjectConsoleUserInputs("proj-hist", 100)
	if err != nil {
		t.Fatalf("ProjectConsoleUserInputs: %v", err)
	}
	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("got = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestProjectConsoleUserInputs_ProjectIsolation(t *testing.T) {
	t.Parallel()
	database, r := setupProjectConsoleUserInputsDB(t)
	if _, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-hist-b', 'Test B', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("project b: %v", err)
	}
	insertConsoleChatSession(t, database, "cc-a", "proj-hist")
	insertConsoleChatSession(t, database, "cc-b", "proj-hist-b")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	insertConsoleMessage(t, database, "cc-a", 0, "user_input", "mine", base)
	insertConsoleMessage(t, database, "cc-b", 0, "user_input", "other project", base.Add(time.Minute))

	got, err := r.ProjectConsoleUserInputs("proj-hist", 100)
	if err != nil {
		t.Fatalf("ProjectConsoleUserInputs: %v", err)
	}
	if len(got) != 1 || got[0] != "mine" {
		t.Fatalf("got = %v, want [mine] (proj-hist-b's row excluded)", got)
	}
}

func TestProjectConsoleUserInputs_ExcludesNonConsoleChatKind(t *testing.T) {
	t.Parallel()
	database, r := setupProjectConsoleUserInputsDB(t)
	// insertWorkflowAgentSession (agent_session_console_test.go) hardcodes
	// project_id='proj'; seed that project too so the FK is satisfied.
	if _, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("project proj: %v", err)
	}
	insertWorkflowAgentSession(t, database, "wf-agent-hist", model.AgentSessionUserInteractive, time.Now().UTC())
	insertConsoleMessage(t, database, "wf-agent-hist", 0, "user_input", "should be excluded", time.Now().UTC())

	insertConsoleChatSession(t, database, "cc-only", "proj-hist")
	insertConsoleMessage(t, database, "cc-only", 0, "user_input", "kept", time.Now().UTC())

	got, err := r.ProjectConsoleUserInputs("proj-hist", 100)
	if err != nil {
		t.Fatalf("ProjectConsoleUserInputs: %v", err)
	}
	if len(got) != 1 || got[0] != "kept" {
		t.Fatalf("got = %v, want [kept] (workflow_agent session excluded)", got)
	}

	// Confirm the workflow_agent row is untouched under its own (different)
	// project scope too, i.e. it never leaks in regardless of project arg.
	gotProj, err := r.ProjectConsoleUserInputs("proj", 100)
	if err != nil {
		t.Fatalf("ProjectConsoleUserInputs(proj): %v", err)
	}
	if len(gotProj) != 0 {
		t.Errorf("ProjectConsoleUserInputs(proj) = %v, want empty (session kind is workflow_agent, not console_chat)", gotProj)
	}
}

func TestProjectConsoleUserInputs_FiltersNonUserInputCategory(t *testing.T) {
	t.Parallel()
	database, r := setupProjectConsoleUserInputsDB(t)
	insertConsoleChatSession(t, database, "cc-cat", "proj-hist")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	insertConsoleMessage(t, database, "cc-cat", 0, "user_input", "keep-me", base)
	insertConsoleMessage(t, database, "cc-cat", 1, "text", "assistant reply", base.Add(time.Minute))
	insertConsoleMessage(t, database, "cc-cat", 2, "tool", "tool call", base.Add(2*time.Minute))
	insertConsoleMessage(t, database, "cc-cat", 3, "thinking", "internal", base.Add(3*time.Minute))

	got, err := r.ProjectConsoleUserInputs("proj-hist", 100)
	if err != nil {
		t.Fatalf("ProjectConsoleUserInputs: %v", err)
	}
	if len(got) != 1 || got[0] != "keep-me" {
		t.Fatalf("got = %v, want [keep-me] (non-user_input categories excluded)", got)
	}
}

func TestProjectConsoleUserInputs_LimitCapsToMostRecent(t *testing.T) {
	t.Parallel()
	database, r := setupProjectConsoleUserInputsDB(t)
	insertConsoleChatSession(t, database, "cc-limit", "proj-hist")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const total = 120
	for i := 0; i < total; i++ {
		insertConsoleMessage(t, database, "cc-limit", i, "user_input", label(i), base.Add(time.Duration(i)*time.Second))
	}

	got, err := r.ProjectConsoleUserInputs("proj-hist", 500) // >100 clamps to 100
	if err != nil {
		t.Fatalf("ProjectConsoleUserInputs: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("len(got) = %d, want 100 (clamped)", len(got))
	}
	if got[0] != label(total-100) {
		t.Errorf("got[0] = %q, want %q (oldest of the last 100)", got[0], label(total-100))
	}
	if got[len(got)-1] != label(total-1) {
		t.Errorf("got[last] = %q, want %q (most recent)", got[len(got)-1], label(total-1))
	}
}

func TestProjectConsoleUserInputs_ConsecutiveDedup(t *testing.T) {
	t.Parallel()
	database, r := setupProjectConsoleUserInputsDB(t)
	insertConsoleChatSession(t, database, "cc-dedup", "proj-hist")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	insertConsoleMessage(t, database, "cc-dedup", 0, "user_input", "a", base)
	insertConsoleMessage(t, database, "cc-dedup", 1, "user_input", "a", base.Add(time.Minute))
	insertConsoleMessage(t, database, "cc-dedup", 2, "user_input", "b", base.Add(2*time.Minute))

	got, err := r.ProjectConsoleUserInputs("proj-hist", 100)
	if err != nil {
		t.Fatalf("ProjectConsoleUserInputs: %v", err)
	}
	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Fatalf("got = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestProjectConsoleUserInputs_ZeroOrNegativeLimitDefaultsTo100(t *testing.T) {
	t.Parallel()
	database, r := setupProjectConsoleUserInputsDB(t)
	insertConsoleChatSession(t, database, "cc-zero", "proj-hist")
	insertConsoleMessage(t, database, "cc-zero", 0, "user_input", "only", time.Now().UTC())

	for _, limit := range []int{0, -1} {
		got, err := r.ProjectConsoleUserInputs("proj-hist", limit)
		if err != nil {
			t.Fatalf("ProjectConsoleUserInputs(limit=%d): %v", limit, err)
		}
		if len(got) != 1 || got[0] != "only" {
			t.Errorf("ProjectConsoleUserInputs(limit=%d) = %v, want [only]", limit, got)
		}
	}
}

func TestProjectConsoleUserInputs_UnknownProject_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	_, r := setupProjectConsoleUserInputsDB(t)

	got, err := r.ProjectConsoleUserInputs("no-such-project", 100)
	if err != nil {
		t.Fatalf("ProjectConsoleUserInputs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %v, want empty for unknown project", got)
	}
}
