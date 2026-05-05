package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Get ---

func TestHandleGetPythonScript_NotFound(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts/ps-missing?project="+projectID, nil)
	req.SetPathValue("id", "ps-missing")
	rr := httptest.NewRecorder()
	s.handleGetPythonScript(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleGetPythonScript_MissingProject(t *testing.T) {
	s, _ := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts/ps-x", nil)
	req.SetPathValue("id", "ps-x")
	rr := httptest.NewRecorder()
	s.handleGetPythonScript(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleGetPythonScript_CrossProject(t *testing.T) {
	s, projectID := newPythonScriptServer(t)

	if _, err := s.pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-other', 'Other', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed other project: %v", err)
	}
	created := createPythonScript(t, s, "proj-other", "Other Script", "x=1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts/"+created.ID+"?project="+projectID, nil)
	req.SetPathValue("id", created.ID)
	rr := httptest.NewRecorder()
	s.handleGetPythonScript(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-project get status = %d, want 404", rr.Code)
	}
}

func TestHandleGetPythonScript_Found(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	created := createPythonScript(t, s, projectID, "Found Script", "y=2")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts/"+created.ID+"?project="+projectID, nil)
	req.SetPathValue("id", created.ID)
	rr := httptest.NewRecorder()
	s.handleGetPythonScript(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got := decodePythonScript(t, rr)
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.Name != "Found Script" {
		t.Errorf("Name = %q, want Found Script", got.Name)
	}
}

// --- Update ---

func TestHandleUpdatePythonScript_NotFound(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/python-scripts/ps-missing?project="+projectID, strings.NewReader(`{"name":"X"}`))
	req.SetPathValue("id", "ps-missing")
	rr := httptest.NewRecorder()
	s.handleUpdatePythonScript(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleUpdatePythonScript_EmptyName(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	created := createPythonScript(t, s, projectID, "Script", "x=1")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/python-scripts/"+created.ID+"?project="+projectID, strings.NewReader(`{"name":""}`))
	req.SetPathValue("id", created.ID)
	rr := httptest.NewRecorder()
	s.handleUpdatePythonScript(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleUpdatePythonScript_Valid(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	created := createPythonScript(t, s, projectID, "Old Name", "x=1")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/python-scripts/"+created.ID+"?project="+projectID, strings.NewReader(`{"name":"New Name"}`))
	req.SetPathValue("id", created.ID)
	rr := httptest.NewRecorder()
	s.handleUpdatePythonScript(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdatePythonScript_InvalidJSON(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/python-scripts/ps-x?project="+projectID, strings.NewReader("bad"))
	req.SetPathValue("id", "ps-x")
	rr := httptest.NewRecorder()
	s.handleUpdatePythonScript(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Delete ---

func TestHandleDeletePythonScript_NotFound(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/python-scripts/ps-missing?project="+projectID, nil)
	req.SetPathValue("id", "ps-missing")
	rr := httptest.NewRecorder()
	s.handleDeletePythonScript(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleDeletePythonScript_Valid(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	created := createPythonScript(t, s, projectID, "ToDelete", "x=1")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/python-scripts/"+created.ID+"?project="+projectID, nil)
	req.SetPathValue("id", created.ID)
	rr := httptest.NewRecorder()
	s.handleDeletePythonScript(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts/"+created.ID+"?project="+projectID, nil)
	req2.SetPathValue("id", created.ID)
	rr2 := httptest.NewRecorder()
	s.handleGetPythonScript(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Errorf("after delete, Get() status = %d, want 404", rr2.Code)
	}
}

func TestHandleDeletePythonScript_MissingProject(t *testing.T) {
	s, _ := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/python-scripts/ps-x", nil)
	req.SetPathValue("id", "ps-x")
	rr := httptest.NewRecorder()
	s.handleDeletePythonScript(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Validate ---

func TestHandleValidatePythonScript_MissingCode(t *testing.T) {
	s, _ := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts/validate", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.handleValidatePythonScript(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "code")
}

func TestHandleValidatePythonScript_InvalidJSON(t *testing.T) {
	s, _ := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts/validate", strings.NewReader("not-json"))
	rr := httptest.NewRecorder()
	s.handleValidatePythonScript(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleValidatePythonScript_ReturnsOKField(t *testing.T) {
	s, _ := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts/validate", strings.NewReader(`{"code":"x = 1"}`))
	rr := httptest.NewRecorder()
	s.handleValidatePythonScript(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var result map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := result["ok"]; !ok {
		t.Error("response missing 'ok' field")
	}
}

func TestHandleValidatePythonScript_NoProjectRequired(t *testing.T) {
	// Validate does not require X-Project header.
	s, _ := newPythonScriptServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts/validate", strings.NewReader(`{"code":"x = 1"}`))
	rr := httptest.NewRecorder()
	s.handleValidatePythonScript(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("validate without project: status = %d, want 200", rr.Code)
	}
}
