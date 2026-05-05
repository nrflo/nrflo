package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleGetGlobalSettings_ExperimentalDefaultFalse verifies fresh DB returns experimental=false.
func TestHandleGetGlobalSettings_ExperimentalDefaultFalse(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200", rr.Code)
	}

	resp := decodeSettingsResponse(t, rr)
	v, ok := resp["experimental"]
	if !ok {
		t.Fatal("response missing experimental field")
	}
	if v != false {
		t.Errorf("experimental = %v, want false", v)
	}
}

// TestHandlePatchGlobalSettings_ExperimentalEnableThenGet verifies PATCH true then GET returns true.
func TestHandlePatchGlobalSettings_ExperimentalEnableThenGet(t *testing.T) {
	s := newGlobalSettingsServer(t)

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"experimental":true}`))
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
	if v, ok := resp["experimental"]; !ok {
		t.Error("response missing experimental")
	} else if v != true {
		t.Errorf("experimental = %v, want true", v)
	}
}

// TestHandlePatchGlobalSettings_ExperimentalToggle verifies enable then disable returns false.
func TestHandlePatchGlobalSettings_ExperimentalToggle(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"experimental":true}`))
	rr1 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("enable PATCH status = %d, want 200", rr1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"experimental":false}`))
	rr2 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("disable PATCH status = %d, want 200", rr2.Code)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr3 := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr3, req3)

	resp := decodeSettingsResponse(t, rr3)
	if v, ok := resp["experimental"]; !ok {
		t.Error("response missing experimental")
	} else if v != false {
		t.Errorf("after toggle off, experimental = %v, want false", v)
	}
}
