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

// TestAPIRoutes_ToolDefinitions_RouteGone verifies that GET /api/v1/tool-definitions
// returns 404 — the route was removed when the HTTP tools feature was deleted.
func TestAPIRoutes_ToolDefinitions_RouteGone(t *testing.T) {
	mux := newRoutesMux(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tool-definitions", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/tool-definitions status = %d, want 404", rr.Code)
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

// TestAPIRoutes_APICredentials_RouteGone verifies that GET /api/v1/api-credentials returns
// 404 — the route was removed when the API Credentials feature was deleted.
func TestAPIRoutes_APICredentials_RouteGone(t *testing.T) {
	mux := newRoutesMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-credentials", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/api-credentials status = %d, want 404", rr.Code)
	}
}
