package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// newGlobalSettingsServerAPIMode creates a Server with the given apiMode flag for
// testing the api_mode_enabled read-only field in the settings response.
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
	return &Server{pool: pool, clock: clock.Real(), apiMode: apiMode}
}

// TestHandleGetGlobalSettings_APIModeEnabled_False verifies that a server started
// without --mode=api reports api_mode_enabled=false in the settings response.
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

// TestHandleGetGlobalSettings_APIModeEnabled_True verifies that a server started
// with --mode=api reports api_mode_enabled=true in the settings response.
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

// TestHandlePatchGlobalSettings_APIModeEnabled_ReadOnly verifies that PATCHing
// api_mode_enabled is silently ignored and does not alter the server's startup mode.
func TestHandlePatchGlobalSettings_APIModeEnabled_ReadOnly(t *testing.T) {
	// Server started with apiMode=false; PATCH tries to set api_mode_enabled=true.
	s := newGlobalSettingsServerAPIMode(t, false)

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"api_mode_enabled":true}`))
	patchRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", patchRR.Code)
	}

	// GET should still report false (read-only field not mutated by PATCH).
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)

	resp := decodeSettingsResponse(t, getRR)
	if v, ok := resp["api_mode_enabled"]; !ok {
		t.Fatal("response missing api_mode_enabled field")
	} else if v != false {
		t.Errorf("api_mode_enabled after PATCH = %v, want false (field is read-only)", v)
	}
}
