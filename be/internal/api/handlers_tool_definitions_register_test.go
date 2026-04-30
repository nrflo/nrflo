package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// registerResp mirrors registerToolsResponse for test decoding.
type registerResp struct {
	ToolsRegistered   int      `json:"tools_registered"`
	ToolsPruned       int      `json:"tools_pruned"`
	ToolsSkippedInUse []string `json:"tools_skipped_in_use"`
}

// doRegister sends a POST to handleRegisterToolDefinitions.
// Pass authHeader="" to omit the Authorization header entirely.
func doRegister(t *testing.T, s *Server, authHeader, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tool-definitions/register", strings.NewReader(body))
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rr := httptest.NewRecorder()
	s.handleRegisterToolDefinitions(rr, req)
	return rr
}

// decodeRegisterResp decodes a successful register response body.
func decodeRegisterResp(t *testing.T, rr *httptest.ResponseRecorder) registerResp {
	t.Helper()
	var r registerResp
	if err := json.NewDecoder(rr.Body).Decode(&r); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	return r
}

// TestHandleRegisterToolDefinitions_NoEnvVar verifies 503 when NRFLO_REGISTER_TOKEN is empty.
func TestHandleRegisterToolDefinitions_NoEnvVar(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "")
	s := newToolDefServer(t)
	rr := doRegister(t, s, "Bearer secret", `{"tools":[]}`)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

// TestHandleRegisterToolDefinitions_Auth verifies all auth failure modes return 401.
func TestHandleRegisterToolDefinitions_Auth(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)

	cases := []struct {
		name       string
		authHeader string
	}{
		{"missing_header", ""},
		{"wrong_scheme", "Token secret"},
		{"basic_scheme", "Basic c2VjcmV0"},
		{"wrong_token", "Bearer wrong"},
		{"empty_bearer", "Bearer "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := doRegister(t, s, tc.authHeader, `{"tools":[]}`)
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("auth=%q -> status = %d, want 401", tc.authHeader, rr.Code)
			}
		})
	}
}

// TestHandleRegisterToolDefinitions_NullTools verifies 400 when tools is JSON null.
func TestHandleRegisterToolDefinitions_NullTools(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)
	rr := doRegister(t, s, "Bearer secret", `{"tools":null}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandleRegisterToolDefinitions_ToolsMissingKey verifies 400 when tools key is absent from body.
func TestHandleRegisterToolDefinitions_ToolsMissingKey(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)
	rr := doRegister(t, s, "Bearer secret", `{}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandleRegisterToolDefinitions_ToolsNotArray verifies 400 when tools is not an array.
func TestHandleRegisterToolDefinitions_ToolsNotArray(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)
	rr := doRegister(t, s, "Bearer secret", `{"tools":"not an array"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandleRegisterToolDefinitions_EmptyToolsArray verifies 200 with 0 tools registered on [].
func TestHandleRegisterToolDefinitions_EmptyToolsArray(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)
	rr := doRegister(t, s, "Bearer secret", `{"tools":[]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeRegisterResp(t, rr)
	if resp.ToolsRegistered != 0 {
		t.Errorf("tools_registered = %d, want 0", resp.ToolsRegistered)
	}
	if resp.ToolsPruned != 0 {
		t.Errorf("tools_pruned = %d, want 0", resp.ToolsPruned)
	}
}

// TestHandleRegisterToolDefinitions_HappyPath_OneTool registers a single tool.
func TestHandleRegisterToolDefinitions_HappyPath_OneTool(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)

	body := `{"tools":[{"name":"echo","endpoint":"http://x/echo","input_schema":{"type":"object"}}]}`
	rr := doRegister(t, s, "Bearer secret", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeRegisterResp(t, rr)
	if resp.ToolsRegistered != 1 {
		t.Errorf("tools_registered = %d, want 1", resp.ToolsRegistered)
	}
	if resp.ToolsPruned != 0 {
		t.Errorf("tools_pruned = %d, want 0", resp.ToolsPruned)
	}
	if len(resp.ToolsSkippedInUse) != 0 {
		t.Errorf("tools_skipped_in_use = %v, want []", resp.ToolsSkippedInUse)
	}
}

// TestHandleRegisterToolDefinitions_HappyPath_ThreeTools registers multiple tools in one call.
func TestHandleRegisterToolDefinitions_HappyPath_ThreeTools(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)

	body := `{"tools":[
		{"name":"alpha","endpoint":"http://x/alpha","input_schema":{"type":"object"}},
		{"name":"beta","endpoint":"http://x/beta","input_schema":{"type":"object"},"description":"b"},
		{"name":"gamma","endpoint":"http://x/gamma","input_schema":{"type":"object"},"timeout_sec":60}
	]}`
	rr := doRegister(t, s, "Bearer secret", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeRegisterResp(t, rr)
	if resp.ToolsRegistered != 3 {
		t.Errorf("tools_registered = %d, want 3", resp.ToolsRegistered)
	}
	if resp.ToolsPruned != 0 {
		t.Errorf("tools_pruned = %d, want 0", resp.ToolsPruned)
	}
}

// TestHandleRegisterToolDefinitions_Idempotent verifies re-POST updates the existing tool record.
func TestHandleRegisterToolDefinitions_Idempotent(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)

	body1 := `{"tools":[{"name":"echo","endpoint":"http://x/echo","input_schema":{"type":"object"}}]}`
	if rr := doRegister(t, s, "Bearer secret", body1); rr.Code != http.StatusOK {
		t.Fatalf("first register status = %d; body=%s", rr.Code, rr.Body.String())
	}

	body2 := `{"tools":[{"name":"echo","endpoint":"http://x/echo-v2","input_schema":{"type":"object","required":["x"]}}]}`
	rr := doRegister(t, s, "Bearer secret", body2)
	if rr.Code != http.StatusOK {
		t.Fatalf("second register status = %d; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeRegisterResp(t, rr)
	if resp.ToolsRegistered != 1 {
		t.Errorf("tools_registered = %d, want 1", resp.ToolsRegistered)
	}
	if resp.ToolsPruned != 0 {
		t.Errorf("tools_pruned = %d, want 0 (same tool updated, not pruned)", resp.ToolsPruned)
	}

	// Verify endpoint was updated via list.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tool-definitions", nil)
	lrr := httptest.NewRecorder()
	s.handleListToolDefinitions(lrr, req)
	if lrr.Code != http.StatusOK {
		t.Fatalf("list status = %d", lrr.Code)
	}
	got := decodeToolDefList(t, lrr)
	if len(got) != 1 {
		t.Fatalf("list len = %d, want 1", len(got))
	}
	if got[0].Endpoint != "http://x/echo-v2" {
		t.Errorf("endpoint = %q, want http://x/echo-v2", got[0].Endpoint)
	}
}
