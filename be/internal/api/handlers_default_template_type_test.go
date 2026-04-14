package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if len(list) != 4 {
		t.Fatalf("len = %d, want 4 (injectable-type templates)", len(list))
	}
	wantIDs := map[string]bool{
		"continuation":      true,
		"low-context":       true,
		"callback":          true,
		"user-instructions": true,
	}
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

// --- Create with type ---

func TestHandleCreateDefaultTemplate_DefaultsTypeToAgent(t *testing.T) {
	s := newDefaultTemplateServer(t)
	body := `{"id":"no-type","name":"No Type","template":"content"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateDefaultTemplate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	tmpl := decodeDefaultTemplate(t, rr)
	if tmpl.Type != "agent" {
		t.Errorf("Type = %q, want %q (default)", tmpl.Type, "agent")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/no-type", nil)
	getReq.SetPathValue("id", "no-type")
	getRR := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR, getReq)
	got := decodeDefaultTemplate(t, getRR)
	if got.Type != "agent" {
		t.Errorf("persisted Type = %q, want %q", got.Type, "agent")
	}
}

func TestHandleCreateDefaultTemplate_InjectableType(t *testing.T) {
	s := newDefaultTemplateServer(t)
	body := `{"id":"new-inj","name":"New Injectable","template":"inj content","type":"injectable"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateDefaultTemplate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	tmpl := decodeDefaultTemplate(t, rr)
	if tmpl.Type != "injectable" {
		t.Errorf("Type = %q, want %q", tmpl.Type, "injectable")
	}
}

func TestHandleCreateDefaultTemplate_CustomType(t *testing.T) {
	s := newDefaultTemplateServer(t)
	body := `{"id":"macro-tmpl","name":"Macro","template":"macro body","type":"macro"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateDefaultTemplate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	tmpl := decodeDefaultTemplate(t, rr)
	if tmpl.Type != "macro" {
		t.Errorf("Type = %q, want %q", tmpl.Type, "macro")
	}
}

// --- Update type on readonly ---

func TestHandleUpdateDefaultTemplate_ReadonlyIgnoresTypeChange(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/implementor",
		strings.NewReader(`{"type":"custom"}`))
	req.SetPathValue("id", "implementor")
	rr := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (silently ignore type change)", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/implementor", nil)
	getReq.SetPathValue("id", "implementor")
	getRR := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR, getReq)
	got := decodeDefaultTemplate(t, getRR)
	if got.Type != "agent" {
		t.Errorf("Type = %q, want %q (should be unchanged)", got.Type, "agent")
	}
}

func TestHandleUpdateDefaultTemplate_ReadonlyInjectableIgnoresTypeChange(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/callback",
		strings.NewReader(`{"type":"agent"}`))
	req.SetPathValue("id", "callback")
	rr := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (silently ignore type change)", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/callback", nil)
	getReq.SetPathValue("id", "callback")
	getRR := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR, getReq)
	got := decodeDefaultTemplate(t, getRR)
	if got.Type != "injectable" {
		t.Errorf("Type = %q, want %q (should be unchanged)", got.Type, "injectable")
	}
}

// --- Update type on non-readonly ---

func TestHandleUpdateDefaultTemplate_NonReadonlyAllowsTypeChange(t *testing.T) {
	s := newDefaultTemplateServer(t)
	createUserDefaultTemplate(t, s, "type-change", "Type Change", "content")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/type-change",
		strings.NewReader(`{"type":"injectable"}`))
	req.SetPathValue("id", "type-change")
	rr := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/type-change", nil)
	getReq.SetPathValue("id", "type-change")
	getRR := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR, getReq)
	got := decodeDefaultTemplate(t, getRR)
	if got.Type != "injectable" {
		t.Errorf("Type = %q, want %q", got.Type, "injectable")
	}
}

// --- JSON response includes type field ---

func TestHandleGetDefaultTemplate_JSONIncludesTypeField(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/implementor", nil)
	req.SetPathValue("id", "implementor")
	rr := httptest.NewRecorder()
	s.handleGetDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	typeVal, ok := raw["type"]
	if !ok {
		t.Fatal("JSON response missing 'type' field")
	}
	if typeVal != "agent" {
		t.Errorf("type = %v, want %q", typeVal, "agent")
	}
}

func TestHandleGetDefaultTemplate_InjectableTypeInJSON(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/continuation", nil)
	req.SetPathValue("id", "continuation")
	rr := httptest.NewRecorder()
	s.handleGetDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if raw["type"] != "injectable" {
		t.Errorf("type = %v, want %q", raw["type"], "injectable")
	}
}

// --- Restore injectable via API ---

func TestHandleRestoreDefaultTemplate_InjectableReadonly(t *testing.T) {
	s := newDefaultTemplateServer(t)

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/user-instructions",
		strings.NewReader(`{"template":"modified text"}`))
	patchReq.SetPathValue("id", "user-instructions")
	patchRR := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200", patchRR.Code)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates/user-instructions/restore", nil)
	restoreReq.SetPathValue("id", "user-instructions")
	restoreRR := httptest.NewRecorder()
	s.handleRestoreDefaultTemplate(restoreRR, restoreReq)
	if restoreRR.Code != http.StatusOK {
		t.Fatalf("restore status = %d, want 200; body: %s", restoreRR.Code, restoreRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/user-instructions", nil)
	getReq.SetPathValue("id", "user-instructions")
	getRR := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR, getReq)
	got := decodeDefaultTemplate(t, getRR)
	if got.Template == "modified text" {
		t.Error("Template still shows modified text after restore")
	}
	if got.Type != "injectable" {
		t.Errorf("Type = %q, want %q after restore", got.Type, "injectable")
	}
}

// --- Delete injectable readonly rejected ---

func TestHandleDeleteDefaultTemplate_InjectableReadonly(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/default-templates/callback", nil)
	req.SetPathValue("id", "callback")
	rr := httptest.NewRecorder()
	s.handleDeleteDefaultTemplate(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for readonly injectable delete", rr.Code)
	}
}

// --- Filter preserves after CRUD ---

func TestHandleListDefaultTemplates_FilterAfterCRUD(t *testing.T) {
	s := newDefaultTemplateServer(t)

	body := `{"id":"api-inj","name":"API Injectable","template":"content","type":"injectable"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates", strings.NewReader(body))
	createRR := httptest.NewRecorder()
	s.handleCreateDefaultTemplate(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", createRR.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates?type=injectable", nil)
	listRR := httptest.NewRecorder()
	s.handleListDefaultTemplates(listRR, listReq)
	list := decodeDefaultTemplateList(t, listRR)
	if len(list) != 5 {
		t.Errorf("injectable count = %d, want 5 (4 seeded + 1 created)", len(list))
	}

	agentReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates?type=agent", nil)
	agentRR := httptest.NewRecorder()
	s.handleListDefaultTemplates(agentRR, agentReq)
	agentList := decodeDefaultTemplateList(t, agentRR)
	if len(agentList) != 6 {
		t.Errorf("agent count = %d, want 6 (unchanged)", len(agentList))
	}
}
