package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/types"
)

// TestPlanApprove_StaleRevisionErrors revises twice (head lands at revision
// 2), then approves the now-stale revision 1, expecting ErrStalePlanRevision.
func TestPlanApprove_StaleRevisionErrors(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	if _, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal 1", "do it"),
	}); err != nil {
		t.Fatalf("revise 1: %v", err)
	}
	if _, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 1, Manifest: validPlanManifestJSON("goal 2", "do it differently"),
	}); err != nil {
		t.Fatalf("revise 2: %v", err)
	}

	if _, err := svc.Approve(instanceID, 1); !errors.Is(err, ErrStalePlanRevision) {
		t.Fatalf("Approve(1) against a head at revision 2: expected ErrStalePlanRevision, got %v", err)
	}
}

// TestPlanApprove_HeadRevisionSucceeds approves the current head revision and
// asserts the returned revision matches, plus the draft head flips to
// approved with the matching ApprovedRevision.
func TestPlanApprove_HeadRevisionSucceeds(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}

	approved, err := svc.Approve(instanceID, rev.Revision)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.Revision != rev.Revision || approved.Manifest != rev.Manifest {
		t.Errorf("Approve returned %+v, want it to match revised %+v", approved, rev)
	}

	draft, err := svc.GetDraft(instanceID)
	if err != nil {
		t.Fatalf("GetDraft: %v", err)
	}
	if draft.Head.Status != model.PlanStatusApproved {
		t.Errorf("Head.Status = %q, want %q", draft.Head.Status, model.PlanStatusApproved)
	}
	if draft.Head.ApprovedRevision != rev.Revision {
		t.Errorf("Head.ApprovedRevision = %d, want %d", draft.Head.ApprovedRevision, rev.Revision)
	}
}

// TestPlanApprove_FailsWhenTemplateModelDisabled asserts Approve re-validates
// the manifest at approval time: a template's model disabled after the draft
// was made must fail approval with a non-sentinel validation error (not
// ErrStalePlanRevision), and the head must remain in draft — approval must
// not partially apply.
func TestPlanApprove_FailsWhenTemplateModelDisabled(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}

	mustExec(t, pool, `UPDATE models SET enabled = 0 WHERE id = 'sonnet-5'`)

	_, err = svc.Approve(instanceID, rev.Revision)
	if err == nil {
		t.Fatal("expected Approve to fail after the template's model was disabled, got nil error")
	}
	if errors.Is(err, ErrStalePlanRevision) {
		t.Fatalf("expected a manifest validation error, not ErrStalePlanRevision: %v", err)
	}
	if !strings.Contains(err.Error(), planTestTemplateID) {
		t.Errorf("expected error to mention template %q, got: %v", planTestTemplateID, err)
	}

	draft, err := svc.GetDraft(instanceID)
	if err != nil {
		t.Fatalf("GetDraft: %v", err)
	}
	if draft.Head.Status != model.PlanStatusDraft {
		t.Errorf("Head.Status = %q after a failed approve, want unchanged %q", draft.Head.Status, model.PlanStatusDraft)
	}
}

// TestPlanApprove_SucceedsWithOpenQuestions asserts a manifest with a
// non-empty questions array still approves cleanly (open questions never
// block approval).
func TestPlanApprove_SucceedsWithOpenQuestions(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestWithQuestions("goal"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}

	if _, err := svc.Approve(instanceID, rev.Revision); err != nil {
		t.Fatalf("Approve with open questions: %v", err)
	}
}

// TestPlanRevise_OnApprovedPlanErrors asserts Revise rejects any further
// revision attempt once a plan head has been approved.
func TestPlanRevise_OnApprovedPlanErrors(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if _, err := svc.Approve(instanceID, rev.Revision); err != nil {
		t.Fatalf("approve: %v", err)
	}

	_, err = svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: rev.Revision,
		Manifest: validPlanManifestJSON("goal 2", "do it again"),
	})
	if !errors.Is(err, ErrPlanNotDraft) {
		t.Fatalf("Revise on an approved plan: expected ErrPlanNotDraft, got %v", err)
	}
}
