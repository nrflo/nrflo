package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
)

// newGlobalSettingsServerAPIMode creates a Server for settings handler tests.
// If apiMode is true, seeds api_mode_enabled=true in the DB before returning.
func newGlobalSettingsServerAPIMode(t *testing.T, apiMode bool) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "settings_apimode_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	if apiMode {
		svc := service.NewGlobalSettingsService(pool, clock.Real())
		if err := svc.Set("api_mode_enabled", "true"); err != nil {
			t.Fatalf("seed api_mode_enabled: %v", err)
		}
	}
	return &Server{pool: pool, clock: clock.Real()}
}

// TestHandleGetGlobalSettings_APIModeEnabled_False verifies that GET /api/v1/settings
// returns api_mode_enabled=false when the setting is not seeded in DB.
func TestHandleGetGlobalSettings_APIModeEnabled_False(t *testing.T) {
	s := newGlobalSettingsServerAPIMode(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}

	resp := decodeSettingsResponse(t, rr)
	v, ok := resp["api_mode_enabled"]
	if !ok {
		t.Fatal("response missing api_mode_enabled field")
	}
	if v != false {
		t.Errorf("api_mode_enabled = %v, want false", v)
	}
}

// TestHandleGetGlobalSettings_APIModeEnabled_True verifies that GET /api/v1/settings
// returns api_mode_enabled=true after seeding the setting in DB.
func TestHandleGetGlobalSettings_APIModeEnabled_True(t *testing.T) {
	s := newGlobalSettingsServerAPIMode(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}

	resp := decodeSettingsResponse(t, rr)
	v, ok := resp["api_mode_enabled"]
	if !ok {
		t.Fatal("response missing api_mode_enabled field")
	}
	if v != true {
		t.Errorf("api_mode_enabled = %v, want true", v)
	}
}

// TestHandlePatchGlobalSettings_APIModeEnabled_Persists verifies that PATCH
// api_mode_enabled=true is stored in DB and subsequent GET reflects the new value.
func TestHandlePatchGlobalSettings_APIModeEnabled_Persists(t *testing.T) {
	s := newGlobalSettingsServerAPIMode(t, false)

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"api_mode_enabled":true}`))
	patchRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200; body: %s", patchRR.Code, patchRR.Body.String())
	}

	// GET should now report true.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)

	resp := decodeSettingsResponse(t, getRR)
	if v, ok := resp["api_mode_enabled"]; !ok {
		t.Fatal("response missing api_mode_enabled field")
	} else if v != true {
		t.Errorf("api_mode_enabled after PATCH = %v, want true", v)
	}
}

// TestHandlePatchGlobalSettings_APIModeEnabled_ToggleOffOn verifies that PATCH
// can toggle api_mode_enabled off and on in sequence.
func TestHandlePatchGlobalSettings_APIModeEnabled_ToggleOffOn(t *testing.T) {
	s := newGlobalSettingsServerAPIMode(t, true)

	// Disable.
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"api_mode_enabled":false}`))
	patchRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH false status = %d, want 200", patchRR.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	resp := decodeSettingsResponse(t, getRR)
	if resp["api_mode_enabled"] != false {
		t.Errorf("api_mode_enabled after disable = %v, want false", resp["api_mode_enabled"])
	}
}
