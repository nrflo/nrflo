package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// isAPIModeDisabledResponse returns true when the response body contains the
// api_mode_disabled error, which is the middleware's specific rejection shape.
func isAPIModeDisabledResponse(rr *httptest.ResponseRecorder) bool {
	if rr.Code != http.StatusBadRequest {
		return false
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		return false
	}
	return body["error"] == "api_mode_disabled"
}

func TestAPIRoutes_Review_DisabledInCLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/review", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if !isAPIModeDisabledResponse(rr) {
		t.Errorf("GET /api/v1/review (api mode off) = %d body=%s; want 400 api_mode_disabled",
			rr.Code, rr.Body.String())
	}
}

func TestAPIRoutes_ConfigFiles_DisabledInCLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config-files", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if !isAPIModeDisabledResponse(rr) {
		t.Errorf("GET /api/v1/config-files (api mode off) = %d body=%s; want 400 api_mode_disabled",
			rr.Code, rr.Body.String())
	}
}

func TestAPIRoutes_InsightsSummary_DisabledInCLIMode(t *testing.T) {
	mux := newRoutesMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/insights/summary", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if !isAPIModeDisabledResponse(rr) {
		t.Errorf("GET /api/v1/insights/summary (api mode off) = %d body=%s; want 400 api_mode_disabled",
			rr.Code, rr.Body.String())
	}
}

func TestAPIRoutes_Review_PassesThroughInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/review", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if isAPIModeDisabledResponse(rr) {
		t.Error("GET /api/v1/review (api mode on): middleware blocked with api_mode_disabled; should pass through")
	}
}

func TestAPIRoutes_ConfigFiles_PassesThroughInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config-files", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if isAPIModeDisabledResponse(rr) {
		t.Error("GET /api/v1/config-files (api mode on): middleware blocked with api_mode_disabled; should pass through")
	}
}

func TestAPIRoutes_InsightsSummary_PassesThroughInAPIMode(t *testing.T) {
	mux := newRoutesMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/insights/summary", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if isAPIModeDisabledResponse(rr) {
		t.Error("GET /api/v1/insights/summary (api mode on): middleware blocked with api_mode_disabled; should pass through")
	}
}
