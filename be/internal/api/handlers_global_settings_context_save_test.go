package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleGetGlobalSettings_ContextSaveViaAgentDefault verifies fresh DB returns false.
func TestHandleGetGlobalSettings_ContextSaveViaAgentDefault(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}

	resp := decodeSettingsResponse(t, rr)
	v, ok := resp["context_save_via_agent"]
	if !ok {
		t.Fatal("response missing context_save_via_agent field")
	}
	if v != false {
		t.Errorf("context_save_via_agent = %v, want false", v)
	}
}

// TestHandlePatchGlobalSettings_ContextSaveViaAgent_EnableThenGet verifies PATCH sets to true.
func TestHandlePatchGlobalSettings_ContextSaveViaAgent_EnableThenGet(t *testing.T) {
	s := newGlobalSettingsServer(t)

	// Enable
	body := strings.NewReader(`{"context_save_via_agent": true}`)
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", body)
	patchRr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRr, patchReq)
	if patchRr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", patchRr.Code)
	}

	// Verify
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRr := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRr, getReq)

	resp := decodeSettingsResponse(t, getRr)
	if resp["context_save_via_agent"] != true {
		t.Errorf("context_save_via_agent = %v, want true", resp["context_save_via_agent"])
	}
}

// TestHandlePatchGlobalSettings_ContextSaveViaAgent_Toggle verifies enable then disable.
func TestHandlePatchGlobalSettings_ContextSaveViaAgent_Toggle(t *testing.T) {
	s := newGlobalSettingsServer(t)

	// Enable
	body := strings.NewReader(`{"context_save_via_agent": true}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", body)
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH enable status = %d, want 200", rr.Code)
	}

	// Disable
	body = strings.NewReader(`{"context_save_via_agent": false}`)
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/settings", body)
	rr = httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH disable status = %d, want 200", rr.Code)
	}

	// Verify
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRr := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRr, getReq)

	resp := decodeSettingsResponse(t, getRr)
	if resp["context_save_via_agent"] != false {
		t.Errorf("context_save_via_agent = %v, want false", resp["context_save_via_agent"])
	}
}

// TestHandlePatchGlobalSettings_ContextSaveViaAgent_AbsentPreserves verifies absent field doesn't change value.
func TestHandlePatchGlobalSettings_ContextSaveViaAgent_AbsentPreserves(t *testing.T) {
	s := newGlobalSettingsServer(t)

	// Enable first
	body := strings.NewReader(`{"context_save_via_agent": true}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", body)
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)

	// Patch something else (absent context_save_via_agent should not change it)
	body = strings.NewReader(`{"low_consumption_mode": true}`)
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/settings", body)
	rr = httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)

	// Verify still true
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRr := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRr, getReq)

	resp := decodeSettingsResponse(t, getRr)
	if resp["context_save_via_agent"] != true {
		t.Errorf("context_save_via_agent = %v, want true (should be preserved)", resp["context_save_via_agent"])
	}
}
