package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// --- handleResumeSessionProject tests ---

func TestHandleResumeSessionProject_MissingProjectID(t *testing.T) {
	s, _ := newResumeTestServer(t)
	// No path value "id" set → r.PathValue("id") returns "".
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects//workflow/resume-session",
		strings.NewReader(`{"session_id":"sess-1"}`))
	rr := httptest.NewRecorder()
	s.handleResumeSessionProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID required")
}

func TestHandleResumeSessionProject_MissingSessionID(t *testing.T) {
	s, _ := newResumeTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-rsp/workflow/resume-session",
		strings.NewReader(`{}`))
	req.SetPathValue("id", "proj-rsp")
	rr := httptest.NewRecorder()
	s.handleResumeSessionProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session_id is required")
}

func TestHandleResumeSessionProject_SessionNotFound(t *testing.T) {
	s, _ := newResumeTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-rsp/workflow/resume-session",
		strings.NewReader(`{"session_id":"no-such-session-rsp"}`))
	req.SetPathValue("id", "proj-rsp")
	rr := httptest.NewRecorder()
	s.handleResumeSessionProject(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleResumeSessionProject_NonClaudeSession(t *testing.T) {
	s, dbPath := newResumeTestServer(t)
	insertResumeTestSession(t, dbPath, "sess-nc-rsp", "proj-rsp-nc", model.AgentSessionCompleted, "codex:gpt-5.3-codex")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-rsp-nc/workflow/resume-session",
		strings.NewReader(`{"session_id":"sess-nc-rsp"}`))
	req.SetPathValue("id", "proj-rsp-nc")
	rr := httptest.NewRecorder()
	s.handleResumeSessionProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "does not support resume")
}

func TestHandleResumeSessionProject_HappyPath(t *testing.T) {
	s, dbPath := newResumeTestServer(t)
	insertResumeTestSession(t, dbPath, "sess-happy-rsp", "proj-rsp-happy", model.AgentSessionFailed, "claude:opus-4-7")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-rsp-happy/workflow/resume-session",
		strings.NewReader(`{"session_id":"sess-happy-rsp"}`))
	req.SetPathValue("id", "proj-rsp-happy")
	rr := httptest.NewRecorder()
	s.handleResumeSessionProject(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "interactive" {
		t.Errorf("status = %q, want interactive", resp["status"])
	}
	if resp["session_id"] != "sess-happy-rsp" {
		t.Errorf("session_id = %q, want sess-happy-rsp", resp["session_id"])
	}

	// Verify the DB status was updated to user_interactive.
	database, err := db.OpenPathExisting(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	r := repo.NewAgentSessionRepo(database, clock.Real())
	updated, err := r.Get("sess-happy-rsp")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updated.Status != model.AgentSessionUserInteractive {
		t.Errorf("session status = %q, want user_interactive", updated.Status)
	}

	assertResumeLaunchRegistered(t, s, "sess-happy-rsp")
}
