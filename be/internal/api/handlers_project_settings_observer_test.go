package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doObserverRequest sends a GET or PUT /api/v1/projects/{id}/settings/observer request.
func doObserverRequest(t *testing.T, s *Server, method, projectID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/projects/"+projectID+"/settings/observer", strings.NewReader(body))
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()
	if method == http.MethodGet {
		s.handleGetProjectObserver(rr, req)
	} else {
		s.handlePutProjectObserver(rr, req)
	}
	return rr
}

func decodeObserverResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("decode observer response: %v", err)
	}
	return m
}

func TestHandleGetProjectObserver_Defaults(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doObserverRequest(t, s, http.MethodGet, projectID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	m := decodeObserverResponse(t, rr)
	if v := m["system_context"]; v != "" {
		t.Errorf("system_context = %v, want empty", v)
	}
	if v := m["provider"]; v != "" {
		t.Errorf("provider = %v, want empty", v)
	}
	if v := m["model"]; v != "" {
		t.Errorf("model = %v, want empty", v)
	}
}

func TestHandlePutProjectObserver_RoundTrip(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doObserverRequest(t, s, http.MethodPut, projectID,
		`{"system_context":"watch errors","provider":"claude","model":"sonnet"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	m := decodeObserverResponse(t, rr)
	if v := m["system_context"]; v != "watch errors" {
		t.Errorf("system_context = %v, want watch errors", v)
	}
	if v := m["provider"]; v != "claude" {
		t.Errorf("provider = %v, want claude", v)
	}
	if v := m["model"]; v != "sonnet" {
		t.Errorf("model = %v, want sonnet", v)
	}

	// GET should return same values.
	getRR := doObserverRequest(t, s, http.MethodGet, projectID, "")
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", getRR.Code)
	}
	m2 := decodeObserverResponse(t, getRR)
	for _, key := range []string{"system_context", "provider", "model"} {
		if m2[key] != m[key] {
			t.Errorf("GET %s = %v, want %v (should match PUT response)", key, m2[key], m[key])
		}
	}
}

func TestHandlePutProjectObserver_PartialUpdate(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	// Set all three.
	doObserverRequest(t, s, http.MethodPut, projectID,
		`{"system_context":"ctx","provider":"claude","model":"opus"}`)

	// Update only provider — other fields should be preserved.
	rr := doObserverRequest(t, s, http.MethodPut, projectID, `{"provider":"openai"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("partial PUT status = %d, want 200", rr.Code)
	}

	getRR := doObserverRequest(t, s, http.MethodGet, projectID, "")
	m := decodeObserverResponse(t, getRR)
	if m["system_context"] != "ctx" {
		t.Errorf("system_context = %v, want ctx (unchanged)", m["system_context"])
	}
	if m["provider"] != "openai" {
		t.Errorf("provider = %v, want openai (updated)", m["provider"])
	}
	if m["model"] != "opus" {
		t.Errorf("model = %v, want opus (unchanged)", m["model"])
	}
}

func TestHandleGetProjectObserver_MissingProjectID(t *testing.T) {
	t.Parallel()
	s, _ := newProjectSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects//settings/observer", nil)
	// No SetPathValue — path value "id" is empty string.
	rr := httptest.NewRecorder()
	s.handleGetProjectObserver(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when project id is missing", rr.Code)
	}
}

func TestHandlePutProjectObserver_MissingProjectID(t *testing.T) {
	t.Parallel()
	s, _ := newProjectSettingsServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects//settings/observer",
		strings.NewReader(`{"provider":"claude"}`))
	rr := httptest.NewRecorder()
	s.handlePutProjectObserver(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when project id is missing", rr.Code)
	}
}

func TestHandlePutProjectObserver_BadBody(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doObserverRequest(t, s, http.MethodPut, projectID, `{not valid json`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for malformed JSON", rr.Code)
	}
}

func TestHandlePutProjectObserver_ProjectsAreIsolated(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	// Seed a second project.
	if _, err := s.pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-b', 'B', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed second project: %v", err)
	}

	doObserverRequest(t, s, http.MethodPut, projectID, `{"system_context":"proj-a-ctx"}`)
	doObserverRequest(t, s, http.MethodPut, "proj-b", `{"system_context":"proj-b-ctx"}`)

	rrA := doObserverRequest(t, s, http.MethodGet, projectID, "")
	rrB := doObserverRequest(t, s, http.MethodGet, "proj-b", "")

	mA := decodeObserverResponse(t, rrA)
	mB := decodeObserverResponse(t, rrB)

	if mA["system_context"] != "proj-a-ctx" {
		t.Errorf("proj A system_context = %v, want proj-a-ctx", mA["system_context"])
	}
	if mB["system_context"] != "proj-b-ctx" {
		t.Errorf("proj B system_context = %v, want proj-b-ctx", mB["system_context"])
	}
}
