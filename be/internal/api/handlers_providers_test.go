package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/model"
)

func newProvidersServer(t *testing.T) *Server {
	t.Helper()
	return newCLIModelsServer(t)
}

// --- GET /api/v1/providers ---

func TestHandleListProviders_Defaults(t *testing.T) {
	s := newProvidersServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	rr := httptest.NewRecorder()
	s.handleListProviders(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	wantProviders := []string{"claude", "codex", "opencode"}
	if len(resp) != len(wantProviders) {
		t.Fatalf("response has %d providers, want %d", len(resp), len(wantProviders))
	}
	wantModes := map[string][]string{
		"claude":   {"cli", "cli_interactive"},
		"codex":    {"cli", "cli_interactive"},
		"opencode": {"cli"},
	}
	for _, p := range wantProviders {
		entry, ok := resp[p]
		if !ok {
			t.Errorf("missing provider %q in response", p)
			continue
		}
		modesRaw, ok := entry["modes"]
		if !ok {
			t.Errorf("provider %q: missing modes key", p)
			continue
		}
		modeSlice, ok := modesRaw.([]interface{})
		if !ok {
			t.Errorf("provider %q: modes is not array, got %T", p, modesRaw)
			continue
		}
		want := wantModes[p]
		if len(modeSlice) != len(want) {
			t.Errorf("provider %q: len(modes) = %d, want %d", p, len(modeSlice), len(want))
			continue
		}
		for i, m := range want {
			if modeSlice[i].(string) != m {
				t.Errorf("provider %q: modes[%d] = %q, want %q", p, i, modeSlice[i], m)
			}
		}
	}
}

// --- PATCH /api/v1/providers/{name} ---

func TestHandlePatchProvider_HappyPath_ThenGetReflects(t *testing.T) {
	s := newProvidersServer(t)

	body := `{"modes":["cli_interactive"]}`
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/providers/claude", strings.NewReader(body))
	patchReq.SetPathValue("name", "claude")
	patchRR := httptest.NewRecorder()
	s.handlePatchProvider(patchRR, patchReq)

	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200; body: %s", patchRR.Code, patchRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	getRR := httptest.NewRecorder()
	s.handleListProviders(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", getRR.Code)
	}
	var resp map[string]map[string]interface{}
	if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	modesRaw, ok := resp["claude"]["modes"].([]interface{})
	if !ok {
		t.Fatalf("claude.modes is not array")
	}
	if len(modesRaw) != 1 || modesRaw[0].(string) != "cli_interactive" {
		t.Errorf("after PATCH claude modes = %v, want [cli_interactive]", modesRaw)
	}
	// Other providers should be unchanged defaults: codex has 2 modes, opencode has 1.
	codexModes := resp["codex"]["modes"].([]interface{})
	if len(codexModes) != 2 {
		t.Errorf("codex modes after claude PATCH = %v, want 2 defaults", codexModes)
	}
	opencodeModes := resp["opencode"]["modes"].([]interface{})
	if len(opencodeModes) != 1 || opencodeModes[0].(string) != "cli" {
		t.Errorf("opencode modes after claude PATCH = %v, want [cli]", opencodeModes)
	}
}

func TestHandlePatchProvider_AllProvidersUpdatable(t *testing.T) {
	providers := []string{"claude", "codex", "opencode"}
	for _, p := range providers {
		p := p
		t.Run(p, func(t *testing.T) {
			s := newProvidersServer(t)
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/providers/"+p, strings.NewReader(`{"modes":["cli"]}`))
			req.SetPathValue("name", p)
			rr := httptest.NewRecorder()
			s.handlePatchProvider(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("PATCH %s status = %d, want 200; body: %s", p, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandlePatchProvider_InvalidBody_Returns400(t *testing.T) {
	s := newProvidersServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/providers/claude", strings.NewReader("not json"))
	req.SetPathValue("name", "claude")
	rr := httptest.NewRecorder()
	s.handlePatchProvider(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandlePatchProvider_ValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		pname   string
		body    string
		wantErr string
	}{
		{
			name:    "unknown provider",
			pname:   "gpt4",
			body:    `{"modes":["cli"]}`,
			wantErr: "invalid provider",
		},
		{
			name:    "empty modes array",
			pname:   "claude",
			body:    `{"modes":[]}`,
			wantErr: "must not be empty",
		},
		{
			name:    "api mode rejected",
			pname:   "claude",
			body:    `{"modes":["api"]}`,
			wantErr: "invalid mode",
		},
		{
			name:    "script mode rejected",
			pname:   "codex",
			body:    `{"modes":["script"]}`,
			wantErr: "invalid mode",
		},
		{
			name:    "mixed valid and invalid mode",
			pname:   "opencode",
			body:    `{"modes":["cli","script"]}`,
			wantErr: "invalid mode",
		},
		{
			name:    "opencode rejects cli_interactive",
			pname:   "opencode",
			body:    `{"modes":["cli","cli_interactive"]}`,
			wantErr: "does not support cli_interactive",
		},
		{
			name:    "opencode rejects cli_interactive only",
			pname:   "opencode",
			body:    `{"modes":["cli_interactive"]}`,
			wantErr: "does not support cli_interactive",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := newProvidersServer(t)
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/providers/"+tc.pname, strings.NewReader(tc.body))
			req.SetPathValue("name", tc.pname)
			rr := httptest.NewRecorder()
			s.handlePatchProvider(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400; body: %s", tc.name, rr.Code, rr.Body.String())
			}
			assertErrorContains(t, rr, tc.wantErr)
		})
	}
}

// TestHandlePatchProvider_OpencodeRejectsCLIInteractive verifies end-to-end that
// PATCH /api/v1/providers/opencode with cli_interactive returns 400, while cli succeeds.
func TestHandlePatchProvider_OpencodeRejectsCLIInteractive(t *testing.T) {
	t.Parallel()

	t.Run("cli_interactive rejected 400", func(t *testing.T) {
		t.Parallel()
		s := newProvidersServer(t)
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/providers/opencode", strings.NewReader(`{"modes":["cli","cli_interactive"]}`))
		req.SetPathValue("name", "opencode")
		rr := httptest.NewRecorder()
		s.handlePatchProvider(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
		}
		assertErrorContains(t, rr, "does not support cli_interactive")
	})

	t.Run("cli accepted 200", func(t *testing.T) {
		t.Parallel()
		s := newProvidersServer(t)
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/providers/opencode", strings.NewReader(`{"modes":["cli"]}`))
		req.SetPathValue("name", "opencode")
		rr := httptest.NewRecorder()
		s.handlePatchProvider(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
		}
	})
}

func TestHandlePatchProvider_NonAdmin_Returns403(t *testing.T) {
	s := newServerWithAuth(t)
	uid := createTestUser(t, s, "viewer-provider@test.com", model.UserRoleViewer, false)
	cookie := injectSession(t, s, uid)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/providers/claude", strings.NewReader(`{"modes":["cli"]}`))
	req.SetPathValue("name", "claude")
	req.AddCookie(cookie)

	handler := s.sessionMgr.LoadAndSave(s.requireAdmin(http.HandlerFunc(s.handlePatchProvider)))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("viewer PATCH: status = %d, want 403", rr.Code)
	}
}
