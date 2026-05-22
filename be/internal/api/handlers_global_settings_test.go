package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

func newGlobalSettingsServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "global_settings_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

func decodeSettingsResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	return resp
}

// globalSettingsBoolFields are the boolean settings that share the identical
// PATCH→GET set/get block in handlers_global_settings.go. All default to false
// in a fresh (template) DB.
var globalSettingsBoolFields = []string{
	"low_consumption_mode",
	"context_save_via_agent",
	"simplified_agents_graph",
	"experimental",
	"api_mode_enabled",
}

// TestGlobalSettings_BoolFields exercises the shared bool-field code path for
// every field × {default, enable, toggle}:
//   - default: a fresh DB reports false
//   - enable:  PATCH true → GET true
//   - toggle:  PATCH true then PATCH false → GET false
func TestGlobalSettings_BoolFields(t *testing.T) {
	for _, field := range globalSettingsBoolFields {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Run("default", func(t *testing.T) {
				s := newGlobalSettingsServer(t)
				resp := getSettings(t, s)
				v, ok := resp[field]
				if !ok {
					t.Fatalf("response missing %s field", field)
				}
				if v != false {
					t.Errorf("%s = %v, want false", field, v)
				}
			})

			t.Run("enable", func(t *testing.T) {
				s := newGlobalSettingsServer(t)
				patchSettings(t, s, fmt.Sprintf(`{%q:true}`, field))
				resp := getSettings(t, s)
				if v, ok := resp[field]; !ok {
					t.Errorf("response missing %s", field)
				} else if v != true {
					t.Errorf("%s = %v, want true", field, v)
				}
			})

			t.Run("toggle", func(t *testing.T) {
				s := newGlobalSettingsServer(t)
				patchSettings(t, s, fmt.Sprintf(`{%q:true}`, field))
				patchSettings(t, s, fmt.Sprintf(`{%q:false}`, field))
				resp := getSettings(t, s)
				if v, ok := resp[field]; !ok {
					t.Errorf("response missing %s", field)
				} else if v != false {
					t.Errorf("after toggle off, %s = %v, want false", field, v)
				}
			})
		})
	}
}

// TestGlobalSettings_BoolFieldAbsentPreserves verifies that a PATCH which does
// not mention a previously enabled bool field leaves that field unchanged. The
// other-field PATCH guarantees the request is non-empty so this also covers the
// empty-body / null-field preserve case.
func TestGlobalSettings_BoolFieldAbsentPreserves(t *testing.T) {
	s := newGlobalSettingsServer(t)

	// Enable context_save_via_agent.
	patchSettings(t, s, `{"context_save_via_agent":true}`)

	// PATCH only a different field — context_save_via_agent must be preserved.
	patchSettings(t, s, `{"low_consumption_mode":true}`)

	// An empty PATCH must likewise preserve everything.
	patchSettings(t, s, `{}`)

	resp := getSettings(t, s)
	if resp["context_save_via_agent"] != true {
		t.Errorf("context_save_via_agent = %v, want true (should be preserved)", resp["context_save_via_agent"])
	}
	if resp["low_consumption_mode"] != true {
		t.Errorf("low_consumption_mode = %v, want true (should be preserved)", resp["low_consumption_mode"])
	}
}

// getSettings issues GET /api/v1/settings and returns the decoded body, failing
// the test on non-200.
func getSettings(t *testing.T, s *Server) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	return decodeSettingsResponse(t, rr)
}

// patchSettings issues PATCH /api/v1/settings with body, failing the test on non-200.
func patchSettings(t *testing.T, s *Server, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH %s status = %d, want 200", body, rr.Code)
	}
}

// TestHandlePatchGlobalSettings_InvalidJSON returns 400 for malformed body.
func TestHandlePatchGlobalSettings_InvalidJSON(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader("not json"))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandleGetGlobalSettings_NoRetentionLimitKey verifies fresh DB response does not include session_retention_limit.
func TestHandleGetGlobalSettings_NoRetentionLimitKey(t *testing.T) {
	s := newGlobalSettingsServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200", rr.Code)
	}
	resp := decodeSettingsResponse(t, rr)
	if _, ok := resp["session_retention_limit"]; ok {
		t.Errorf("response must not include session_retention_limit key, got %v", resp["session_retention_limit"])
	}
}

// TestHandlePatchGlobalSettings_RetentionLimitSilentIgnore verifies PATCH with session_retention_limit
// returns 200 and subsequent GET still has no key (unknown fields are silently ignored).
func TestHandlePatchGlobalSettings_RetentionLimitSilentIgnore(t *testing.T) {
	s := newGlobalSettingsServer(t)
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"session_retention_limit":50}`))
	patchRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", patchRR.Code)
	}
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200", getRR.Code)
	}
	resp := decodeSettingsResponse(t, getRR)
	if _, ok := resp["session_retention_limit"]; ok {
		t.Errorf("after PATCH with session_retention_limit, GET must not include the key, got %v", resp["session_retention_limit"])
	}
}

// TestHandlePatchGlobalSettings_RetentionLimitAnyValueIgnored verifies various session_retention_limit
// values in PATCH are silently ignored (200, no key in GET).
func TestHandlePatchGlobalSettings_RetentionLimitAnyValueIgnored(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"zero", `{"session_retention_limit":0}`},
		{"one", `{"session_retention_limit":1}`},
		{"nine", `{"session_retention_limit":9}`},
		{"negative", `{"session_retention_limit":-5}`},
		{"valid_fifty", `{"session_retention_limit":50}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newGlobalSettingsServer(t)
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()
			s.handlePatchGlobalSettings(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("PATCH %s: status = %d, want 200 (silent ignore)", tc.body, rr.Code)
			}
			getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
			getRR := httptest.NewRecorder()
			s.handleGetGlobalSettings(getRR, getReq)
			resp := decodeSettingsResponse(t, getRR)
			if _, ok := resp["session_retention_limit"]; ok {
				t.Errorf("PATCH %s: GET must not include session_retention_limit key", tc.body)
			}
		})
	}
}

// TestHandlePatchGlobalSettings_ResponseBody verifies PATCH returns status field.
func TestHandlePatchGlobalSettings_ResponseBody(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"low_consumption_mode":true}`))
	rr := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "updated" {
		t.Errorf("status = %q, want %q", resp["status"], "updated")
	}
}
