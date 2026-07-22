package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/types"
)

// TestHandleCheckCustomProvider_HappyPath verifies a reachable
// OpenAI-compatible server produces {ok:true, models:[...]}.
func TestHandleCheckCustomProvider_HappyPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "qwen3.5:4b"}},
		})
	}))
	t.Cleanup(upstream.Close)

	s := newCustomProvidersServer(t)
	rr := httptest.NewRecorder()
	s.handleCheckCustomProvider(rr, customProviderRequest(http.MethodPost, "/api/v1/custom-providers/check", "",
		`{"base_url":"`+upstream.URL+`"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp types.CustomProviderCheckResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK || len(resp.Models) != 1 || resp.Models[0] != "qwen3.5:4b" {
		t.Errorf("resp = %+v, want ok=true models=[qwen3.5:4b]", resp)
	}
}

// TestHandleCheckCustomProvider_UpstreamFailure_Returns200WithOKFalse
// verifies connectivity/upstream failures are reported inline in a 200 body
// rather than as an HTTP error status, mirroring the models test route.
func TestHandleCheckCustomProvider_UpstreamFailure_Returns200WithOKFalse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)

	s := newCustomProvidersServer(t)
	rr := httptest.NewRecorder()
	s.handleCheckCustomProvider(rr, customProviderRequest(http.MethodPost, "/api/v1/custom-providers/check", "",
		`{"base_url":"`+upstream.URL+`"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp types.CustomProviderCheckResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.OK || resp.Error == "" {
		t.Errorf("resp = %+v, want ok=false with a non-empty error", resp)
	}
}

// TestHandleCheckCustomProvider_MalformedBaseURL_Returns400 verifies a
// validation failure on base_url maps to 400, distinct from upstream
// connectivity failures which map to 200.
func TestHandleCheckCustomProvider_MalformedBaseURL_Returns400(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
	}{
		{"empty", ""},
		{"no scheme", "not-a-url"},
		{"ftp scheme", "ftp://localhost:11434"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newCustomProvidersServer(t)
			rr := httptest.NewRecorder()
			s.handleCheckCustomProvider(rr, customProviderRequest(http.MethodPost, "/api/v1/custom-providers/check", "",
				`{"base_url":"`+tc.baseURL+`"}`))
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

// TestHandleCheckCustomProvider_InvalidJSONBody_Returns400 verifies a
// malformed request body 400s before reaching the service layer.
func TestHandleCheckCustomProvider_InvalidJSONBody_Returns400(t *testing.T) {
	s := newCustomProvidersServer(t)
	rr := httptest.NewRecorder()
	s.handleCheckCustomProvider(rr, customProviderRequest(http.MethodPost, "/api/v1/custom-providers/check", "", `not json`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

// TestCheckCustomProviderRoute_AdminOnly verifies the /check route is gated
// like the rest of the custom-providers routes: 403 without an admin user in
// context, 200 with one.
func TestCheckCustomProviderRoute_AdminOnly(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{}})
	}))
	t.Cleanup(upstream.Close)

	_, mux := customProviderRoutesMux(t)
	body := `{"base_url":"` + upstream.URL + `"}`

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/custom-providers/check", strings.NewReader(body)))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("no-admin status = %d, want 403; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, asAdmin(httptest.NewRequest(http.MethodPost, "/api/v1/custom-providers/check", strings.NewReader(body))))
	if rr.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
}
