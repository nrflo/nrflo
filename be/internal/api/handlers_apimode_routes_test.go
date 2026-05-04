package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIRoutes_Review_404_CLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/review", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/review (cli mode) = %d, want 404", rr.Code)
	}
}

func TestAPIRoutes_ConfigFiles_404_CLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config-files", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/config-files (cli mode) = %d, want 404", rr.Code)
	}
}

func TestAPIRoutes_InsightsSummary_404_CLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/insights/summary", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/insights/summary (cli mode) = %d, want 404", rr.Code)
	}
}

func TestAPIRoutes_Review_RegisteredInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/review", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/review (api mode) returned 404; route should be registered")
	}
}

func TestAPIRoutes_ConfigFiles_RegisteredInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config-files", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/config-files (api mode) returned 404; route should be registered")
	}
}

func TestAPIRoutes_InsightsSummary_RegisteredInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/insights/summary", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/insights/summary (api mode) returned 404; route should be registered")
	}
}
