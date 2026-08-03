package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- List with type filter ---

func TestHandleListDefaultTemplates_FilterByTypeAgent(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates?type=agent", nil)
	rr := httptest.NewRecorder()
	s.handleListDefaultTemplates(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	list := decodeDefaultTemplateList(t, rr)
	if len(list) != 6 {
		t.Fatalf("len = %d, want 6 (agent-type templates)", len(list))
	}
	for _, tmpl := range list {
		if tmpl.Type != "agent" {
			t.Errorf("template %q: Type = %q, want %q", tmpl.ID, tmpl.Type, "agent")
		}
	}
}

func TestHandleListDefaultTemplates_FilterByTypeInjectable(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates?type=injectable", nil)
	rr := httptest.NewRecorder()
	s.handleListDefaultTemplates(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	list := decodeDefaultTemplateList(t, rr)
	if len(list) != 19 {
		t.Fatalf("len = %d, want 19 (injectable-type templates)", len(list))
	}
	wantIDs := map[string]bool{
		"low-context": true, "callback": true, "user-instructions": true,
		"system-prompt-suffix": true, "finish-reminder": true, "system-prompt": true, "working-set": true,
		"api-system-prompt": true,
		"tier-t0-decider":   true, "tier-t1-executor": true, "tier-t2-extractor": true,
		"delegation-guidance": true, "tier-t0-bare": true, "crash-resume": true, "stepwise-guidance": true,
		"validation-failure": true, "timeout-restart": true,
		"workspace-live-tree": true, "workspace-worktree": true}
	for _, tmpl := range list {
		if tmpl.Type != "injectable" {
			t.Errorf("template %q: Type = %q, want %q", tmpl.ID, tmpl.Type, "injectable")
		}
		if !wantIDs[tmpl.ID] {
			t.Errorf("unexpected injectable ID: %q", tmpl.ID)
		}
	}
}

func TestHandleListDefaultTemplates_FilterByTypeEmpty(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates?type=nonexistent", nil)
	rr := httptest.NewRecorder()
	s.handleListDefaultTemplates(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	list := decodeDefaultTemplateList(t, rr)
	if len(list) != 0 {
		t.Errorf("len = %d, want 0 for nonexistent type", len(list))
	}
}

// Create/update/restore/delete type handling + JSON field presence live in
// handlers_default_template_type_crud_test.go (300-line split).
