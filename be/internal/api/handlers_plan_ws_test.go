package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
	"be/internal/ws"
)

// planWSEnv is a test env with a running wsHub for plan handler broadcast tests.
type planWSEnv struct {
	s   *Server
	rec *wsRecorder
}

func newPlanWSEnv(t *testing.T) *planWSEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "plan_ws_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	hub := ws.NewHub(clock.Real())
	rec := &wsRecorder{ch: make(chan *ws.Event, 32)}
	hub.RegisterListener(rec)
	go hub.Run()
	t.Cleanup(func() {
		hub.Stop()
		pool.Close()
	})
	return &planWSEnv{
		s:   &Server{pool: pool, clock: clock.Real(), wsHub: hub},
		rec: rec,
	}
}

// TestPlanRoutes_BroadcastsLifecycleEvents drives revise -> revise -> approve
// -> cancel and asserts each lifecycle transition emits the correct WS event.
func TestPlanRoutes_BroadcastsLifecycleEvents(t *testing.T) {
	env := newPlanWSEnv(t)
	const pid = "proj-plan-ws"
	const iid = "inst-plan-ws"
	seedPlanInstance(t, env.s, pid, iid)

	// First revise (revision 0 -> 1) should fire plan.drafted, not plan.revised.
	body1, _ := json.Marshal(types.PlanReviseRequest{
		Revision: 0,
		Manifest: json.RawMessage(validPlanManifestJSON),
	})
	rr1 := httptest.NewRecorder()
	env.s.handleRevisePlan(rr1, planReviseReq(t, iid, body1))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first revise status = %d, want 200; body: %s", rr1.Code, rr1.Body.String())
	}
	drafted := env.rec.waitEvent(t, ws.EventPlanDrafted)
	if drafted.ProjectID != pid {
		t.Errorf("plan.drafted event.ProjectID = %q, want %q", drafted.ProjectID, pid)
	}

	// Second revise (revision 1 -> 2) should fire plan.revised.
	body2, _ := json.Marshal(types.PlanReviseRequest{
		Revision: 1,
		Manifest: json.RawMessage(validPlanManifestJSON),
	})
	rr2 := httptest.NewRecorder()
	env.s.handleRevisePlan(rr2, planReviseReq(t, iid, body2))
	if rr2.Code != http.StatusOK {
		t.Fatalf("second revise status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}
	rev2 := decodePlanRevision(t, rr2)
	if rev2.Revision != 2 {
		t.Fatalf("rev2.Revision = %d, want 2", rev2.Revision)
	}
	env.rec.waitEvent(t, ws.EventPlanRevised)

	// Approve at the correct head (revision 2) should fire plan.approved.
	approveBody, _ := json.Marshal(types.PlanApproveRequest{Revision: rev2.Revision})
	rrApprove := httptest.NewRecorder()
	env.s.handleApprovePlan(rrApprove, planApproveReq(t, iid, approveBody))
	if rrApprove.Code != http.StatusOK {
		t.Fatalf("approve status = %d, want 200; body: %s", rrApprove.Code, rrApprove.Body.String())
	}
	env.rec.waitEvent(t, ws.EventPlanApproved)

	// Approve() now always materializes in the same request (DYNWF-5), so
	// handleApprovePlan always broadcasts plan.materialized right after
	// plan.approved.
	materialized := env.rec.waitEvent(t, ws.EventPlanMaterialized)
	if materialized.ProjectID != pid {
		t.Errorf("plan.materialized event.ProjectID = %q, want %q", materialized.ProjectID, pid)
	}
	if got, _ := materialized.Data["instance_id"].(string); got != iid {
		t.Errorf("plan.materialized event.Data[instance_id] = %q, want %q", got, iid)
	}
}

// TestHandleCancelPlan_BroadcastsCancelled verifies cancel emits plan.cancelled.
func TestHandleCancelPlan_BroadcastsCancelled(t *testing.T) {
	env := newPlanWSEnv(t)
	const pid = "proj-plan-cancel-ws"
	const iid = "inst-plan-cancel-ws"
	seedPlanInstance(t, env.s, pid, iid)

	// Need a draft plan to cancel (Cancel just transitions the head, so a
	// prior revise is not strictly required by the repo, but drive through the
	// normal flow for realism).
	body, _ := json.Marshal(types.PlanReviseRequest{
		Revision: 0,
		Manifest: json.RawMessage(validPlanManifestJSON),
	})
	rr := httptest.NewRecorder()
	env.s.handleRevisePlan(rr, planReviseReq(t, iid, body))
	if rr.Code != http.StatusOK {
		t.Fatalf("revise status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	env.rec.waitEvent(t, ws.EventPlanDrafted)

	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-instances/"+iid+"/plan/cancel", nil)
	cancelReq.SetPathValue("iid", iid)
	rrCancel := httptest.NewRecorder()
	env.s.handleCancelPlan(rrCancel, cancelReq)
	if rrCancel.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want 200; body: %s", rrCancel.Code, rrCancel.Body.String())
	}
	ev := env.rec.waitEvent(t, ws.EventPlanCancelled)
	if ev.ProjectID != pid {
		t.Errorf("plan.cancelled event.ProjectID = %q, want %q", ev.ProjectID, pid)
	}
}
