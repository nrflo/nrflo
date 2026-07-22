package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/ws"
)

// newTierModelsServer creates a minimal Server for tier-models handler tests,
// backed by the shared api-package template DB (migration-seeded tier1/tier4
// chains intact — mirrors newSystemAgentServerWithSeeds).
func newTierModelsServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tier_models_handler_test.db")
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

func decodeTierModelList(t *testing.T, rr *httptest.ResponseRecorder) []model.TierModel {
	t.Helper()
	var rows []model.TierModel
	if err := json.NewDecoder(rr.Body).Decode(&rows); err != nil {
		t.Fatalf("decode tier model list: %v", err)
	}
	return rows
}

// --- GET ---

// TestHandleListTierModels_ReturnsSeededRows verifies GET returns the
// migration-seeded tier1/tier4 chains.
func TestHandleListTierModels_ReturnsSeededRows(t *testing.T) {
	s := newTierModelsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tier-models", nil)
	rr := httptest.NewRecorder()
	s.handleListTierModels(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	rows := decodeTierModelList(t, rr)
	if len(rows) == 0 {
		t.Fatal("expected seeded tier_models rows, got empty list")
	}
	found1, found4 := false, false
	for _, r := range rows {
		if r.Tier == 1 {
			found1 = true
		}
		if r.Tier == 4 {
			found4 = true
		}
	}
	if !found1 || !found4 {
		t.Errorf("expected seeded tier=1 and tier=4 rows, found1=%v found4=%v", found1, found4)
	}
}

// --- PUT ---

// TestHandleSetTierChain_ReplaceSucceedsAndBroadcasts verifies a valid PUT
// returns 200 and broadcasts tier_models.updated over the WS hub.
func TestHandleSetTierChain_ReplaceSucceedsAndBroadcasts(t *testing.T) {
	s := newTierModelsServer(t)
	hub := ws.NewHub(clock.Real())
	s.wsHub = hub
	go hub.Run()
	t.Cleanup(hub.Stop)
	client, ch := ws.NewTestClient(hub, "tier-model-events")
	hub.Register(client)
	hub.Subscribe(client, "", "")
	defer hub.Unregister(client)

	body := `{"entries":[{"execution_mode":"api","model_id":"opus-4-7"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tier-models/2", strings.NewReader(body))
	req.SetPathValue("tier", "2")
	rr := httptest.NewRecorder()
	s.handleSetTierChain(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != ws.EventTierModelsUpdated {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventTierModelsUpdated)
		}
		if tier, ok := event.Data["tier"].(float64); !ok || int(tier) != 2 {
			t.Errorf("event.Data[tier] = %v, want 2", event.Data["tier"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tier_models.updated broadcast")
	}

	// Verify the replace persisted via GET.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/tier-models", nil)
	getRR := httptest.NewRecorder()
	s.handleListTierModels(getRR, getReq)
	rows := decodeTierModelList(t, getRR)
	found := false
	for _, r := range rows {
		if r.Tier == 2 && r.Position == 0 && r.ModelID == "opus-4-7" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected tier=2 pos=0 model=opus-4-7 after PUT, rows=%+v", rows)
	}
}

// TestHandleSetTierChain_InvalidModel400 verifies an unknown model_id in the
// request body returns 400.
func TestHandleSetTierChain_InvalidModel400(t *testing.T) {
	s := newTierModelsServer(t)

	body := `{"entries":[{"execution_mode":"api","model_id":"no-such-model"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tier-models/2", strings.NewReader(body))
	req.SetPathValue("tier", "2")
	rr := httptest.NewRecorder()
	s.handleSetTierChain(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "invalid model")
}

// TestHandleSetTierChain_TierOutOfRange400 verifies a tier outside [1,5] in
// the path returns 400.
func TestHandleSetTierChain_TierOutOfRange400(t *testing.T) {
	s := newTierModelsServer(t)

	body := `{"entries":[]}`
	for _, tier := range []string{"0", "6"} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/tier-models/"+tier, strings.NewReader(body))
		req.SetPathValue("tier", tier)
		rr := httptest.NewRecorder()
		s.handleSetTierChain(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("tier=%s: status = %d, want 400; body: %s", tier, rr.Code, rr.Body.String())
		}
	}
}

// TestHandleSetTierChain_NonNumericTier400 verifies a non-numeric tier path
// segment returns 400 rather than panicking or 500ing.
func TestHandleSetTierChain_NonNumericTier400(t *testing.T) {
	s := newTierModelsServer(t)

	body := `{"entries":[]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tier-models/abc", strings.NewReader(body))
	req.SetPathValue("tier", "abc")
	rr := httptest.NewRecorder()
	s.handleSetTierChain(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleSetTierChain_EmptyEntriesClears verifies PUT with an empty
// entries array clears the tier (200, not an error).
func TestHandleSetTierChain_EmptyEntriesClears(t *testing.T) {
	s := newTierModelsServer(t)

	// Tier 1 arrives pre-seeded (migration 000195).
	body := `{"entries":[]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tier-models/1", strings.NewReader(body))
	req.SetPathValue("tier", "1")
	rr := httptest.NewRecorder()
	s.handleSetTierChain(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/tier-models", nil)
	getRR := httptest.NewRecorder()
	s.handleListTierModels(getRR, getReq)
	rows := decodeTierModelList(t, getRR)
	for _, r := range rows {
		if r.Tier == 1 {
			t.Errorf("tier=1 row survived empty-entries PUT: %+v", r)
		}
	}
}

// --- Auth gating ---

// TestPutTierChain_NonAdminForbidden verifies a non-admin viewer session
// receives 403 on the admin-gated PUT route.
func TestPutTierChain_NonAdminForbidden(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "tiermodel-viewer@test.com", "pass", model.UserRoleViewer, false)
	mustLogin(t, as, "tiermodel-viewer@test.com", "pass")

	putReq, err := http.NewRequest(http.MethodPut, as.baseURL+"/api/v1/tier-models/1", strings.NewReader(`{"entries":[]}`))
	if err != nil {
		t.Fatalf("build PUT request: %v", err)
	}
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := as.client.Do(putReq)
	if err != nil {
		t.Fatalf("PUT /tier-models/1: %v", err)
	}
	defer drain(putResp)
	if putResp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer PUT status = %d, want 403", putResp.StatusCode)
	}
}

// TestGetTierModels_ViewerAllowed verifies the read route is protected (any
// authenticated user), not admin-gated.
func TestGetTierModels_ViewerAllowed(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "tiermodel-reader@test.com", "pass", model.UserRoleViewer, false)
	mustLogin(t, as, "tiermodel-reader@test.com", "pass")

	resp, err := as.client.Get(as.baseURL + "/api/v1/tier-models")
	if err != nil {
		t.Fatalf("GET /tier-models: %v", err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("viewer GET status = %d, want 200", resp.StatusCode)
	}
}
