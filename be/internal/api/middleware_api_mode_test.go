package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
)

// newMiddlewareTestServer creates a minimal Server backed by a fresh DB.
// The api_mode_enabled setting is NOT seeded (default off).
func newMiddlewareTestServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "mw_api_mode_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

// TestAPIMode_Middleware_BlocksWhenUnset verifies that apiModeOnly returns 400
// {"error":"api_mode_disabled"} when the setting is absent (never set in DB).
func TestAPIMode_Middleware_BlocksWhenUnset(t *testing.T) {
	s := newMiddlewareTestServer(t)

	handler := s.apiModeOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "api_mode_disabled" {
		t.Errorf("error = %q, want %q", body["error"], "api_mode_disabled")
	}
}

// TestAPIMode_Middleware_BlocksWhenFalse verifies that apiModeOnly returns 400
// when api_mode_enabled is explicitly set to "false".
func TestAPIMode_Middleware_BlocksWhenFalse(t *testing.T) {
	s := newMiddlewareTestServer(t)
	svc := service.NewGlobalSettingsService(s.pool, s.clock)
	if err := svc.Set("api_mode_enabled", "false"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	handler := s.apiModeOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "api_mode_disabled" {
		t.Errorf("error = %q, want %q", body["error"], "api_mode_disabled")
	}
}

// TestAPIMode_Middleware_PassesWhenTrue verifies that apiModeOnly calls next
// when api_mode_enabled=true is set in DB.
func TestAPIMode_Middleware_PassesWhenTrue(t *testing.T) {
	s := newMiddlewareTestServer(t)
	svc := service.NewGlobalSettingsService(s.pool, s.clock)
	if err := svc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	nextCalled := false
	handler := s.apiModeOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !nextCalled {
		t.Error("next handler was not called")
	}
}

// TestAPIMode_Middleware_LiveRead verifies request-time freshness: Set then GET
// reflects the new value immediately (no cache between requests).
func TestAPIMode_Middleware_LiveRead(t *testing.T) {
	s := newMiddlewareTestServer(t)
	svc := service.NewGlobalSettingsService(s.pool, s.clock)

	handler := s.apiModeOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request — setting absent → blocked.
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusBadRequest {
		t.Fatalf("before Set: status = %d, want 400", rr1.Code)
	}

	// Enable the setting.
	if err := svc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Second request — same handler, same server instance — should now pass.
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("after Set: status = %d, want 200 (live read not reflected)", rr2.Code)
	}

	// Disable again.
	if err := svc.Set("api_mode_enabled", "false"); err != nil {
		t.Fatalf("Set false: %v", err)
	}

	// Third request — must be blocked again.
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusBadRequest {
		t.Fatalf("after disable: status = %d, want 400", rr3.Code)
	}
}
