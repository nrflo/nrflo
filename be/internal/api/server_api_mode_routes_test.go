package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
)

// newRoutesMux creates a minimal Server and calls registerRoutes, returning the
// configured mux. Routes are always registered; the apiMode bool controls whether
// api_mode_enabled is seeded in the DB (true) or left unset (false).
func newRoutesMux(t *testing.T, apiMode bool) *http.ServeMux {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "routes_test.db")
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
	s := &Server{pool: pool, clock: clock.Real()}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return mux
}

// TestAPIRoutes_ToolDefinitions_DisabledInCLIMode verifies that GET /api/v1/tool-definitions
// returns 400 api_mode_disabled when setting is not enabled.
func TestAPIRoutes_ToolDefinitions_DisabledInCLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tool-definitions", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("GET /api/v1/tool-definitions (api mode off) status = %d, want 400", rr.Code)
	}
}

// TestAPIRoutes_APICredentials_DisabledInCLIMode verifies that GET /api/v1/api-credentials
// returns 400 api_mode_disabled when setting is not enabled.
func TestAPIRoutes_APICredentials_DisabledInCLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-credentials", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("GET /api/v1/api-credentials (api mode off) status = %d, want 400", rr.Code)
	}
}

// TestAPIRoutes_ToolDefinitions_EnabledInAPIMode verifies that GET /api/v1/tool-definitions
// is not blocked by the middleware when api_mode_enabled=true.
func TestAPIRoutes_ToolDefinitions_EnabledInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tool-definitions", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code == http.StatusBadRequest {
		t.Errorf("GET /api/v1/tool-definitions (api mode on) returned 400; middleware should pass")
	}
}

// TestAPIRoutes_APICredentials_EnabledInAPIMode verifies that GET /api/v1/api-credentials
// is not blocked by the middleware when api_mode_enabled=true.
func TestAPIRoutes_APICredentials_EnabledInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-credentials", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code == http.StatusBadRequest {
		t.Errorf("GET /api/v1/api-credentials (api mode on) returned 400; middleware should pass")
	}
}

// TestAPIRoutes_NonGatedRoute_AlwaysAccessible verifies that standard routes like
// GET /api/v1/settings are accessible regardless of api_mode_enabled.
func TestAPIRoutes_NonGatedRoute_AlwaysAccessible(t *testing.T) {
	for _, apiMode := range []bool{false, true} {
		mux := newRoutesMux(t, apiMode)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code == http.StatusNotFound {
			t.Errorf("GET /api/v1/settings (apiMode=%v) returned 404; should always be registered", apiMode)
		}
	}
}
