package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// newRoutesMux creates a minimal Server and calls registerRoutes, returning the
// configured mux. The server has only pool and clock set; all other fields are nil.
// This is sufficient for route-existence tests since only tool-definitions and
// api-credentials handlers are invoked (both only need s.pool).
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
	s := &Server{pool: pool, clock: clock.Real(), apiMode: apiMode}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return mux
}

// TestAPIRoutes_ToolDefinitions_404_CLIMode verifies that GET /api/v1/tool-definitions
// returns 404 when the server is in cli mode (route not registered).
func TestAPIRoutes_ToolDefinitions_404_CLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tool-definitions", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/tool-definitions (cli mode) status = %d, want 404", rr.Code)
	}
}

// TestAPIRoutes_APICredentials_404_CLIMode verifies that GET /api/v1/api-credentials
// returns 404 when the server is in cli mode (route not registered).
func TestAPIRoutes_APICredentials_404_CLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-credentials", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/api-credentials (cli mode) status = %d, want 404", rr.Code)
	}
}

// TestAPIRoutes_ToolDefinitions_RegisteredInAPIMode verifies that GET /api/v1/tool-definitions
// is served (not 404) when the server is in api mode.
func TestAPIRoutes_ToolDefinitions_RegisteredInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tool-definitions", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/tool-definitions (api mode) returned 404; route should be registered")
	}
}

// TestAPIRoutes_APICredentials_RegisteredInAPIMode verifies that GET /api/v1/api-credentials
// is served (not 404) when the server is in api mode.
func TestAPIRoutes_APICredentials_RegisteredInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-credentials", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/api-credentials (api mode) returned 404; route should be registered")
	}
}

// TestAPIRoutes_ToolDefinitionsRegister_404_CLIMode verifies the register endpoint returns 404 in cli mode.
func TestAPIRoutes_ToolDefinitionsRegister_404_CLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tool-definitions/register", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("POST /api/v1/tool-definitions/register (cli mode) status = %d, want 404", rr.Code)
	}
}

// TestAPIRoutes_ToolDefinitionsRegister_RegisteredInAPIMode verifies the register endpoint is served in api mode.
func TestAPIRoutes_ToolDefinitionsRegister_RegisteredInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tool-definitions/register", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("POST /api/v1/tool-definitions/register (api mode) returned 404; route should be registered")
	}
}

// TestAPIRoutes_NonGatedRoute_AlwaysRegistered verifies that standard routes like
// GET /api/v1/settings are accessible regardless of apiMode.
func TestAPIRoutes_NonGatedRoute_AlwaysRegistered(t *testing.T) {
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
