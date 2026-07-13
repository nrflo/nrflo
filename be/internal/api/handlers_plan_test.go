package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
	"be/internal/service"
	"be/internal/types"
)

// --- Case 1 & 2: GET .../plan 404s ---

func TestHandleGetPlan_NoPlanYet_Returns404(t *testing.T) {
	s := newPlanServer(t)
	seedPlanInstance(t, s, "proj-getplan", "inst-getplan")

	rr := httptest.NewRecorder()
	s.handleGetPlan(rr, planGetReq(t, "inst-getplan"))

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleGetPlan_UnknownInstance_Returns404(t *testing.T) {
	s := newPlanServer(t)

	rr := httptest.NewRecorder()
	s.handleGetPlan(rr, planGetReq(t, "no-such-instance"))

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

// --- Case 3: first revise succeeds ---

func TestHandleRevisePlan_FirstRevise_Returns200WithRevision1(t *testing.T) {
	s := newPlanServer(t)
	seedPlanInstance(t, s, "proj-revise1", "inst-revise1")

	body, _ := json.Marshal(types.PlanReviseRequest{
		Revision: 0,
		Manifest: json.RawMessage(validPlanManifestJSON),
	})
	rr := httptest.NewRecorder()
	s.handleRevisePlan(rr, planReviseReq(t, "inst-revise1", body))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	rev := decodePlanRevision(t, rr)
	if rev.Revision != 1 {
		t.Errorf("rev.Revision = %d, want 1", rev.Revision)
	}
	if rev.Author != model.PlanAuthorCaller {
		t.Errorf("rev.Author = %q, want %q", rev.Author, model.PlanAuthorCaller)
	}
}

// --- Case 4: stale revision on revise ---

func TestHandleRevisePlan_StaleRevision_Returns409(t *testing.T) {
	s := newPlanServer(t)
	seedPlanInstance(t, s, "proj-stale", "inst-stale")

	body, _ := json.Marshal(types.PlanReviseRequest{
		Revision: 0,
		Manifest: json.RawMessage(validPlanManifestJSON),
	})
	rr1 := httptest.NewRecorder()
	s.handleRevisePlan(rr1, planReviseReq(t, "inst-stale", body))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first revise status = %d, want 200; body: %s", rr1.Code, rr1.Body.String())
	}

	// Reuse the same (now stale) Revision: 0.
	rr2 := httptest.NewRecorder()
	s.handleRevisePlan(rr2, planReviseReq(t, "inst-stale", body))
	if rr2.Code != http.StatusConflict {
		t.Fatalf("second revise status = %d, want 409; body: %s", rr2.Code, rr2.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr2.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp["error"] == "" {
		t.Error("expected non-empty error field in 409 response")
	}
}

// --- Case 5: invalid manifest ---

func TestHandleRevisePlan_InvalidManifest_Returns400(t *testing.T) {
	s := newPlanServer(t)
	seedPlanInstance(t, s, "proj-invalid", "inst-invalid")

	body, _ := json.Marshal(types.PlanReviseRequest{
		Revision: 0,
		Manifest: json.RawMessage(invalidPlanManifestJSON),
	})
	rr := httptest.NewRecorder()
	s.handleRevisePlan(rr, planReviseReq(t, "inst-invalid", body))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp["error"] == "" {
		t.Error("expected non-empty error field in 400 response")
	}
}

// --- Case 6: stale revision on approve ---

func TestHandleApprovePlan_StaleRevision_Returns409(t *testing.T) {
	s := newPlanServer(t)
	seedPlanInstance(t, s, "proj-appstale", "inst-appstale")

	// revise #1 -> head=1
	body1, _ := json.Marshal(types.PlanReviseRequest{
		Revision: 0,
		Manifest: json.RawMessage(validPlanManifestJSON),
	})
	rr1 := httptest.NewRecorder()
	s.handleRevisePlan(rr1, planReviseReq(t, "inst-appstale", body1))
	if rr1.Code != http.StatusOK {
		t.Fatalf("revise #1 status = %d, want 200; body: %s", rr1.Code, rr1.Body.String())
	}
	rev1 := decodePlanRevision(t, rr1)
	if rev1.Revision != 1 {
		t.Fatalf("rev1.Revision = %d, want 1", rev1.Revision)
	}

	// revise #2 (using the correct running revision 1) -> head=2
	body2, _ := json.Marshal(types.PlanReviseRequest{
		Revision: 1,
		Manifest: json.RawMessage(validPlanManifestJSON),
	})
	rr2 := httptest.NewRecorder()
	s.handleRevisePlan(rr2, planReviseReq(t, "inst-appstale", body2))
	if rr2.Code != http.StatusOK {
		t.Fatalf("revise #2 status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}
	rev2 := decodePlanRevision(t, rr2)
	if rev2.Revision != 2 {
		t.Fatalf("rev2.Revision = %d, want 2", rev2.Revision)
	}

	// approve at stale revision 1 (head is now 2) -> 409
	approveBody, _ := json.Marshal(types.PlanApproveRequest{Revision: 1})
	rr3 := httptest.NewRecorder()
	s.handleApprovePlan(rr3, planApproveReq(t, "inst-appstale", approveBody))
	if rr3.Code != http.StatusConflict {
		t.Errorf("approve status = %d, want 409; body: %s", rr3.Code, rr3.Body.String())
	}
}

// --- Case 7: approve at correct head ---

func TestHandleApprovePlan_CorrectRevision_Returns200AndApproves(t *testing.T) {
	s := newPlanServer(t)
	seedPlanInstance(t, s, "proj-approve", "inst-approve")

	body, _ := json.Marshal(types.PlanReviseRequest{
		Revision: 0,
		Manifest: json.RawMessage(validPlanManifestJSON),
	})
	rr1 := httptest.NewRecorder()
	s.handleRevisePlan(rr1, planReviseReq(t, "inst-approve", body))
	if rr1.Code != http.StatusOK {
		t.Fatalf("revise status = %d, want 200; body: %s", rr1.Code, rr1.Body.String())
	}
	rev1 := decodePlanRevision(t, rr1)

	approveBody, _ := json.Marshal(types.PlanApproveRequest{Revision: rev1.Revision})
	rr2 := httptest.NewRecorder()
	s.handleApprovePlan(rr2, planApproveReq(t, "inst-approve", approveBody))
	if rr2.Code != http.StatusOK {
		t.Fatalf("approve status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}
	_ = decodePlanRevision(t, rr2)

	// Follow-up GET shows Head.Status == "approved".
	getRR := httptest.NewRecorder()
	s.handleGetPlan(getRR, planGetReq(t, "inst-approve"))
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET plan status = %d, want 200; body: %s", getRR.Code, getRR.Body.String())
	}
	var draft service.PlanDraft
	if err := json.NewDecoder(getRR.Body).Decode(&draft); err != nil {
		t.Fatalf("decode PlanDraft: %v", err)
	}
	if draft.Head == nil {
		t.Fatal("draft.Head is nil")
	}
	if draft.Head.Status != model.PlanStatusApproved {
		t.Errorf("draft.Head.Status = %q, want %q", draft.Head.Status, model.PlanStatusApproved)
	}
}

// --- Case 8: plan routes on a __global__-project instance are NOT gated by
// denyNonAdminGlobalWrite (removed in DYNWF-6) ---

// TestPlanRoutes_GlobalProjectInstance_WritesAllowedForBearerLikeCaller is the
// regression test for the removed denyNonAdminGlobalWrite guard on plan
// revise/approve/cancel: these are per-instance RUNTIME operations (not
// __global__ definition mutations), so a spawned agent's spawn token or an
// mcp-external service token — neither of which populates a human user in
// request context — must be able to drive a dynamic_workflow run living in
// the hidden __global__ project. Simulates that principal by stashing a
// ServicePrincipal in the request context (mirroring what requireAuth does
// for a bearer request) with no user at all; previously
// denyNonAdminGlobalWrite required a human admin user and would 403 this.
func TestPlanRoutes_GlobalProjectInstance_WritesAllowedForBearerLikeCaller(t *testing.T) {
	s := newPlanServer(t)
	seedPlanInstance(t, s, service.GlobalProjectID, "inst-global")

	asServicePrincipal := func(r *http.Request) *http.Request {
		ctx := context.WithValue(r.Context(), servicePrincipalKey, &ServicePrincipal{TokenID: "tok-1", Global: true})
		return r.WithContext(ctx)
	}

	// Write route (revise) with a service-principal (no user) context -> 200, not 403.
	body, _ := json.Marshal(types.PlanReviseRequest{
		Revision: 0,
		Manifest: json.RawMessage(validPlanManifestJSON),
	})
	rr := httptest.NewRecorder()
	s.handleRevisePlan(rr, asServicePrincipal(planReviseReq(t, "inst-global", body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("revise status = %d, want 200 (denyNonAdminGlobalWrite must not gate plan routes); body: %s", rr.Code, rr.Body.String())
	}
	rev := decodePlanRevision(t, rr)

	// Write route (approve) -> 200, not 403.
	approveBody, _ := json.Marshal(types.PlanApproveRequest{Revision: rev.Revision})
	rrApprove := httptest.NewRecorder()
	s.handleApprovePlan(rrApprove, asServicePrincipal(planApproveReq(t, "inst-global", approveBody)))
	if rrApprove.Code != http.StatusOK {
		t.Errorf("approve status = %d, want 200; body: %s", rrApprove.Code, rrApprove.Body.String())
	}

	// Read route (GET plan) is unaffected either way.
	getRR := httptest.NewRecorder()
	s.handleGetPlan(getRR, planGetReq(t, "inst-global"))
	if getRR.Code != http.StatusOK {
		t.Errorf("GET plan status = %d, want 200; body: %s", getRR.Code, getRR.Body.String())
	}
}

// TestPlanRoutes_GlobalProjectInstance_CancelAllowedForBearerLikeCaller
// covers the cancel route separately (needs its own draft instance since
// cancel only applies to a draft head, not an already-approved one).
func TestPlanRoutes_GlobalProjectInstance_CancelAllowedForBearerLikeCaller(t *testing.T) {
	s := newPlanServer(t)
	seedPlanInstance(t, s, service.GlobalProjectID, "inst-global-cancel")

	body, _ := json.Marshal(types.PlanReviseRequest{
		Revision: 0,
		Manifest: json.RawMessage(validPlanManifestJSON),
	})
	rr := httptest.NewRecorder()
	s.handleRevisePlan(rr, planReviseReq(t, "inst-global-cancel", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("revise status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-instances/inst-global-cancel/plan/cancel", nil)
	cancelReq.SetPathValue("iid", "inst-global-cancel")
	rrCancel := httptest.NewRecorder()
	s.handleCancelPlan(rrCancel, cancelReq)
	if rrCancel.Code != http.StatusOK {
		t.Errorf("cancel status = %d, want 200 (not 403); body: %s", rrCancel.Code, rrCancel.Body.String())
	}
}
