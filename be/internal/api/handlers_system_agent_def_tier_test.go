package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleCreateSystemAgentDef_EmptyModelRequiresTier verifies that omitting
// both model and tier is rejected with 400 (UI's tier selector always sends
// one or the other).
func TestHandleCreateSystemAgentDef_EmptyModelRequiresTier(t *testing.T) {
	s := newSystemAgentServer(t)
	body := `{"id":"no-model-no-tier","prompt":"p"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "model or tier is required")
}

// TestHandleCreateSystemAgentDef_EmptyModelWithTierSucceeds verifies that an
// empty model with a tier set succeeds and persists the tier.
func TestHandleCreateSystemAgentDef_EmptyModelWithTierSucceeds(t *testing.T) {
	s := newSystemAgentServer(t)
	body := `{"id":"tiered-agent","prompt":"p","tier":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	def := decodeSystemAgentDef(t, rr)
	if def.Model != "" {
		t.Errorf("Model = %q, want empty (tier-only)", def.Model)
	}
	if def.Tier == nil || *def.Tier != 1 {
		t.Errorf("Tier = %v, want 1", def.Tier)
	}
}

// TestHandleUpdateSystemAgentDef_ModelEmptyWithTierClearsOverride verifies
// that PATCHing Model="" while a tier is set on the same request clears the
// model override (system_agent_definition_validate.go's cross-field
// invariant treats the effective tier as populated).
func TestHandleUpdateSystemAgentDef_ModelEmptyWithTierClearsOverride(t *testing.T) {
	s := newSystemAgentServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents",
		strings.NewReader(`{"id":"clear-override","prompt":"p","model":"haiku-4-5"}`))
	s.handleCreateSystemAgentDef(httptest.NewRecorder(), createReq)

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/system-agents/clear-override",
		strings.NewReader(`{"model":"","tier":2}`))
	patchReq.SetPathValue("id", "clear-override")
	rr := httptest.NewRecorder()
	s.handleUpdateSystemAgentDef(rr, patchReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents/clear-override", nil)
	getReq.SetPathValue("id", "clear-override")
	getRR := httptest.NewRecorder()
	s.handleGetSystemAgentDef(getRR, getReq)
	def := decodeSystemAgentDef(t, getRR)
	if def.Model != "" {
		t.Errorf("Model = %q, want empty after clearing override", def.Model)
	}
	if def.Tier == nil || *def.Tier != 2 {
		t.Errorf("Tier = %v, want 2", def.Tier)
	}
}

// TestHandleUpdateSystemAgentDef_ModelEmptyNoTier400 verifies that PATCHing
// Model="" with no tier present (neither already set nor in this request)
// returns 400 rather than persisting a dangling def with no model and no
// tier.
func TestHandleUpdateSystemAgentDef_ModelEmptyNoTier400(t *testing.T) {
	s := newSystemAgentServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents",
		strings.NewReader(`{"id":"no-tier-clear","prompt":"p","model":"haiku-4-5"}`))
	s.handleCreateSystemAgentDef(httptest.NewRecorder(), createReq)

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/system-agents/no-tier-clear",
		strings.NewReader(`{"model":""}`))
	patchReq.SetPathValue("id", "no-tier-clear")
	rr := httptest.NewRecorder()
	s.handleUpdateSystemAgentDef(rr, patchReq)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "model or tier is required")
}
