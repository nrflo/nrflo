package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// seedConsoleHistoryMessage writes one agent_messages row directly against
// sid, mirroring repo's insertConsoleMessage test helper for handler-level
// history tests.
func seedConsoleHistoryMessage(t *testing.T, s *Server, sid string, seq int, category, content string, createdAt time.Time) {
	t.Helper()
	if _, err := s.pool.Exec(`INSERT INTO agent_messages (session_id, seq, content, category, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		sid, seq, content, category, createdAt.UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed message: %v", err)
	}
}

func TestHandleGetConsoleHistory_AdminCookie_ReturnsAggregatedMessages(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-console-history")
	adminID := createTestUser(t, s, "history-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	sid1, _ := createChatSession(t, s, factory, "proj-console-history", cookie)
	sid2, _ := createChatSession(t, s, factory, "proj-console-history", cookie)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedConsoleHistoryMessage(t, s, sid1, 100, "user_input", "first", base)
	seedConsoleHistoryMessage(t, s, sid2, 100, "user_input", "second", base.Add(time.Minute))
	seedConsoleHistoryMessage(t, s, sid1, 101, "text", "ignored assistant reply", base.Add(2*time.Minute))

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleHistory)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/history?project=proj-console-history", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body types.ConsoleHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.ProjectID != "proj-console-history" {
		t.Errorf("project_id = %q, want proj-console-history", body.ProjectID)
	}
	want := []string{"first", "second"}
	if len(body.Messages) != len(want) {
		t.Fatalf("messages = %v, want %v", body.Messages, want)
	}
	for i := range want {
		if body.Messages[i] != want[i] {
			t.Errorf("messages[%d] = %q, want %q", i, body.Messages[i], want[i])
		}
	}
}

func TestHandleGetConsoleHistory_MissingProject_Returns400(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "history-admin2@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleHistory)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/history", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleGetConsoleHistory_UnknownProject_Returns404(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "history-admin3@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleHistory)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/history?project=no-such-project", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleGetConsoleHistory_NonAdminUser_Returns403(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-history-viewer")
	viewerID := createTestUser(t, s, "history-viewer@test.com", model.UserRoleViewer, false)
	cookie := injectSession(t, s, viewerID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleHistory)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/history?project=proj-history-viewer", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleGetConsoleHistory_ChatBearer_PinnedToSessionProject verifies a
// console_chat bearer is pinned unconditionally to its own session's
// project: a ?project= override pointing at a different project (with its
// own distinct history) must be ignored.
func TestHandleGetConsoleHistory_ChatBearer_PinnedToSessionProject(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-history-chat-a")
	seedConsoleProject(t, s, "proj-history-chat-b")
	adminID := createTestUser(t, s, "history-chat-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	sidA, _ := createChatSession(t, s, factory, "proj-history-chat-a", cookie)
	sidB, _ := createChatSession(t, s, factory, "proj-history-chat-b", cookie)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedConsoleHistoryMessage(t, s, sidA, 100, "user_input", "from-a", base)
	seedConsoleHistoryMessage(t, s, sidB, 100, "user_input", "from-b", base)

	row, err := repo.NewAgentSessionRepo(s.pool, s.clock).GetConsoleChat(sidA)
	if err != nil || row == nil {
		t.Fatalf("load session: row=%v err=%v", row, err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleHistory)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/history?project=proj-history-chat-b", nil)
	req.Header.Set("Authorization", "Bearer "+row.SpawnToken.String)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body types.ConsoleHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.ProjectID != "proj-history-chat-a" {
		t.Errorf("project_id = %q, want proj-history-chat-a (session project, ?project= override ignored)", body.ProjectID)
	}
	if len(body.Messages) != 1 || body.Messages[0] != "from-a" {
		t.Fatalf("messages = %v, want [from-a], not proj-history-chat-b's row", body.Messages)
	}
}

// TestHandleGetConsoleHistory_LimitClamp verifies an out-of-range ?limit is
// clamped to the default (100) instead of being applied literally.
func TestHandleGetConsoleHistory_LimitClamp(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-history-limit")
	adminID := createTestUser(t, s, "history-admin-limit@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-history-limit", cookie)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const total = 120
	for i := 0; i < total; i++ {
		seedConsoleHistoryMessage(t, s, sid, 100+i, "user_input", label(i), base.Add(time.Duration(i)*time.Second))
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleHistory)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/history?project=proj-history-limit&limit=99999", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body types.ConsoleHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Messages) != 100 {
		t.Fatalf("len(messages) = %d, want 100 (out-of-range limit clamped to default)", len(body.Messages))
	}
}

func label(i int) string {
	return fmt.Sprintf("msg-%d", i)
}
