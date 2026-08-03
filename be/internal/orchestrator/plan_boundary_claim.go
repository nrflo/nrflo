package orchestrator

// enterPlanBoundary/leavePlanBoundary/ClaimPlanApprovalAtBoundary implement the
// live-boundary handoff: an approval landing while draftPlanAndProceed is
// still blocking inline on the planner must be picked up by that same run
// instead of suspending and waiting for a separate resume. Ordering
// invariant that makes the handoff race-free: the approver commits the
// approve (DB write) first, then claims; the boundary releases its claim
// under the same o.mu right after its blocking planner call returns. So
// either the boundary is still registered when the approver claims (the
// approver's claim wins, the boundary sees it on release and materializes
// inline), or the boundary has already released before the approver claims
// (ClaimPlanApprovalAtBoundary sees no boundary and returns false, so the
// approver falls back to the normal resume path) — never both, never
// neither.

// enterPlanBoundary marks wfiID's run as currently drafting inline at the
// plan boundary. No-op if the run is not registered (should not happen: the
// caller is always inside that run's own runLoop).
func (o *Orchestrator) enterPlanBoundary(wfiID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if rs, ok := o.runs[wfiID]; ok {
		rs.atPlanBoundary = true
	}
}

// leavePlanBoundary clears the boundary flag and returns whether an approver
// claimed the boundary while it was set.
func (o *Orchestrator) leavePlanBoundary(wfiID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	rs, ok := o.runs[wfiID]
	if !ok {
		return false
	}
	claimed := rs.planApprovedAtBoundary
	rs.atPlanBoundary = false
	rs.planApprovedAtBoundary = false
	return claimed
}

// ClaimPlanApprovalAtBoundary claims a live plan boundary for instanceID: it
// returns true (and marks the claim) only when the run is registered AND
// currently at the boundary, so the caller can skip the normal resume path
// and let the live runLoop materialize the approval inline. Idempotent — a
// second claim while the boundary is still live just re-sets the flag.
func (o *Orchestrator) ClaimPlanApprovalAtBoundary(instanceID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	rs, ok := o.runs[instanceID]
	if !ok || !rs.atPlanBoundary {
		return false
	}
	rs.planApprovedAtBoundary = true
	return true
}
