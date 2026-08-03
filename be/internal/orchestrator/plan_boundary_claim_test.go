package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// registerBoundaryRun registers a bare runState for wfiID directly in
// o.runs (mirroring orchestrator_settle_test.go's pattern), purged via
// t.Cleanup so newTestEnv's own cleanup (which waits on every run's done
// channel) cannot block on a never-closing done below.
func registerBoundaryRun(t *testing.T, env *testEnv, wfiID string, atBoundary bool) *runState {
	t.Helper()
	rs := &runState{cancel: func() {}, done: make(chan struct{}), atPlanBoundary: atBoundary}
	env.orch.mu.Lock()
	env.orch.runs[wfiID] = rs
	env.orch.mu.Unlock()
	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})
	return rs
}

// TestClaimPlanApprovalAtBoundary_Handshake covers the claim/release
// helpers directly: an unregistered instance and a registered-but-not-at-
// boundary run both fail to claim; claiming succeeds once enterPlanBoundary
// ran; leavePlanBoundary returns the claim exactly once and clears both flags.
func TestClaimPlanApprovalAtBoundary_Handshake(t *testing.T) {
	env := newTestEnv(t)

	if got := env.orch.ClaimPlanApprovalAtBoundary("wfi-unregistered"); got {
		t.Errorf("ClaimPlanApprovalAtBoundary(unregistered) = true, want false")
	}

	registerBoundaryRun(t, env, "wfi-not-boundary", false)
	if got := env.orch.ClaimPlanApprovalAtBoundary("wfi-not-boundary"); got {
		t.Errorf("ClaimPlanApprovalAtBoundary(registered, not at boundary) = true, want false")
	}

	rs := registerBoundaryRun(t, env, "wfi-at-boundary", true)
	if got := env.orch.ClaimPlanApprovalAtBoundary("wfi-at-boundary"); !got {
		t.Fatalf("ClaimPlanApprovalAtBoundary(at boundary) = false, want true")
	}
	if !rs.planApprovedAtBoundary {
		t.Errorf("planApprovedAtBoundary = false after claim, want true")
	}

	// Idempotent: a second claim while still at the boundary re-sets the flag.
	if got := env.orch.ClaimPlanApprovalAtBoundary("wfi-at-boundary"); !got {
		t.Errorf("second claim = false, want true (idempotent)")
	}

	if claimed := env.orch.leavePlanBoundary("wfi-at-boundary"); !claimed {
		t.Errorf("leavePlanBoundary returned false, want true (claim was set)")
	}
	if rs.atPlanBoundary || rs.planApprovedAtBoundary {
		t.Errorf("flags after leavePlanBoundary: atPlanBoundary=%v planApprovedAtBoundary=%v, want both false", rs.atPlanBoundary, rs.planApprovedAtBoundary)
	}
	// leavePlanBoundary is not idempotent on the claim result: it returns the
	// flag once and clears it, so a repeat call reports no claim.
	if claimed := env.orch.leavePlanBoundary("wfi-at-boundary"); claimed {
		t.Errorf("second leavePlanBoundary returned true, want false (already cleared)")
	}
}

// TestResumeAfterPlanApproval_LiveBoundary_ClaimsAndReturnsNil: a
// plan-suspended instance whose registered runState is at the boundary with
// a never-closing done channel must be claimed and handed off instead of
// blocking on waitForRunToSettle (a 35s runSettleTimeout block would blow
// Rule 4's 60s suite cap on its own). Status stays 'planning' and no
// EventWorkflowResumed is broadcast — the live runLoop owns materializing
// the approval, not this call.
func TestResumeAfterPlanApproval_LiveBoundary_ClaimsAndReturnsNil(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl")
	env.createTicket(t, "PBC-1", "live boundary claim")
	wfiID := env.initWorkflow(t, "PBC-1")
	ch := env.subscribeWSClient(t, "ws-pbc-1", "PBC-1")

	if err := repo.NewWorkflowInstanceRepo(env.pool, clock.Real()).UpdateStatus(wfiID, model.WorkflowInstancePlanning); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	registerBoundaryRun(t, env, wfiID, true)

	if err := env.orch.ResumeAfterPlanApproval(context.Background(), wfiID); err != nil {
		t.Fatalf("ResumeAfterPlanApproval: %v", err)
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstancePlanning {
		t.Errorf("status = %v, want unchanged planning (the live boundary, not this call, flips it)", wi.Status)
	}

	select {
	case raw := <-ch:
		var evt ws.Event
		if err := json.Unmarshal(raw, &evt); err == nil && evt.Type == ws.EventWorkflowResumed {
			t.Errorf("got EventWorkflowResumed, want no resume event for a claimed boundary")
		}
	case <-time.After(100 * time.Millisecond):
		// No event within a short window is the expected outcome.
	}
}
