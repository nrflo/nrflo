package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// newDefaultTemplateServer creates a minimal Server for default template handler tests.
// Does NOT delete migration-seeded templates — tests verify the 6 pre-filled entries.
func newDefaultTemplateServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "default_template_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

func decodeDefaultTemplate(t *testing.T, rr *httptest.ResponseRecorder) *model.DefaultTemplate {
	t.Helper()
	var tmpl model.DefaultTemplate
	if err := json.NewDecoder(rr.Body).Decode(&tmpl); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return &tmpl
}

func decodeDefaultTemplateList(t *testing.T, rr *httptest.ResponseRecorder) []*model.DefaultTemplate {
	t.Helper()
	var list []*model.DefaultTemplate
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return list
}

// createUserDefaultTemplate POSTs a non-readonly template and asserts 201.
func createUserDefaultTemplate(t *testing.T, s *Server, id, name, tmplContent string) *model.DefaultTemplate {
	t.Helper()
	body := `{"id":"` + id + `","name":"` + name + `","template":"` + tmplContent + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateDefaultTemplate(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createUserDefaultTemplate(%q) status = %d, want 201; body: %s", id, rr.Code, rr.Body.String())
	}
	return decodeDefaultTemplate(t, rr)
}

// --- List ---

func TestHandleListDefaultTemplates_SixReadonly(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates", nil)
	rr := httptest.NewRecorder()
	s.handleListDefaultTemplates(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	list := decodeDefaultTemplateList(t, rr)
	if len(list) != 6 {
		t.Fatalf("len = %d, want 6 (pre-filled readonly templates)", len(list))
	}
	for _, tmpl := range list {
		if !tmpl.Readonly {
			t.Errorf("template %q: Readonly = false, want true", tmpl.ID)
		}
	}
	// Ordered by name ascending.
	wantOrder := []string{"doc-updater", "implementor", "qa-verifier", "setup-analyzer", "test-writer", "ticket-creator"}
	for i, want := range wantOrder {
		if list[i].ID != want {
			t.Errorf("list[%d].ID = %q, want %q", i, list[i].ID, want)
		}
	}
}

func TestHandleListDefaultTemplates_IncludesUserCreated(t *testing.T) {
	s := newDefaultTemplateServer(t)
	createUserDefaultTemplate(t, s, "my-custom", "My Custom", "custom content")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/default-templates", nil)
	rr := httptest.NewRecorder()
	s.handleListDefaultTemplates(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	list := decodeDefaultTemplateList(t, rr)
	if len(list) != 7 {
		t.Errorf("len = %d, want 7 (6 readonly + 1 user-created)", len(list))
	}
}

// --- Create ---

func TestHandleCreateDefaultTemplate_Valid(t *testing.T) {
	s := newDefaultTemplateServer(t)
	body := `{"id":"new-agent","name":"New Agent","template":"do stuff"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateDefaultTemplate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	tmpl := decodeDefaultTemplate(t, rr)
	if tmpl.ID != "new-agent" {
		t.Errorf("ID = %q, want %q", tmpl.ID, "new-agent")
	}
	if tmpl.Name != "New Agent" {
		t.Errorf("Name = %q, want %q", tmpl.Name, "New Agent")
	}
	if tmpl.Template != "do stuff" {
		t.Errorf("Template = %q, want %q", tmpl.Template, "do stuff")
	}
	if tmpl.Readonly {
		t.Errorf("Readonly = true, want false for user-created template")
	}
}

func TestHandleCreateDefaultTemplate_MissingFields(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing id", `{"name":"X","template":"t"}`, "id"},
		{"missing name", `{"id":"x","template":"t"}`, "name"},
		{"missing template", `{"id":"x","name":"X"}`, "template"},
		{"empty body", `{}`, "id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newDefaultTemplateServer(t)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()
			s.handleCreateDefaultTemplate(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rr.Code)
			}
			assertErrorContains(t, rr, tc.want)
		})
	}
}

func TestHandleCreateDefaultTemplate_InvalidJSON(t *testing.T) {
	s := newDefaultTemplateServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates", strings.NewReader("not-json"))
	rr := httptest.NewRecorder()
	s.handleCreateDefaultTemplate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateDefaultTemplate_Duplicate(t *testing.T) {
	s := newDefaultTemplateServer(t)
	createUserDefaultTemplate(t, s, "dup-tmpl", "Dup", "content")

	body := `{"id":"dup-tmpl","name":"Dup2","template":"content2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/default-templates", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateDefaultTemplate(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("duplicate create status = %d, want 409", rr.Code)
	}
	assertErrorContains(t, rr, "already exists")
}
