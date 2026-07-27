package service

import (
	"context"
	"errors"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/types"
)

// TestPlanApprove_ReApprovingApprovedRevisionIsIdempotent covers the recovery
// path for a resume that failed after a successful approve: every approve call
// site resumes the run in its own tail, so re-approving the same revision must
// return the revision (letting that tail run again) instead of ErrPlanNotDraft,
// which would strand the instance plan-suspended over an approved plan —
// ContinueWorkflow only accepts `waiting`, so nothing else could resume it.
func TestPlanApprove_ReApprovingApprovedRevisionIsIdempotent(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	first, err := svc.Approve(instanceID, rev.Revision)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}

	again, err := svc.Approve(instanceID, first.Revision)
	if err != nil {
		t.Fatalf("re-approve at the approved revision: %v (want idempotent success)", err)
	}
	if again.Revision != first.Revision {
		t.Errorf("re-approve revision = %d, want %d", again.Revision, first.Revision)
	}

	head, err := svc.planRepo.GetHead(instanceID)
	if err != nil {
		t.Fatalf("GetHead after re-approve: %v", err)
	}
	if head.Status != model.PlanStatusApproved {
		t.Errorf("head.Status = %q, want approved", head.Status)
	}

	if _, err := svc.Approve(instanceID, first.Revision+1); !errors.Is(err, ErrStalePlanRevision) {
		t.Errorf("re-approve at a different revision = %v, want ErrStalePlanRevision", err)
	}
}
