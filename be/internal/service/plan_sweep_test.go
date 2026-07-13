package service

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// TestPlanSweepExpiredDrafts sets up three plans under a clock.NewTest clock
// (no real sleeping): one draft backdated past the plan_draft_ttl_min window,
// one fresh draft, and one already approved. SweepExpiredDrafts must cancel
// only the first.
func TestPlanSweepExpiredDrafts(t *testing.T) {
	t.Parallel()
	pool, expiredID := setupPlanTestEnv(t)
	freshID := "plan-wfi-fresh"
	approvedID := "plan-wfi-approved"
	insertPlanTestInstance(t, pool, freshID)
	insertPlanTestInstance(t, pool, approvedID)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(start)
	svc := NewPlanService(pool, clk, nil)

	// Expired draft: revise now, then backdate workflow_plans.updated_at to
	// older than the default 1440-minute TTL window.
	if _, err := svc.Revise(context.Background(), expiredID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("expired goal", "do it"),
	}); err != nil {
		t.Fatalf("revise expired: %v", err)
	}
	oldUpdatedAt := start.Add(-1500 * time.Minute).UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `UPDATE workflow_plans SET updated_at = ? WHERE instance_id = ?`, oldUpdatedAt, expiredID)

	// Fresh draft: revise now, updated_at stays at 'start' (well inside TTL).
	if _, err := svc.Revise(context.Background(), freshID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("fresh goal", "do it"),
	}); err != nil {
		t.Fatalf("revise fresh: %v", err)
	}

	// Approved plan: revise + approve; must be left alone by the sweep
	// regardless of age.
	approvedRev, err := svc.Revise(context.Background(), approvedID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("approved goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise approved: %v", err)
	}
	if _, err := svc.Approve(approvedID, approvedRev.Revision); err != nil {
		t.Fatalf("approve: %v", err)
	}

	count, err := svc.SweepExpiredDrafts(start)
	if err != nil {
		t.Fatalf("SweepExpiredDrafts: %v", err)
	}
	if count != 1 {
		t.Fatalf("SweepExpiredDrafts count = %d, want 1", count)
	}

	planRepo := repo.NewPlanRepo(pool, clk)

	expiredHead, err := planRepo.GetHead(expiredID)
	if err != nil {
		t.Fatalf("GetHead(expired): %v", err)
	}
	if expiredHead.Status != model.PlanStatusCancelled {
		t.Errorf("expired draft status = %q, want %q", expiredHead.Status, model.PlanStatusCancelled)
	}

	freshHead, err := planRepo.GetHead(freshID)
	if err != nil {
		t.Fatalf("GetHead(fresh): %v", err)
	}
	if freshHead.Status != model.PlanStatusDraft {
		t.Errorf("fresh draft status = %q, want %q", freshHead.Status, model.PlanStatusDraft)
	}

	approvedHead, err := planRepo.GetHead(approvedID)
	if err != nil {
		t.Fatalf("GetHead(approved): %v", err)
	}
	if approvedHead.Status != model.PlanStatusApproved {
		t.Errorf("approved plan status = %q, want %q", approvedHead.Status, model.PlanStatusApproved)
	}
}
