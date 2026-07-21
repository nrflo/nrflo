package console

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/service"
)

// insertChatMessage writes one agent_messages row directly against the
// shared pool (bypassing seq auto-assignment), mirroring repo's
// insertConsoleMessage test helper.
func insertChatMessage(t *testing.T, pool *db.Pool, sessionID string, seq int, category, content string, createdAt time.Time) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO agent_messages (session_id, seq, content, category, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		sessionID, seq, content, category, createdAt.UTC().Format(time.RFC3339Nano))
}

// TestChatService_ProjectHistory_UnknownProject_ReturnsSentinel mirrors
// TestChatService_ListSkills_UnknownProject_ReturnsSentinel.
func TestChatService_ProjectHistory_UnknownProject_ReturnsSentinel(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	_, err := svc.ProjectHistory("no-such-project", 100)
	if !errors.Is(err, service.ErrConsoleProjectNotFound) {
		t.Errorf("ProjectHistory(unknown) = %v, want service.ErrConsoleProjectNotFound", err)
	}
}

// TestChatService_ProjectHistory_KnownProject_NoMessages_ReturnsEmpty covers a
// known project with no console_chat sessions/messages yet.
func TestChatService_ProjectHistory_KnownProject_NoMessages_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	got, err := svc.ProjectHistory(chatTestProjectID, 100)
	if err != nil {
		t.Fatalf("ProjectHistory: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ProjectHistory(no messages) = %v, want empty", got)
	}
}

// TestChatService_ProjectHistory_AggregatesUserInputAcrossSessions seeds two
// console_chat sessions (via svc.Create so the FK graph/kind matches
// production exactly) and asserts ProjectHistory returns their user_input
// contents oldest->newest, with non-user_input categories filtered by the
// repo layer underneath.
func TestChatService_ProjectHistory_AggregatesUserInputAcrossSessions(t *testing.T) {
	t.Parallel()
	svc, pool, _, _ := newChatTestService(t)

	sid1, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	sid2, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	insertChatMessage(t, pool, sid1, 0, "user_input", "hello", base)
	insertChatMessage(t, pool, sid2, 0, "user_input", "world", base.Add(time.Minute))
	insertChatMessage(t, pool, sid1, 1, "text", "assistant reply, ignored", base.Add(2*time.Minute))

	got, err := svc.ProjectHistory(chatTestProjectID, 100)
	if err != nil {
		t.Fatalf("ProjectHistory: %v", err)
	}
	want := []string{"hello", "world"}
	if len(got) != len(want) {
		t.Fatalf("got = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestChatService_ProjectHistory_LimitClamp asserts a limit>100 request is
// clamped to the most recent 100 by the underlying repo call.
func TestChatService_ProjectHistory_LimitClamp(t *testing.T) {
	t.Parallel()
	svc, pool, _, _ := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const total = 120
	for i := 0; i < total; i++ {
		insertChatMessage(t, pool, sid, i, "user_input", label(i), base.Add(time.Duration(i)*time.Second))
	}

	got, err := svc.ProjectHistory(chatTestProjectID, 500)
	if err != nil {
		t.Fatalf("ProjectHistory: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("len(got) = %d, want 100 (clamped)", len(got))
	}
	if got[0] != label(total-100) || got[len(got)-1] != label(total-1) {
		t.Errorf("got = [%q..%q], want [%q..%q]", got[0], got[len(got)-1], label(total-100), label(total-1))
	}
}

func label(i int) string {
	return fmt.Sprintf("msg-%d", i)
}
