package api

import (
	"encoding/json"
	"fmt"
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

// TestHandleGetGlobalSettings_DefaultRetentionLimit verifies fresh DB returns session_retention_limit=100.
func TestHandleGetGlobalSettings_DefaultRetentionLimit(t *testing.T) {
	s := newGlobalSettingsServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200", rr.Code)
	}
	resp := decodeSettingsResponse(t, rr)
	// JSON numbers decode to float64 in map[string]interface{}
	if v, ok := resp["session_retention_limit"]; !ok {
		t.Fatal("response missing session_retention_limit field")
	} else if v != float64(100) {
		t.Errorf("session_retention_limit = %v, want 100", v)
	}
}

// TestHandlePatchGlobalSettings_RetentionLimit verifies PATCH persists and GET reflects new value.
func TestHandlePatchGlobalSettings_RetentionLimit(t *testing.T) {
	s := newGlobalSettingsServer(t)
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"session_retention_limit":50}`))
	patchRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", patchRR.Code)
	}
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200", getRR.Code)
	}
	resp := decodeSettingsResponse(t, getRR)
	if v, ok := resp["session_retention_limit"]; !ok {
		t.Error("response missing session_retention_limit")
	} else if v != float64(50) {
		t.Errorf("session_retention_limit = %v, want 50", v)
	}
}

// TestHandlePatchGlobalSettings_RetentionLimitTooLow verifies PATCH with value < 10 returns 400.
func TestHandlePatchGlobalSettings_RetentionLimitTooLow(t *testing.T) {
	s := newGlobalSettingsServer(t)
	cases := []struct {
		name  string
		value int
	}{
		{"zero", 0},
		{"one", 1},
		{"nine", 9},
		{"negative", -5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.NewReader(`{"session_retention_limit":` + fmt.Sprintf("%d", tc.value) + `}`)
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", body)
			rr := httptest.NewRecorder()
			s.handlePatchGlobalSettings(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("value %d: status = %d, want 400", tc.value, rr.Code)
			}
		})
	}
}

// TestHandlePatchGlobalSettings_RetentionLimitMinimumAccepted verifies that exactly 10 is accepted.
func TestHandlePatchGlobalSettings_RetentionLimitMinimumAccepted(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"session_retention_limit":10}`))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("PATCH with 10 (min): status = %d, want 200", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	resp := decodeSettingsResponse(t, getRR)
	if v := resp["session_retention_limit"]; v != float64(10) {
		t.Errorf("session_retention_limit = %v, want 10", v)
	}
}

// TestHandlePatchGlobalSettings_RetentionLimitNull verifies empty PATCH preserves existing value.
func TestHandlePatchGlobalSettings_RetentionLimitNull(t *testing.T) {
	s := newGlobalSettingsServer(t)
	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"session_retention_limit":50}`))
	rr1 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want 200", rr1.Code)
	}
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{}`))
	rr2 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("empty PATCH status = %d, want 200", rr2.Code)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr3 := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr3, req3)
	resp := decodeSettingsResponse(t, rr3)
	if v := resp["session_retention_limit"]; v != float64(50) {
		t.Errorf("after null PATCH: session_retention_limit = %v, want 50", v)
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
