package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleGetGlobalSettings_SimplifiedAgentsGraphDefaultFalse verifies fresh DB returns false.
func TestHandleGetGlobalSettings_SimplifiedAgentsGraphDefaultFalse(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200", rr.Code)
	}

	resp := decodeSettingsResponse(t, rr)
	v, ok := resp["simplified_agents_graph"]
	if !ok {
		t.Fatal("response missing simplified_agents_graph field")
	}
	if v != false {
		t.Errorf("simplified_agents_graph = %v, want false", v)
	}
}

// TestHandlePatchGlobalSettings_SimplifiedAgentsGraphEnableThenGet verifies PATCH sets to true and GET reflects it.
func TestHandlePatchGlobalSettings_SimplifiedAgentsGraphEnableThenGet(t *testing.T) {
	s := newGlobalSettingsServer(t)

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"simplified_agents_graph":true}`))
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
	if v, ok := resp["simplified_agents_graph"]; !ok {
		t.Error("response missing simplified_agents_graph")
	} else if v != true {
		t.Errorf("simplified_agents_graph = %v, want true", v)
	}
}

// TestHandlePatchGlobalSettings_SimplifiedAgentsGraphToggle verifies enable then disable works correctly.
func TestHandlePatchGlobalSettings_SimplifiedAgentsGraphToggle(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"simplified_agents_graph":true}`))
	rr1 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("enable PATCH status = %d, want 200", rr1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"simplified_agents_graph":false}`))
	rr2 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("disable PATCH status = %d, want 200", rr2.Code)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr3 := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr3, req3)

	resp := decodeSettingsResponse(t, rr3)
	if v, ok := resp["simplified_agents_graph"]; !ok {
		t.Error("response missing simplified_agents_graph")
	} else if v != false {
		t.Errorf("after toggle off, simplified_agents_graph = %v, want false", v)
	}
}
