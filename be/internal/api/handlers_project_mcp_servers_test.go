package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func doMCPServersRequest(t *testing.T, s *Server, method, projectID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/projects/"+projectID+"/settings/mcp-servers", strings.NewReader(body))
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()
	if method == http.MethodGet {
		s.handleGetProjectMCPServers(rr, req)
	} else {
		s.handlePutProjectMCPServers(rr, req)
	}
	return rr
}

func decodeMCPServersResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("decode mcp servers response: %v", err)
	}
	return m
}

func TestProjectMCPServersRoundTrip(t *testing.T) {
	s, projectID := newProjectSettingsServer(t)

	rr := doMCPServersRequest(t, s, http.MethodGet, projectID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET unset: %d %s", rr.Code, rr.Body.String())
	}
	if got := string(decodeMCPServersResponse(t, rr)["servers"]); got != "null" {
		t.Fatalf("unset servers = %s, want null", got)
	}

	body := `{"servers":{"unity":{"command":"uv","args":["run","server.py"]}}}`
	rr = doMCPServersRequest(t, s, http.MethodPut, projectID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rr.Code, rr.Body.String())
	}

	rr = doMCPServersRequest(t, s, http.MethodGet, projectID, "")
	servers := decodeMCPServersResponse(t, rr)["servers"]
	if !strings.Contains(string(servers), `"unity"`) {
		t.Fatalf("GET after PUT missing unity: %s", servers)
	}

	// Clearing with null empties the stored config.
	rr = doMCPServersRequest(t, s, http.MethodPut, projectID, `{"servers":null}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT null: %d %s", rr.Code, rr.Body.String())
	}
	rr = doMCPServersRequest(t, s, http.MethodGet, projectID, "")
	if got := string(decodeMCPServersResponse(t, rr)["servers"]); got != "null" {
		t.Fatalf("cleared servers = %s, want null", got)
	}
}

func TestProjectMCPServersValidation(t *testing.T) {
	s, projectID := newProjectSettingsServer(t)
	for name, body := range map[string]string{
		"reserved name":   `{"servers":{"nrflo":{"command":"x"}}}`,
		"missing command": `{"servers":{"unity":{}}}`,
		"bad name":        `{"servers":{"bad name":{"command":"x"}}}`,
	} {
		rr := doMCPServersRequest(t, s, http.MethodPut, projectID, body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400 (%s)", name, rr.Code, rr.Body.String())
		}
	}
}
