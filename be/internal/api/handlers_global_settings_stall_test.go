package api

import (
	"fmt"
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

// TestHandlePatchGlobalSettings_StallTimeout_SetUpdate verifies PATCH of an integer
// value (including 0/disabled) persists and is reflected by GET, for both stall
// fields, including overwriting a previously set value.
func TestHandlePatchGlobalSettings_StallTimeout_SetUpdate(t *testing.T) {
	cases := []struct {
		name  string
		field string
		// pre is an optional value PATCHed before the asserted value (overwrite case).
		pre  int
		hasP bool
		val  int
	}{
		{"start_60", "stall_start_timeout_sec", 0, false, 60},
		{"running_300", "stall_running_timeout_sec", 0, false, 300},
		{"start_zero_disabled", "stall_start_timeout_sec", 0, false, 0},
		{"running_zero_disabled", "stall_running_timeout_sec", 0, false, 0},
		{"start_update_overwrites", "stall_start_timeout_sec", 60, true, 180},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := newGlobalSettingsServer(t)

			if tc.hasP {
				req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
					strings.NewReader(fmt.Sprintf(`{%q:%d}`, tc.field, tc.pre)))
				rr := httptest.NewRecorder()
				s.handlePatchGlobalSettings(rr, req)
				if rr.Code != http.StatusOK {
					t.Fatalf("pre PATCH status = %d, want 200", rr.Code)
				}
			}

			req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
				strings.NewReader(fmt.Sprintf(`{%q:%d}`, tc.field, tc.val)))
			rr := httptest.NewRecorder()
			s.handlePatchGlobalSettings(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("PATCH status = %d, want 200", rr.Code)
			}

			getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
			getRR := httptest.NewRecorder()
			s.handleGetGlobalSettings(getRR, getReq)
			resp := decodeSettingsResponse(t, getRR)
			if v := resp[tc.field]; v != float64(tc.val) {
				t.Errorf("%s = %v, want %d", tc.field, v, tc.val)
			}
		})
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

	body := `{"low_consumption_mode":true,"stall_start_timeout_sec":60,"stall_running_timeout_sec":300}`
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
	if v := resp["stall_start_timeout_sec"]; v != float64(60) {
		t.Errorf("stall_start_timeout_sec = %v, want 60", v)
	}
	if v := resp["stall_running_timeout_sec"]; v != float64(300) {
		t.Errorf("stall_running_timeout_sec = %v, want 300", v)
	}
}
