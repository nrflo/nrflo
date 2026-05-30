package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/service"
)

func doCaptureThinkingRequest(t *testing.T, s *Server, method, projectID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/projects/"+projectID+"/settings/capture-thinking",
		strings.NewReader(body))
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()
	if method == http.MethodGet {
		s.handleGetProjectCaptureThinking(rr, req)
	} else {
		s.handlePutProjectCaptureThinking(rr, req)
	}
	return rr
}

func decodeCaptureThinkingResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("decode capture_thinking response: %v", err)
	}
	return m
}

func TestHandleGetProjectCaptureThinking_Default(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doCaptureThinkingRequest(t, s, http.MethodGet, projectID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	m := decodeCaptureThinkingResponse(t, rr)
	if m["enabled"] != false {
		t.Errorf("default enabled = %v, want false", m["enabled"])
	}
	if m["inherited"] != true {
		t.Errorf("default inherited = %v, want true", m["inherited"])
	}
}

func TestHandlePutProjectCaptureThinking_SetTrue(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doCaptureThinkingRequest(t, s, http.MethodPut, projectID, `{"enabled":true}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	m := decodeCaptureThinkingResponse(t, rr)
	if m["enabled"] != true {
		t.Errorf("PUT true → enabled = %v, want true", m["enabled"])
	}
	if m["inherited"] != false {
		t.Errorf("PUT true → inherited = %v, want false", m["inherited"])
	}

	// GET must return the same values.
	getRR := doCaptureThinkingRequest(t, s, http.MethodGet, projectID, "")
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", getRR.Code)
	}
	m2 := decodeCaptureThinkingResponse(t, getRR)
	if m2["enabled"] != true {
		t.Errorf("GET after PUT true: enabled = %v, want true", m2["enabled"])
	}
	if m2["inherited"] != false {
		t.Errorf("GET after PUT true: inherited = %v, want false", m2["inherited"])
	}
}

func TestHandlePutProjectCaptureThinking_SetFalse(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doCaptureThinkingRequest(t, s, http.MethodPut, projectID, `{"enabled":false}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	m := decodeCaptureThinkingResponse(t, rr)
	if m["enabled"] != false {
		t.Errorf("PUT false → enabled = %v, want false", m["enabled"])
	}
	if m["inherited"] != false {
		t.Errorf("PUT false → inherited = %v, want false (explicit, not inherited)", m["inherited"])
	}

	getRR := doCaptureThinkingRequest(t, s, http.MethodGet, projectID, "")
	m2 := decodeCaptureThinkingResponse(t, getRR)
	if m2["inherited"] != false {
		t.Errorf("GET after PUT false: inherited = %v, want false", m2["inherited"])
	}
}

func TestHandlePutProjectCaptureThinking_NullClearsToGlobal(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	// Establish a global value so we can verify fallback.
	globalSvc := service.NewGlobalSettingsService(s.pool, clock.Real())
	if err := globalSvc.SetCaptureThinkingEnabled(true); err != nil {
		t.Fatalf("SetCaptureThinkingEnabled(true): %v", err)
	}

	// Set a project-level override first.
	doCaptureThinkingRequest(t, s, http.MethodPut, projectID, `{"enabled":false}`)

	// Clear via null — should now inherit global=true.
	rr := doCaptureThinkingRequest(t, s, http.MethodPut, projectID, `{"enabled":null}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT null status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	m := decodeCaptureThinkingResponse(t, rr)
	if m["inherited"] != true {
		t.Errorf("after PUT null: inherited = %v, want true", m["inherited"])
	}
	if m["enabled"] != true {
		t.Errorf("after PUT null: enabled = %v, want true (resolved from global=true)", m["enabled"])
	}
}

func TestHandleGetProjectCaptureThinking_MissingProjectID(t *testing.T) {
	t.Parallel()
	s, _ := newProjectSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects//settings/capture-thinking", nil)
	rr := httptest.NewRecorder()
	s.handleGetProjectCaptureThinking(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GET empty id: status = %d, want 400", rr.Code)
	}
}

func TestHandlePutProjectCaptureThinking_MissingProjectID(t *testing.T) {
	t.Parallel()
	s, _ := newProjectSettingsServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects//settings/capture-thinking",
		strings.NewReader(`{"enabled":true}`))
	rr := httptest.NewRecorder()
	s.handlePutProjectCaptureThinking(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("PUT empty id: status = %d, want 400", rr.Code)
	}
}

func TestHandlePutProjectCaptureThinking_BadBody(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	rr := doCaptureThinkingRequest(t, s, http.MethodPut, projectID, `{not valid json`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("malformed JSON: status = %d, want 400", rr.Code)
	}
}

func TestHandlePutProjectCaptureThinking_ProjectsAreIsolated(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	if _, err := s.pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-ct-b', 'CT-B', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed second project: %v", err)
	}

	doCaptureThinkingRequest(t, s, http.MethodPut, projectID, `{"enabled":true}`)
	doCaptureThinkingRequest(t, s, http.MethodPut, "proj-ct-b", `{"enabled":false}`)

	rrA := doCaptureThinkingRequest(t, s, http.MethodGet, projectID, "")
	rrB := doCaptureThinkingRequest(t, s, http.MethodGet, "proj-ct-b", "")

	mA := decodeCaptureThinkingResponse(t, rrA)
	mB := decodeCaptureThinkingResponse(t, rrB)

	if mA["enabled"] != true {
		t.Errorf("proj A enabled = %v, want true", mA["enabled"])
	}
	if mB["enabled"] != false {
		t.Errorf("proj B enabled = %v, want false", mB["enabled"])
	}
	if mA["inherited"] != false {
		t.Errorf("proj A inherited = %v, want false", mA["inherited"])
	}
	if mB["inherited"] != false {
		t.Errorf("proj B inherited = %v, want false", mB["inherited"])
	}
}
