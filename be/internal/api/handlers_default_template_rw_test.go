package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Get ---

func TestHandleGetDefaultTemplate_Valid(t *testing.T) {
	s := newDefaultTemplateServer(t)
	createUserDefaultTemplate(t, s, "get-me", "Get Me", "get content")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/get-me", nil)
	req.SetPathValue("id", "get-me")
	rr := httptest.NewRecorder()
	s.handleGetDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	tmpl := decodeDefaultTemplate(t, rr)
	if tmpl.ID != "get-me" {
		t.Errorf("ID = %q, want %q", tmpl.ID, "get-me")
	}
	if tmpl.Readonly {
		t.Errorf("Readonly = true, want false for user-created template")
	}
}

func TestHandleGetDefaultTemplate_ReadonlySeeded(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/implementor", nil)
	req.SetPathValue("id", "implementor")
	rr := httptest.NewRecorder()
	s.handleGetDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	tmpl := decodeDefaultTemplate(t, rr)
	if !tmpl.Readonly {
		t.Errorf("Readonly = false, want true for seeded template")
	}
	if tmpl.Name == "" {
		t.Errorf("Name is empty, want non-empty for seeded template")
	}
}

func TestHandleGetDefaultTemplate_NotFound(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/no-such", nil)
	req.SetPathValue("id", "no-such")
	rr := httptest.NewRecorder()
	s.handleGetDefaultTemplate(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "not found")
}

// --- Update ---

func TestHandleUpdateDefaultTemplate_UserCreated(t *testing.T) {
	s := newDefaultTemplateServer(t)
	createUserDefaultTemplate(t, s, "upd-tmpl", "Upd", "original content")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/upd-tmpl",
		strings.NewReader(`{"name":"Updated Name","template":"new content"}`))
	req.SetPathValue("id", "upd-tmpl")
	rr := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// Verify persisted.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/upd-tmpl", nil)
	getReq.SetPathValue("id", "upd-tmpl")
	getRR := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR, getReq)
	got := decodeDefaultTemplate(t, getRR)
	if got.Name != "Updated Name" {
		t.Errorf("after update Name = %q, want %q", got.Name, "Updated Name")
	}
	if got.Template != "new content" {
		t.Errorf("after update Template = %q, want %q", got.Template, "new content")
	}
}

func TestHandleUpdateDefaultTemplate_PartialUpdate(t *testing.T) {
	s := newDefaultTemplateServer(t)
	createUserDefaultTemplate(t, s, "partial-tmpl", "Original", "original template")

	// Update name only — template should be unchanged.
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/partial-tmpl",
		strings.NewReader(`{"name":"New Name"}`))
	req.SetPathValue("id", "partial-tmpl")
	rr := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/partial-tmpl", nil)
	getReq.SetPathValue("id", "partial-tmpl")
	getRR := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR, getReq)
	got := decodeDefaultTemplate(t, getRR)
	if got.Name != "New Name" {
		t.Errorf("Name = %q, want %q", got.Name, "New Name")
	}
	if got.Template != "original template" {
		t.Errorf("Template = %q, want %q (unchanged)", got.Template, "original template")
	}
}

func TestHandleUpdateDefaultTemplate_Readonly(t *testing.T) {
	s := newDefaultTemplateServer(t)
	// "implementor" is a migration-seeded readonly template.
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/implementor",
		strings.NewReader(`{"name":"Hacked"}`))
	req.SetPathValue("id", "implementor")
	rr := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for readonly template name update", rr.Code)
	}
}

func TestHandleUpdateDefaultTemplate_NotFound(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/no-such",
		strings.NewReader(`{"name":"X"}`))
	req.SetPathValue("id", "no-such")
	rr := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "not found")
}

func TestHandleUpdateDefaultTemplate_InvalidJSON(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/x",
		strings.NewReader("bad json"))
	req.SetPathValue("id", "x")
	rr := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Delete ---

func TestHandleDeleteDefaultTemplate_UserCreated(t *testing.T) {
	s := newDefaultTemplateServer(t)
	createUserDefaultTemplate(t, s, "del-tmpl", "Del", "content")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/default-templates/del-tmpl", nil)
	req.SetPathValue("id", "del-tmpl")
	rr := httptest.NewRecorder()
	s.handleDeleteDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// Subsequent GET returns 404.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/del-tmpl", nil)
	getReq.SetPathValue("id", "del-tmpl")
	getRR := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR, getReq)
	if getRR.Code != http.StatusNotFound {
		t.Errorf("post-delete GET status = %d, want 404", getRR.Code)
	}
}

func TestHandleDeleteDefaultTemplate_Readonly(t *testing.T) {
	s := newDefaultTemplateServer(t)
	// "setup-analyzer" is a migration-seeded readonly template.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/default-templates/setup-analyzer", nil)
	req.SetPathValue("id", "setup-analyzer")
	rr := httptest.NewRecorder()
	s.handleDeleteDefaultTemplate(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for readonly template delete", rr.Code)
	}
}

func TestHandleDeleteDefaultTemplate_NotFound(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/default-templates/no-such", nil)
	req.SetPathValue("id", "no-such")
	rr := httptest.NewRecorder()
	s.handleDeleteDefaultTemplate(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "not found")
}

// --- Full CRUD flow ---

func TestHandleDefaultTemplate_FullCRUDFlow(t *testing.T) {
	s := newDefaultTemplateServer(t)

	// 1. List — 6 pre-seeded readonly.
	listRR := httptest.NewRecorder()
	s.handleListDefaultTemplates(listRR, httptest.NewRequest(http.MethodGet, "/api/v1/default-templates", nil))
	if got := decodeDefaultTemplateList(t, listRR); len(got) != 6 {
		t.Fatalf("initial list len = %d, want 6", len(got))
	}

	// 2. Create.
	created := createUserDefaultTemplate(t, s, "flow-tmpl", "Flow Template", "flow content")
	if created.Readonly {
		t.Errorf("newly created Readonly = true, want false")
	}

	// 3. List — 7.
	listRR2 := httptest.NewRecorder()
	s.handleListDefaultTemplates(listRR2, httptest.NewRequest(http.MethodGet, "/api/v1/default-templates", nil))
	if got := decodeDefaultTemplateList(t, listRR2); len(got) != 7 {
		t.Fatalf("after create list len = %d, want 7", len(got))
	}

	// 4. Update.
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/flow-tmpl",
		strings.NewReader(`{"name":"Updated Flow"}`))
	patchReq.SetPathValue("id", "flow-tmpl")
	patchRR := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200", patchRR.Code)
	}

	// 5. Verify update persisted, template field unchanged.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/flow-tmpl", nil)
	getReq.SetPathValue("id", "flow-tmpl")
	getRR := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR, getReq)
	got := decodeDefaultTemplate(t, getRR)
	if got.Name != "Updated Flow" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated Flow")
	}
	if got.Template != "flow content" {
		t.Errorf("Template = %q, want unchanged %q", got.Template, "flow content")
	}

	// 6. Cannot update a readonly seeded template.
	roReq := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/qa-verifier",
		strings.NewReader(`{"name":"Hacked"}`))
	roReq.SetPathValue("id", "qa-verifier")
	roRR := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(roRR, roReq)
	if roRR.Code != http.StatusBadRequest {
		t.Errorf("readonly name update status = %d, want 400", roRR.Code)
	}

	// 7. Delete user template.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/default-templates/flow-tmpl", nil)
	delReq.SetPathValue("id", "flow-tmpl")
	delRR := httptest.NewRecorder()
	s.handleDeleteDefaultTemplate(delRR, delReq)
	if delRR.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", delRR.Code)
	}

	// 8. Back to 6.
	listRR3 := httptest.NewRecorder()
	s.handleListDefaultTemplates(listRR3, httptest.NewRequest(http.MethodGet, "/api/v1/default-templates", nil))
	if got := decodeDefaultTemplateList(t, listRR3); len(got) != 6 {
		t.Errorf("after delete list len = %d, want 6", len(got))
	}
}
