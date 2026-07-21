package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// writeConsoleSkill creates <root>/.claude/skills/<dir>/SKILL.md with the
// given frontmatter+body — mirrors console.skills_test.go's fixture shape.
func writeConsoleSkill(t *testing.T, root, dir, content string) {
	t.Helper()
	skillDir := filepath.Join(root, ".claude", "skills", dir)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", skillDir, err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func TestHandleListConsoleSkills_ReturnsDiscoveredSkills(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-console-skills")
	root := t.TempDir()
	writeConsoleSkill(t, root, "finalize", "---\nname: finalize\ndescription: \"Close out work.\"\n---\nBODY\n")
	if _, err := s.pool.Exec(`UPDATE projects SET root_path = ? WHERE id = ?`, root, "proj-console-skills"); err != nil {
		t.Fatalf("set root_path: %v", err)
	}
	adminID := createTestUser(t, s, "skills-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleSkills)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/skills?project=proj-console-skills", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body types.ConsoleSkillsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.ProjectID != "proj-console-skills" {
		t.Errorf("project_id = %q, want proj-console-skills", body.ProjectID)
	}
	if len(body.Skills) != 1 || body.Skills[0].Name != "finalize" {
		t.Fatalf("skills = %+v, want one skill named finalize", body.Skills)
	}
	if body.Skills[0].Description != "Close out work." {
		t.Errorf("description = %q, want %q", body.Skills[0].Description, "Close out work.")
	}
}

func TestHandleListConsoleSkills_NoRootPath_ReturnsEmptyList(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-console-skills-empty")
	adminID := createTestUser(t, s, "skills-admin2@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleSkills)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/skills?project=proj-console-skills-empty", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body types.ConsoleSkillsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Skills) != 0 {
		t.Errorf("skills = %+v, want empty (no root_path set)", body.Skills)
	}
}

func TestHandleListConsoleSkills_MissingProject_Returns400(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "skills-admin3@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleSkills)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/skills", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleListConsoleSkills_UnknownProject_Returns404(t *testing.T) {
	s, _ := newChatTestServer(t)
	adminID := createTestUser(t, s, "skills-admin4@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleSkills)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/skills?project=no-such-project", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleListConsoleSkills_ChatBearer_PinnedToSessionProject verifies a
// console_chat bearer is pinned unconditionally to its own session's
// project: a ?project= override pointing at a different project (with its
// own distinct skill set) must be ignored.
func TestHandleListConsoleSkills_ChatBearer_PinnedToSessionProject(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-skills-chat-a")
	rootA := t.TempDir()
	writeConsoleSkill(t, rootA, "finalize", "---\nname: finalize\ndescription: \"Close out work.\"\n---\nBODY\n")
	if _, err := s.pool.Exec(`UPDATE projects SET root_path = ? WHERE id = ?`, rootA, "proj-skills-chat-a"); err != nil {
		t.Fatalf("set root_path a: %v", err)
	}

	seedConsoleProject(t, s, "proj-skills-chat-b")
	rootB := t.TempDir()
	writeConsoleSkill(t, rootB, "other", "---\nname: other\ndescription: \"Other project skill.\"\n---\nBODY\n")
	if _, err := s.pool.Exec(`UPDATE projects SET root_path = ? WHERE id = ?`, rootB, "proj-skills-chat-b"); err != nil {
		t.Fatalf("set root_path b: %v", err)
	}

	adminID := createTestUser(t, s, "skills-chat-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-skills-chat-a", cookie)

	row, err := repo.NewAgentSessionRepo(s.pool, s.clock).GetConsoleChat(sid)
	if err != nil || row == nil {
		t.Fatalf("load session: row=%v err=%v", row, err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleSkills)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/skills?project=proj-skills-chat-b", nil)
	req.Header.Set("Authorization", "Bearer "+row.SpawnToken.String)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body types.ConsoleSkillsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.ProjectID != "proj-skills-chat-a" {
		t.Errorf("project_id = %q, want proj-skills-chat-a (session project, ?project= override ignored)", body.ProjectID)
	}
	if len(body.Skills) != 1 || body.Skills[0].Name != "finalize" {
		t.Fatalf("skills = %+v, want one skill named finalize (proj-skills-chat-a's), not proj-skills-chat-b's", body.Skills)
	}
}

// TestHandleListConsoleSkills_NonAdminUser_Returns403 verifies a non-admin
// human (viewer role) is rejected — the handler falls through the
// console-session and admin-user branches to the service-principal branch,
// which also fails for a cookie-authed request, landing on the final 403.
func TestHandleListConsoleSkills_NonAdminUser_Returns403(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-skills-viewer")
	viewerID := createTestUser(t, s, "skills-viewer@test.com", model.UserRoleViewer, false)
	cookie := injectSession(t, s, viewerID)

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleSkills)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/skills?project=proj-skills-viewer", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleListConsoleSkills_MismatchedServiceToken_Returns403 verifies a
// project-scoped service token cannot read another project's skills via a
// ?project= override — mirrors the console-chat mismatch shape
// (handlers_console_chat_isolation_test.go TestConsoleChatRoutes_
// MismatchedServiceToken_Returns403).
func TestHandleListConsoleSkills_MismatchedServiceToken_Returns403(t *testing.T) {
	s, _ := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-skills-svc-b")
	_, plain := seedServiceToken(t, s, "proj-skills-svc-a", "ci")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleSkills)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/skills?project=proj-skills-svc-b", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleListConsoleSkills_MatchingServiceToken_Returns200 is the
// symmetry case for the mismatch test above: a service token scoped to the
// requested project succeeds.
func TestHandleListConsoleSkills_MatchingServiceToken_Returns200(t *testing.T) {
	s, _ := newChatTestServer(t)
	_, plain := seedServiceToken(t, s, "proj-skills-svc-match", "ci")

	chain := s.sessionMgr.LoadAndSave(s.requireAuth(http.HandlerFunc(s.handleListConsoleSkills)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/skills?project=proj-skills-svc-match", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body types.ConsoleSkillsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.ProjectID != "proj-skills-svc-match" {
		t.Errorf("project_id = %q, want proj-skills-svc-match", body.ProjectID)
	}
}
