package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleGetGlobalSettings_ObserverDefaults verifies fresh DB returns documented observer defaults.
func TestHandleGetGlobalSettings_ObserverDefaults(t *testing.T) {
	t.Parallel()
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	resp := decodeSettingsResponse(t, rr)

	if v := resp["experimental_observer_enabled"]; v != false {
		t.Errorf("experimental_observer_enabled = %v, want false", v)
	}
	if v := resp["observer_system_context"]; v != "" {
		t.Errorf("observer_system_context = %v, want empty string", v)
	}
	if v := resp["observer_provider"]; v != "" {
		t.Errorf("observer_provider = %v, want empty string", v)
	}
	if v := resp["observer_model"]; v != "" {
		t.Errorf("observer_model = %v, want empty string", v)
	}
}

// TestHandlePatchGlobalSettings_ObserverEnabled_RoundTrip verifies enable/disable round-trip.
func TestHandlePatchGlobalSettings_ObserverEnabled_RoundTrip(t *testing.T) {
	t.Parallel()
	s := newGlobalSettingsServer(t)

	// Enable
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"experimental_observer_enabled":true}`))
	patchRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", patchRR.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)

	resp := decodeSettingsResponse(t, getRR)
	if v := resp["experimental_observer_enabled"]; v != true {
		t.Errorf("after enable: experimental_observer_enabled = %v, want true", v)
	}

	// Disable
	patchReq2 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"experimental_observer_enabled":false}`))
	patchRR2 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR2, patchReq2)
	if patchRR2.Code != http.StatusOK {
		t.Fatalf("second PATCH status = %d, want 200", patchRR2.Code)
	}

	getReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR2 := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR2, getReq2)

	resp2 := decodeSettingsResponse(t, getRR2)
	if v := resp2["experimental_observer_enabled"]; v != false {
		t.Errorf("after disable: experimental_observer_enabled = %v, want false", v)
	}
}

// TestHandlePatchGlobalSettings_ObserverContext_RoundTrip verifies observer_system_context round-trip.
func TestHandlePatchGlobalSettings_ObserverContext_RoundTrip(t *testing.T) {
	t.Parallel()
	s := newGlobalSettingsServer(t)

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"observer_system_context":"watch for errors"}`))
	patchRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", patchRR.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)

	resp := decodeSettingsResponse(t, getRR)
	if v := resp["observer_system_context"]; v != "watch for errors" {
		t.Errorf("observer_system_context = %v, want %q", v, "watch for errors")
	}
}

// TestHandlePatchGlobalSettings_ObserverProviderModel_RoundTrip verifies provider and model fields.
func TestHandlePatchGlobalSettings_ObserverProviderModel_RoundTrip(t *testing.T) {
	t.Parallel()
	s := newGlobalSettingsServer(t)

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"observer_provider":"claude","observer_model":"opus"}`))
	patchRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", patchRR.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)

	resp := decodeSettingsResponse(t, getRR)
	if v := resp["observer_provider"]; v != "claude" {
		t.Errorf("observer_provider = %v, want claude", v)
	}
	if v := resp["observer_model"]; v != "opus" {
		t.Errorf("observer_model = %v, want opus", v)
	}
}

// TestHandlePatchGlobalSettings_ObserverAllFields verifies all four observer fields in one PATCH.
func TestHandlePatchGlobalSettings_ObserverAllFields(t *testing.T) {
	t.Parallel()
	s := newGlobalSettingsServer(t)

	body := `{"experimental_observer_enabled":true,"observer_system_context":"ctx","observer_provider":"openai","observer_model":"gpt-4"}`
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(body))
	patchRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200; body=%s", patchRR.Code, patchRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)

	resp := decodeSettingsResponse(t, getRR)
	checks := map[string]interface{}{
		"experimental_observer_enabled": true,
		"observer_system_context":       "ctx",
		"observer_provider":             "openai",
		"observer_model":                "gpt-4",
	}
	for key, want := range checks {
		if got := resp[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
}

// TestHandlePatchGlobalSettings_ObserverFieldsPreservedOnEmptyPatch verifies unrelated PATCH leaves observer fields intact.
func TestHandlePatchGlobalSettings_ObserverFieldsPreservedOnEmptyPatch(t *testing.T) {
	t.Parallel()
	s := newGlobalSettingsServer(t)

	// Set observer context first.
	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"observer_system_context":"my-context"}`))
	rr1 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want 200", rr1.Code)
	}

	// PATCH unrelated field — observer_system_context must be untouched.
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"low_consumption_mode":true}`))
	rr2 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second PATCH status = %d, want 200", rr2.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)

	resp := decodeSettingsResponse(t, getRR)
	if v := resp["observer_system_context"]; v != "my-context" {
		t.Errorf("observer_system_context = %v, want my-context (should be preserved)", v)
	}
}
