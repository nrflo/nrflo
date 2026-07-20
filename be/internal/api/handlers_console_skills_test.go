package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"be/internal/model"
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

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleListConsoleSkills)))
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

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleListConsoleSkills)))
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

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleListConsoleSkills)))
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

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleListConsoleSkills)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/skills?project=no-such-project", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}
