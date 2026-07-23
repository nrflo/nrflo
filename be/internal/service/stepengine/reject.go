package stepengine

// CountsTowardEvidenceCap reports whether this rejection's reason should
// increment the durable per-step rejection counter: an evidence/check
// failure does (the agent had a real shot at the step and missed), a guard
// miss (stale_revision/step_mismatch — the agent's call landed against a
// cursor that already moved) does not. Rule 6: the classification lives on
// the type, not a reason-string switch at the call site.
func (r *Rejection) CountsTowardEvidenceCap() bool {
	if r == nil {
		return false
	}
	switch r.Reason {
	case "missing_evidence", "invalid_evidence", "check_failed", "path_overlap":
		return true
	default:
		return false
	}
}

// RecordRejection increments the durable per-step rejection counter for
// (instanceID, nodeID, stepID) and returns the new count.
func (e *Engine) RecordRejection(instanceID, nodeID, stepID string) (int, error) {
	return e.cursorRepo.RecordRejection(instanceID, nodeID, stepID)
}

// RejectionCount returns the current rejection count for (instanceID,
// nodeID, stepID) without incrementing it.
func (e *Engine) RejectionCount(instanceID, nodeID, stepID string) (int, error) {
	counts, err := e.cursorRepo.Rejections(instanceID, nodeID)
	if err != nil {
		return 0, err
	}
	return counts[stepID], nil
}
