package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/console"
	"be/internal/model"
	"be/internal/repo"
)

// TestConsoleChatClose_KindGuard_RejectsConsoleAndWorkflowAgentIDs covers
// acceptance case 3: the chat close route must refuse a kind='console' or
// kind='workflow_agent' id (404, not touching the row), and vice versa for
// the existing /console/sessions/{sid}/close route.
func TestConsoleChatClose_KindGuard_RejectsConsoleAndWorkflowAgentIDs(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-kindguard")
	adminID := createTestUser(t, s, "chat-kg-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chatSID, _ := createChatSession(t, s, factory, "proj-chat-kindguard", cookie)
	consoleSID, _ := seedConsoleSession(t, s, "proj-chat-kindguard")
	seedWorkflowAgentForConsoleTest(t, s, "proj-chat-kindguard", "wf-agent-kg-1")

	chatCloseChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleChat)))
	for _, id := range []string{consoleSID, "wf-agent-kg-1"} {
		req := chatCloseReq(id)
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		chatCloseChain.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("POST /console/chats/%s/close = %d, want 404 (wrong kind)", id, rr.Code)
		}
	}

	consoleCloseChain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleCloseConsoleSession)))
	req := closeConsoleReq(chatSID)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	consoleCloseChain.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("POST /console/sessions/%s/close (a console_chat id) = %d, want 404", chatSID, rr.Code)
	}
}

// TestConsoleChatBearer_ToolCatalogue_MatchesConsoleProfile covers
// acceptance case 4: a chat session's bearer against GET
// /api/v1/console/tools returns the console catalogue and none of the
// session-bound lifecycle tools.
func TestConsoleChatBearer_ToolCatalogue_MatchesConsoleProfile(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-profile")
	adminID := createTestUser(t, s, "chat-profile-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-chat-profile", cookie)

	row, err := repo.NewAgentSessionRepo(s.pool, s.clock).GetConsoleChat(sid)
	if err != nil || row == nil {
		t.Fatalf("load session: row=%v err=%v", row, err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleTools)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, catalogueReq(row.SpawnToken.String))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var body catalogueResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	byName := make(map[string]bool, len(body.Tools))
	for _, tool := range body.Tools {
		byName[tool.Name] = true
	}

	for _, name := range append(append([]string{}, wantReusedBuiltinsForTest...), wantConsoleOnlyForTest...) {
		if !byName[name] {
			t.Errorf("chat bearer catalogue missing expected tool %q", name)
		}
	}
	for _, name := range nonGoalTools {
		if byName[name] {
			t.Errorf("chat bearer catalogue unexpectedly contains lifecycle tool %q", name)
		}
	}
}

// TestConsoleChatBearer_ToolCatalogue_T0DeciderProfile_IsRestricted verifies
// catalogueForSession (handlers_console_tools.go) actually restricts the
// HTTP-mediated tool routes for a profiled chat: a t0-decider chat's bearer
// must see exactly the profile's catalogue over GET /api/v1/console/tools,
// not the full console set that TestConsoleChatBearer_ToolCatalogue_
// MatchesConsoleProfile observes for a chat with no profile — otherwise a
// claude/codex t0-decider chat would regain fs/bash-adjacent and
// non-catalogued tools (e.g. workflow_wait, project_list) via the bridge
// path even though the in-process api-engine registry is restricted.
func TestConsoleChatBearer_ToolCatalogue_T0DeciderProfile_IsRestricted(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-t0-catalogue")
	adminID := createTestUser(t, s, "chat-t0-catalogue-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid := createT0DeciderChatSession(t, s, factory, "proj-chat-t0-catalogue", cookie)

	row, err := repo.NewAgentSessionRepo(s.pool, s.clock).GetConsoleChat(sid)
	if err != nil || row == nil {
		t.Fatalf("load session: row=%v err=%v", row, err)
	}
	if row.ConsoleProfile != "t0-decider" {
		t.Fatalf("row.ConsoleProfile = %q, want t0-decider", row.ConsoleProfile)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleTools)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, catalogueReq(row.SpawnToken.String))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var body catalogueResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	profile, err := console.ProfileByName("t0-decider")
	if err != nil {
		t.Fatalf("ProfileByName: %v", err)
	}
	if len(body.Tools) != len(profile.Catalogue) {
		t.Fatalf("len(catalogue) = %d, want %d (exactly the t0-decider allowlist)", len(body.Tools), len(profile.Catalogue))
	}
	byName := make(map[string]bool, len(body.Tools))
	for _, tool := range body.Tools {
		byName[tool.Name] = true
	}
	for _, name := range profile.Catalogue {
		if !byName[name] {
			t.Errorf("t0-decider chat bearer catalogue missing catalogued tool %q", name)
		}
	}
	for _, name := range append(append([]string{}, wantReusedBuiltinsForTest...), wantConsoleOnlyForTest...) {
		inCatalogue := false
		for _, c := range profile.Catalogue {
			if c == name {
				inCatalogue = true
			}
		}
		if !inCatalogue && byName[name] {
			t.Errorf("t0-decider chat bearer catalogue unexpectedly contains non-catalogued tool %q", name)
		}
	}
}

// TestConsoleChatRoutes_NonAdminHuman_Returns403OnCreate and
// TestConsoleChatRoutes_MismatchedServiceToken_Returns403 cover acceptance
// case 6: a non-admin user / mismatched-project token is denied.
func TestConsoleChatRoutes_MismatchedServiceToken_Returns403(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-svc-b")
	_, plain := seedServiceToken(t, s, "proj-chat-svc-a", "ci")

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq("proj-chat-svc-b", `{"engine":"codex"}`)
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestConsoleChatRoutes_MatchingServiceToken_Returns201(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-svc-match")
	_, plain := seedServiceToken(t, s, "proj-chat-svc-match", "ci")

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	req := createChatReq("proj-chat-svc-match", `{"engine":"codex"}`)
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	if factory.last() == nil {
		t.Error("no engine constructed for a service-token-authorized create")
	}
}
