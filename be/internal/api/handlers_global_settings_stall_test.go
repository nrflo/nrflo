package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleGetGlobalSettings_StallTimeoutsNull verifies fresh DB returns null for both stall timeout fields.
func TestHandleGetGlobalSettings_StallTimeoutsNull(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200", rr.Code)
	}
	resp := decodeSettingsResponse(t, rr)

	for _, field := range []string{"stall_start_timeout_sec", "stall_running_timeout_sec"} {
		v, ok := resp[field]
		if !ok {
			t.Errorf("response missing %s field", field)
			continue
		}
		if v != nil {
			t.Errorf("%s = %v, want null", field, v)
		}
	}
}

// TestHandlePatchGlobalSettings_StallStartTimeout_Set verifies PATCH integer persists and GET reflects it.
func TestHandlePatchGlobalSettings_StallStartTimeout_Set(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"stall_start_timeout_sec":60}`))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	resp := decodeSettingsResponse(t, getRR)
	if v := resp["stall_start_timeout_sec"]; v != float64(60) {
		t.Errorf("stall_start_timeout_sec = %v, want 60", v)
	}
}

// TestHandlePatchGlobalSettings_StallRunningTimeout_Set verifies PATCH integer persists and GET reflects it.
func TestHandlePatchGlobalSettings_StallRunningTimeout_Set(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"stall_running_timeout_sec":300}`))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	resp := decodeSettingsResponse(t, getRR)
	if v := resp["stall_running_timeout_sec"]; v != float64(300) {
		t.Errorf("stall_running_timeout_sec = %v, want 300", v)
	}
}

// TestHandlePatchGlobalSettings_StallStartTimeout_Zero verifies 0 (disabled) persists and GET returns 0.
func TestHandlePatchGlobalSettings_StallStartTimeout_Zero(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"stall_start_timeout_sec":0}`))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	resp := decodeSettingsResponse(t, getRR)
	if v := resp["stall_start_timeout_sec"]; v != float64(0) {
		t.Errorf("stall_start_timeout_sec = %v, want 0 (disabled)", v)
	}
}

// TestHandlePatchGlobalSettings_StallRunningTimeout_Zero verifies 0 (disabled) persists and GET returns 0.
func TestHandlePatchGlobalSettings_StallRunningTimeout_Zero(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"stall_running_timeout_sec":0}`))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	resp := decodeSettingsResponse(t, getRR)
	if v := resp["stall_running_timeout_sec"]; v != float64(0) {
		t.Errorf("stall_running_timeout_sec = %v, want 0 (disabled)", v)
	}
}

// TestHandlePatchGlobalSettings_StallStartTimeout_NullClears verifies null clears a previously set value.
func TestHandlePatchGlobalSettings_StallStartTimeout_NullClears(t *testing.T) {
	s := newGlobalSettingsServer(t)

	// Set a value first.
	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"stall_start_timeout_sec":60}`))
	rr1 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("set PATCH status = %d, want 200", rr1.Code)
	}

	// Send null to clear.
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"stall_start_timeout_sec":null}`))
	rr2 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("null PATCH status = %d, want 200", rr2.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	resp := decodeSettingsResponse(t, getRR)
	if v := resp["stall_start_timeout_sec"]; v != nil {
		t.Errorf("stall_start_timeout_sec = %v, want nil (cleared)", v)
	}
}

// TestHandlePatchGlobalSettings_StallTimeouts_Negative verifies negative values are rejected with 400.
func TestHandlePatchGlobalSettings_StallTimeouts_Negative(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"start_negative_1", `{"stall_start_timeout_sec":-1}`},
		{"start_negative_large", `{"stall_start_timeout_sec":-100}`},
		{"running_negative_1", `{"stall_running_timeout_sec":-1}`},
		{"running_negative_large", `{"stall_running_timeout_sec":-100}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newGlobalSettingsServer(t)
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()
			s.handlePatchGlobalSettings(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("PATCH %s: status = %d, want 400", tc.body, rr.Code)
			}
		})
	}
}

// TestHandlePatchGlobalSettings_BothStallFields verifies both stall fields can be set in one PATCH.
func TestHandlePatchGlobalSettings_BothStallFields(t *testing.T) {
	s := newGlobalSettingsServer(t)

	body := `{"stall_start_timeout_sec":60,"stall_running_timeout_sec":300}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	resp := decodeSettingsResponse(t, getRR)

	if v := resp["stall_start_timeout_sec"]; v != float64(60) {
		t.Errorf("stall_start_timeout_sec = %v, want 60", v)
	}
	if v := resp["stall_running_timeout_sec"]; v != float64(300) {
		t.Errorf("stall_running_timeout_sec = %v, want 300", v)
	}
}

// TestHandlePatchGlobalSettings_StallTimeout_AbsentPreserves verifies absent field does not clear existing value.
func TestHandlePatchGlobalSettings_StallTimeout_AbsentPreserves(t *testing.T) {
	s := newGlobalSettingsServer(t)

	// Set both.
	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"stall_start_timeout_sec":90,"stall_running_timeout_sec":600}`))
	rr1 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want 200", rr1.Code)
	}

	// PATCH only one field — the other must be preserved.
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"stall_start_timeout_sec":45}`))
	rr2 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("partial PATCH status = %d, want 200", rr2.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	resp := decodeSettingsResponse(t, getRR)

	if v := resp["stall_start_timeout_sec"]; v != float64(45) {
		t.Errorf("stall_start_timeout_sec = %v, want 45 (updated)", v)
	}
	if v := resp["stall_running_timeout_sec"]; v != float64(600) {
		t.Errorf("stall_running_timeout_sec = %v, want 600 (preserved)", v)
	}
}

// TestHandlePatchGlobalSettings_StallAndOtherFields verifies stall fields coexist with other settings.
func TestHandlePatchGlobalSettings_StallAndOtherFields(t *testing.T) {
	s := newGlobalSettingsServer(t)

	body := `{"low_consumption_mode":true,"session_retention_limit":50,"stall_start_timeout_sec":60,"stall_running_timeout_sec":300}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", rr.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	resp := decodeSettingsResponse(t, getRR)

	if v := resp["low_consumption_mode"]; v != true {
		t.Errorf("low_consumption_mode = %v, want true", v)
	}
	if v := resp["session_retention_limit"]; v != float64(50) {
		t.Errorf("session_retention_limit = %v, want 50", v)
	}
	if v := resp["stall_start_timeout_sec"]; v != float64(60) {
		t.Errorf("stall_start_timeout_sec = %v, want 60", v)
	}
	if v := resp["stall_running_timeout_sec"]; v != float64(300) {
		t.Errorf("stall_running_timeout_sec = %v, want 300", v)
	}
}

// TestHandleGetGlobalSettings_StallTimeout_Update verifies sequential PATCH updates are reflected by GET.
func TestHandleGetGlobalSettings_StallTimeout_Update(t *testing.T) {
	s := newGlobalSettingsServer(t)

	// First value.
	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"stall_start_timeout_sec":60}`))
	rr1 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first PATCH status = %d, want 200", rr1.Code)
	}

	// Update to a different value.
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"stall_start_timeout_sec":180}`))
	rr2 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second PATCH status = %d, want 200", rr2.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	resp := decodeSettingsResponse(t, getRR)
	if v := resp["stall_start_timeout_sec"]; v != float64(180) {
		t.Errorf("stall_start_timeout_sec = %v, want 180 (latest value)", v)
	}
}
