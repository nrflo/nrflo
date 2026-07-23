package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"be/internal/model"
)

// TestHandleCreateAgentDef_TierRoundTrip verifies POSTing a tier-only def
// (no model) persists tier and leaves model empty.
func TestHandleCreateAgentDef_TierRoundTrip(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, false)

	rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"tier-agent","prompt":"do stuff","tier":2}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}

	var def model.AgentDefinition
	if err := json.NewDecoder(rr.Body).Decode(&def); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if def.Model != "" {
		t.Errorf("Model = %q, want '' (tier-driven)", def.Model)
	}
	if def.Tier == nil || *def.Tier != 2 {
		t.Errorf("Tier = %v, want 2", def.Tier)
	}
}

// TestHandleUpdateAgentDef_TierRoundTrip verifies PATCHing tier onto an
// existing override-model def is persisted.
func TestHandleUpdateAgentDef_TierRoundTrip(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, false)

	if rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"patch-tier-agent","prompt":"do stuff","model":"sonnet-5"}`); rr.Code != http.StatusCreated {
		t.Fatalf("setup: create status = %d, body=%s", rr.Code, rr.Body.String())
	}

	rr := patchAgentDefRequest(t, s, pid, wid, "patch-tier-agent", `{"model":"","tier":3}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var dbModel string
	var dbTier *int
	row := s.pool.QueryRow(`SELECT model, tier FROM agent_definitions WHERE project_id=? AND workflow_id=? AND id=?`, pid, wid, "patch-tier-agent")
	if err := row.Scan(&dbModel, &dbTier); err != nil {
		t.Fatalf("query: %v", err)
	}
	if dbModel != "" {
		t.Errorf("model = %q, want '' after clearing override", dbModel)
	}
	if dbTier == nil || *dbTier != 3 {
		t.Errorf("tier = %v, want 3", dbTier)
	}
}

// TestHandleUpdateAgentDef_ClearBothModelAndTier400 verifies clearing model
// with no tier present (neither already set nor in this request) is
// rejected with 400.
func TestHandleUpdateAgentDef_ClearBothModelAndTier400(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, false)

	if rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"clear-both-agent","prompt":"do stuff","model":"sonnet-5"}`); rr.Code != http.StatusCreated {
		t.Fatalf("setup: create status = %d, body=%s", rr.Code, rr.Body.String())
	}

	rr := patchAgentDefRequest(t, s, pid, wid, "clear-both-agent", `{"model":""}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "model or tier is required")
}
