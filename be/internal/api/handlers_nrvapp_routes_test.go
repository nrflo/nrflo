package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIRoutes_NrvappReview_404_CLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nrvapp/review", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/nrvapp/review (cli mode) = %d, want 404", rr.Code)
	}
}

func TestAPIRoutes_NrvappConfigFiles_404_CLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nrvapp/config/files", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/nrvapp/config/files (cli mode) = %d, want 404", rr.Code)
	}
}

func TestAPIRoutes_NrvappInsightsSummary_404_CLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nrvapp/insights/summary", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/nrvapp/insights/summary (cli mode) = %d, want 404", rr.Code)
	}
}

func TestAPIRoutes_NrvappReview_RegisteredInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nrvapp/review", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/nrvapp/review (api mode) returned 404; route should be registered")
	}
}

func TestAPIRoutes_NrvappConfigFiles_RegisteredInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nrvapp/config/files", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/nrvapp/config/files (api mode) returned 404; route should be registered")
	}
}

func TestAPIRoutes_NrvappInsightsSummary_RegisteredInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nrvapp/insights/summary", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/nrvapp/insights/summary (api mode) returned 404; route should be registered")
	}
}
