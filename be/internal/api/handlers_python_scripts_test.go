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

func newPythonScriptServer(t *testing.T) (*Server, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "ps_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	projectID := "proj-ps-handler"
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, 'TestProject', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	return &Server{pool: pool, clock: clock.Real()}, projectID
}

func decodePythonScript(t *testing.T, rr *httptest.ResponseRecorder) *model.PythonScript {
	t.Helper()
	var script model.PythonScript
	if err := json.NewDecoder(rr.Body).Decode(&script); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return &script
}

func decodePythonScriptList(t *testing.T, rr *httptest.ResponseRecorder) []*model.PythonScript {
	t.Helper()
	var list []*model.PythonScript
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return list
}

func createPythonScript(t *testing.T, s *Server, projectID, name, code string) *model.PythonScript {
	t.Helper()
	body := `{"name":"` + name + `","code":"` + code + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project="+projectID, strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createPythonScript(%q) status = %d, want 201; body: %s", name, rr.Code, rr.Body.String())
	}
	return decodePythonScript(t, rr)
}

// --- List ---

func TestHandleListPythonScripts_MissingProject(t *testing.T) {
	s, _ := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts", nil)
	rr := httptest.NewRecorder()
	s.handleListPythonScripts(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleListPythonScripts_Empty(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts?project="+projectID, nil)
	rr := httptest.NewRecorder()
	s.handleListPythonScripts(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	list := decodePythonScriptList(t, rr)
	if len(list) != 0 {
		t.Errorf("list = %d items, want 0", len(list))
	}
}

func TestHandleListPythonScripts_ReturnsSorted(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	for _, name := range []string{"Zebra", "Alpha"} {
		createPythonScript(t, s, projectID, name, "")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts?project="+projectID, nil)
	rr := httptest.NewRecorder()
	s.handleListPythonScripts(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	list := decodePythonScriptList(t, rr)
	if len(list) != 2 {
		t.Fatalf("list = %d items, want 2", len(list))
	}
	if list[0].Name != "Alpha" {
		t.Errorf("list[0].Name = %q, want Alpha (ASC)", list[0].Name)
	}
}

// --- Create ---

func TestHandleCreatePythonScript_MissingProject(t *testing.T) {
	s, _ := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts", strings.NewReader(`{"name":"X"}`))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreatePythonScript_MissingName(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project="+projectID, strings.NewReader(`{"code":"x=1"}`))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "name")
}

func TestHandleCreatePythonScript_InvalidJSON(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project="+projectID, strings.NewReader("not-json"))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreatePythonScript_Valid(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	body := `{"name":"My Script","description":"desc","code":"print(1)"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project="+projectID, strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	script := decodePythonScript(t, rr)
	if script.ID == "" {
		t.Error("ID is empty")
	}
	if script.Name != "My Script" {
		t.Errorf("Name = %q, want My Script", script.Name)
	}
	if script.Description != "desc" {
		t.Errorf("Description = %q, want desc", script.Description)
	}
}
