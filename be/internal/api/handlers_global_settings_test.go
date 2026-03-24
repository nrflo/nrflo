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
)

func newGlobalSettingsServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "global_settings_handler_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

func decodeSettingsResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	return resp
}

// TestHandleGetGlobalSettings_DefaultFalse verifies fresh DB returns false.
func TestHandleGetGlobalSettings_DefaultFalse(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200", rr.Code)
	}

	resp := decodeSettingsResponse(t, rr)
	v, ok := resp["low_consumption_mode"]
	if !ok {
		t.Fatal("response missing low_consumption_mode field")
	}
	if v != false {
		t.Errorf("low_consumption_mode = %v, want false", v)
	}
}

// TestHandlePatchGlobalSettings_EnableThenGet verifies PATCH sets to true and GET reflects it.
func TestHandlePatchGlobalSettings_EnableThenGet(t *testing.T) {
	s := newGlobalSettingsServer(t)

	// PATCH to enable
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"low_consumption_mode":true}`))
	patchRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", patchRR.Code)
	}

	// GET should return true
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200", getRR.Code)
	}
	resp := decodeSettingsResponse(t, getRR)
	if v, ok := resp["low_consumption_mode"]; !ok {
		t.Error("response missing low_consumption_mode")
	} else if v != true {
		t.Errorf("low_consumption_mode = %v, want true", v)
	}
}

// TestHandlePatchGlobalSettings_Toggle verifies enable then disable works correctly.
func TestHandlePatchGlobalSettings_Toggle(t *testing.T) {
	s := newGlobalSettingsServer(t)

	// Enable
	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"low_consumption_mode":true}`))
	rr1 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("enable PATCH status = %d, want 200", rr1.Code)
	}

	// Disable
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"low_consumption_mode":false}`))
	rr2 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("disable PATCH status = %d, want 200", rr2.Code)
	}

	// GET should return false
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr3 := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr3, req3)

	resp := decodeSettingsResponse(t, rr3)
	if v, ok := resp["low_consumption_mode"]; !ok {
		t.Error("response missing low_consumption_mode")
	} else if v != false {
		t.Errorf("after toggle off, low_consumption_mode = %v, want false", v)
	}
}

// TestHandlePatchGlobalSettings_InvalidJSON returns 400 for malformed body.
func TestHandlePatchGlobalSettings_InvalidJSON(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader("not json"))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandlePatchGlobalSettings_NullFieldPreserves verifies PATCH with null field doesn't clear.
func TestHandlePatchGlobalSettings_NullFieldPreserves(t *testing.T) {
	s := newGlobalSettingsServer(t)

	// Enable
	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"low_consumption_mode":true}`))
	rr1 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("PATCH enable status = %d, want 200", rr1.Code)
	}

	// PATCH with empty body — should not change value
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{}`))
	rr2 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("empty PATCH status = %d, want 200", rr2.Code)
	}

	// GET should still return true
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr3 := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr3, req3)

	resp := decodeSettingsResponse(t, rr3)
	if v, ok := resp["low_consumption_mode"]; !ok {
		t.Error("response missing low_consumption_mode")
	} else if v != true {
		t.Errorf("null-field PATCH: low_consumption_mode = %v, want true (field should be preserved)", v)
	}
}

// TestHandlePatchGlobalSettings_ResponseBody verifies PATCH returns status field.
func TestHandlePatchGlobalSettings_ResponseBody(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"low_consumption_mode":true}`))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "updated" {
		t.Errorf("status = %q, want %q", resp["status"], "updated")
	}
}
