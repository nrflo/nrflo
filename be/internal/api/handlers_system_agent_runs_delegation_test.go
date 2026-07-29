package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// seedRunsDelegation writes a delegations row (via DelegationRepo, the only
// write path per delegation.go) with one spawned worker slot, returning the
// worker session id it seeded so the caller can insert its agent_sessions
// row with a matching id.
func seedRunsDelegation(t *testing.T, s *Server, id, callerSessionID, tier, workerSessionID string) {
	t.Helper()
	dr := repo.NewDelegationRepo(s.pool, clock.Real())
	del := &model.Delegation{
		ID:                 id,
		CallerSessionID:    callerSessionID,
		WorkflowInstanceID: "wfi-runs",
		ProjectID:          "proj",
		Tier:               tier,
		Brief:              "test brief",
		Fanout:             1,
		Depth:              1,
	}
	if err := dr.Create(del); err != nil {
		t.Fatalf("seed delegation %s: %v", id, err)
	}
	if err := dr.SetWorkerSlot(id, 0, workerSessionID, ""); err != nil {
		t.Fatalf("seed delegation worker slot %s: %v", id, err)
	}
}

// TestHandleListSystemAgentRuns_DelegateRowCarriesDelegationFields verifies
// the 5 delegation JSON keys appear (with correct values) on a delegate
// worker's agent_session row in the merged response.
func TestHandleListSystemAgentRuns_DelegateRowCarriesDelegationFields(t *testing.T) {
	s := newSystemAgentRunsServer(t)
	wfiID := seedRunsProjectAndWFI(t, s)

	seedRunsDelegation(t, s, "deleg-api-1", "sess-caller", "executor", "sess-worker")
	seedRunsAgentSession(t, s, "sess-worker", wfiID, time.Now().UTC().Format(time.RFC3339Nano), 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	items, _ := decodeRunsResponse(t, rr)
	var worker map[string]interface{}
	for _, it := range items {
		if it["session_id"] == "sess-worker" {
			worker = it
		}
	}
	if worker == nil {
		t.Fatalf("sess-worker not found in items: %+v", items)
	}
	if worker["delegation_id"] != "deleg-api-1" {
		t.Errorf("delegation_id = %v, want deleg-api-1", worker["delegation_id"])
	}
	if worker["caller_session_id"] != "sess-caller" {
		t.Errorf("caller_session_id = %v, want sess-caller", worker["caller_session_id"])
	}
	if worker["delegate_tier"] != "executor" {
		t.Errorf("delegate_tier = %v, want executor", worker["delegate_tier"])
	}
	if worker["fanout"] != float64(1) {
		t.Errorf("fanout = %v, want 1", worker["fanout"])
	}
	if worker["delegation_status"] != "running" {
		t.Errorf("delegation_status = %v, want running", worker["delegation_status"])
	}
}

// TestHandleListSystemAgentRuns_NonDelegateRowsOmitDelegationFields verifies
// the omitempty tags drop the 5 delegation keys entirely on non-delegate
// rows: a plain-tier agent_session, a refinery_fold, and a step_rotation.
func TestHandleListSystemAgentRuns_NonDelegateRowsOmitDelegationFields(t *testing.T) {
	s := newSystemAgentRunsServer(t)
	wfiID := seedRunsProjectAndWFI(t, s)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedRunsAgentSession(t, s, "sess-plain", wfiID, base.Format(time.RFC3339Nano), 1)
	seedRefineryRun(t, s, "sess-fold", base.Add(time.Minute).Format(time.RFC3339Nano))
	seedStepRotation(t, s, wfiID, "node-a", "s1", "sess-rot", base.Add(2*time.Minute).Format(time.RFC3339Nano))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	items, _ := decodeRunsResponse(t, rr)
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	delegationKeys := []string{"delegation_id", "caller_session_id", "delegate_tier", "fanout", "delegation_status"}
	for _, it := range items {
		for _, key := range delegationKeys {
			if _, present := it[key]; present {
				t.Errorf("item kind=%v session_id=%v has key %q = %v, want omitted (omitempty)", it["kind"], it["session_id"], key, it[key])
			}
		}
	}
}
