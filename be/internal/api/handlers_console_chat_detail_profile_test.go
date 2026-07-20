package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
)

// TestHandleGetConsoleChat_Detail_ProfileField_ReflectsCreatedProfile
// verifies a t0-decider chat's profile name round-trips onto the detail
// response's "profile" field.
func TestHandleGetConsoleChat_Detail_ProfileField_ReflectsCreatedProfile(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-chat-detail-profile")
	adminID := createTestUser(t, s, "chat-detail-profile-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	createChain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleChat)))
	createReq := createChatReq("proj-chat-detail-profile", `{"engine":"claude","model":"","profile":"t0-decider"}`)
	createReq.AddCookie(cookie)
	createRR := httptest.NewRecorder()
	createChain.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body=%s", createRR.Code, createRR.Body.String())
	}
	var created map[string]string
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleGetConsoleChat)))
	req := getChatReq(created["session_id"])
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal detail response: %v", err)
	}
	if body["profile"] != "t0-decider" {
		t.Errorf("profile = %v, want t0-decider", body["profile"])
	}
}
