package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Tests for readonly template editing and the Restore endpoint (migration 000050 feature).

func TestHandleUpdateDefaultTemplate_ReadonlyTemplateTextAllowed(t *testing.T) {
	s := newDefaultTemplateServer(t)

	// Capture original metadata before edit.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/implementor", nil)
	getReq.SetPathValue("id", "implementor")
	getRR := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR, getReq)
	original := decodeDefaultTemplate(t, getRR)

	// PATCH template field only — was 403 before migration 000050, now must return 200.
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/implementor",
		strings.NewReader(`{"template":"custom implementor prompt"}`))
	req.SetPathValue("id", "implementor")
	rr := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for readonly template text update; body: %s", rr.Code, rr.Body.String())
	}

	// Verify persisted.
	getReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/implementor", nil)
	getReq2.SetPathValue("id", "implementor")
	getRR2 := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR2, getReq2)
	got := decodeDefaultTemplate(t, getRR2)

	if got.Template != "custom implementor prompt" {
		t.Errorf("Template = %q, want %q", got.Template, "custom implementor prompt")
	}
	if got.Name != original.Name {
		t.Errorf("Name = %q after template-only update, want %q (unchanged)", got.Name, original.Name)
	}
	if !got.Readonly {
		t.Error("Readonly = false after update, want true")
	}
}

func TestHandleRestoreDefaultTemplate_Valid(t *testing.T) {
	s := newDefaultTemplateServer(t)

	// Capture original default_template value from seeded readonly.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/setup-analyzer", nil)
	getReq.SetPathValue("id", "setup-analyzer")
	getRR := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR, getReq)
	original := decodeDefaultTemplate(t, getRR)
	if original.DefaultTemplate == nil {
		t.Fatal("setup-analyzer DefaultTemplate is nil — migration 000050 may not have run")
	}
	originalDefault := *original.DefaultTemplate

	// Edit the template text.
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/default-templates/setup-analyzer",
		strings.NewReader(`{"template":"modified text"}`))
	patchReq.SetPathValue("id", "setup-analyzer")
	patchRR := httptest.NewRecorder()
	s.handleUpdateDefaultTemplate(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body: %s", patchRR.Code, patchRR.Body.String())
	}

	// Restore.
	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates/setup-analyzer/restore", nil)
	restoreReq.SetPathValue("id", "setup-analyzer")
	restoreRR := httptest.NewRecorder()
	s.handleRestoreDefaultTemplate(restoreRR, restoreReq)

	if restoreRR.Code != http.StatusOK {
		t.Errorf("restore status = %d, want 200; body: %s", restoreRR.Code, restoreRR.Body.String())
	}

	// Verify template reset to original value.
	getReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/setup-analyzer", nil)
	getReq2.SetPathValue("id", "setup-analyzer")
	getRR2 := httptest.NewRecorder()
	s.handleGetDefaultTemplate(getRR2, getReq2)
	restored := decodeDefaultTemplate(t, getRR2)

	if restored.Template != originalDefault {
		t.Errorf("Template after restore = %q, want original %q", restored.Template, originalDefault)
	}
}

func TestHandleRestoreDefaultTemplate_NonReadonly(t *testing.T) {
	s := newDefaultTemplateServer(t)
	createUserDefaultTemplate(t, s, "restore-user", "Restore User", "user content")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates/restore-user/restore", nil)
	req.SetPathValue("id", "restore-user")
	rr := httptest.NewRecorder()
	s.handleRestoreDefaultTemplate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for non-readonly restore", rr.Code)
	}
	assertErrorContains(t, rr, "non-readonly")
}

func TestHandleRestoreDefaultTemplate_NotFound(t *testing.T) {
	s := newDefaultTemplateServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates/no-such/restore", nil)
	req.SetPathValue("id", "no-such")
	rr := httptest.NewRecorder()
	s.handleRestoreDefaultTemplate(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "not found")
}

func TestHandleListDefaultTemplates_ReadonlyDefaultTemplateField(t *testing.T) {
	s := newDefaultTemplateServer(t)
	createUserDefaultTemplate(t, s, "no-default-user", "No Default", "user content")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates", nil)
	rr := httptest.NewRecorder()
	s.handleListDefaultTemplates(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	list := decodeDefaultTemplateList(t, rr)
	for _, tmpl := range list {
		if tmpl.Readonly && tmpl.DefaultTemplate == nil {
			t.Errorf("readonly template %q: DefaultTemplate is nil, want non-nil", tmpl.ID)
		}
		if !tmpl.Readonly && tmpl.DefaultTemplate != nil {
			t.Errorf("non-readonly template %q: DefaultTemplate = %q, want nil", tmpl.ID, *tmpl.DefaultTemplate)
		}
	}
}

func TestHandleGetDefaultTemplate_ReadonlyDefaultTemplateField(t *testing.T) {
	s := newDefaultTemplateServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/doc-updater", nil)
	req.SetPathValue("id", "doc-updater")
	rr := httptest.NewRecorder()
	s.handleGetDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	tmpl := decodeDefaultTemplate(t, rr)
	if !tmpl.Readonly {
		t.Error("Readonly = false, want true for seeded template")
	}
	if tmpl.DefaultTemplate == nil {
		t.Fatal("DefaultTemplate = nil, want non-nil for seeded readonly template")
	}
	if *tmpl.DefaultTemplate != tmpl.Template {
		t.Errorf("DefaultTemplate %q != Template %q before any edit", *tmpl.DefaultTemplate, tmpl.Template)
	}
}

func TestHandleGetDefaultTemplate_UserCreatedNoDefaultTemplateField(t *testing.T) {
	s := newDefaultTemplateServer(t)
	createUserDefaultTemplate(t, s, "no-def-user", "No Def User", "user template text")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates/no-def-user", nil)
	req.SetPathValue("id", "no-def-user")
	rr := httptest.NewRecorder()
	s.handleGetDefaultTemplate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	tmpl := decodeDefaultTemplate(t, rr)
	if tmpl.DefaultTemplate != nil {
		t.Errorf("DefaultTemplate = %q, want nil for user-created template", *tmpl.DefaultTemplate)
	}
}
