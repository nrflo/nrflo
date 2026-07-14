package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/ws"
)

func listChatsReq(project string) *http.Request {
	url := "/api/v1/console/chats"
	if project != "" {
		url += "?project=" + project
	}
	return httptest.NewRequest(http.MethodGet, url, nil)
}

// startTestHub runs s.wsHub's dispatch loop (only started by Server.Start in
// production, which these tests never call) so Register/Subscribe/
// BroadcastSession — all channel sends consumed by that loop — don't block
// forever.
func startTestHub(t *testing.T, s *Server) {
	t.Helper()
	go s.wsHub.Run()
	t.Cleanup(s.wsHub.Stop)
}

// waitForChatEngineExit subscribes to sid's session channel, stops eng, and
// blocks until console_chat.turn state=idle arrives — pumpChatEvents pushes
// that event LAST, after tearing the session down (removed from
// ChatService's map, row closed), so its arrival is the deterministic signal
// that ChatService.Live(sid) will now report false. No sleep, no polling.
func waitForChatEngineExit(t *testing.T, s *Server, sid string, eng *fakeConsoleEngine) {
	t.Helper()
	client, ch := ws.NewTestClient(s.wsHub, "exit-wait-"+sid)
	s.wsHub.Register(client)
	s.wsHub.SubscribeSession(client, sid)

	eng.Stop()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case raw := <-ch:
			var ev ws.Event
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatalf("unmarshal WS event: %v", err)
			}
			if ev.Type == ws.EventConsoleChatTurn && ev.Data["state"] == "idle" {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for engine exit teardown on session %q", sid)
		}
	}
}

// TestHandleListConsoleChats_ShapeAndLiveFlag asserts the list response shape
// (engine/model/status/started_at/live) and that live flips to false once the
// session's engine has exited — a hard-killed engine must not be offered as
// resumable.
func TestHandleListConsoleChats_ShapeAndLiveFlag(t *testing.T) {
	s, factory := newChatTestServer(t)
	startTestHub(t, s)
	seedConsoleProject(t, s, "proj-chat-list")
	adminID := createTestUser(t, s, "chat-list-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, eng := createChatSession(t, s, factory, "proj-chat-list", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleListConsoleChats)))
	req := listChatsReq("proj-chat-list")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var body struct {
		Sessions []consoleChatListItem `json:"sessions"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var item *consoleChatListItem
	for i := range body.Sessions {
		if body.Sessions[i].SessionID == sid {
			item = &body.Sessions[i]
		}
	}
	if item == nil {
		t.Fatalf("session %q not found in list response: %+v", sid, body.Sessions)
	}
	if item.Engine != "codex" {
		t.Errorf("Engine = %q, want codex", item.Engine)
	}
	if item.Status != string(model.AgentSessionUserInteractive) {
		t.Errorf("Status = %q, want user_interactive", item.Status)
	}
	if item.StartedAt == "" {
		t.Error("StartedAt is empty, want a timestamp")
	}
	if !item.Live {
		t.Error("Live = false, want true for a freshly created session")
	}

	waitForChatEngineExit(t, s, sid, eng)

	rr2 := httptest.NewRecorder()
	req2 := listChatsReq("proj-chat-list")
	req2.AddCookie(cookie)
	chain.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second list status = %d, want 200; body=%s", rr2.Code, rr2.Body.String())
	}
	var body2 struct {
		Sessions []consoleChatListItem `json:"sessions"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &body2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	item = nil
	for i := range body2.Sessions {
		if body2.Sessions[i].SessionID == sid {
			item = &body2.Sessions[i]
		}
	}
	if item == nil {
		t.Fatalf("session %q missing from list after engine exit: %+v", sid, body2.Sessions)
	}
	if item.Live {
		t.Error("Live = true after the engine exited, want false")
	}
}

func TestHandleListConsoleChats_NonAdminHuman_Returns403(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-list-403")
	userID := createTestUser(t, s, "chat-list-viewer@test.com", model.UserRoleViewer, false)
	cookie := injectSession(t, s, userID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleListConsoleChats)))
	req := listChatsReq("proj-chat-list-403")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleListConsoleChats_MissingProject_Returns400(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "chat-list-noproj@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleListConsoleChats)))
	req := listChatsReq("")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}
