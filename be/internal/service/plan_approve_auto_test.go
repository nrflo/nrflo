package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// TestApproveAuto_UnderCap_DelegatesStraightToApprove asserts that a manifest
// at or under the premium cap approves the SAME revision (no extra revision
// appended, no downgrade finding written) — ApproveAuto must be a pure
// passthrough to Approve in this case.
func TestApproveAuto_UnderCap_DelegatesStraightToApprove(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}

	approved, err := svc.ApproveAuto(instanceID, rev.Revision)
	if err != nil {
		t.Fatalf("ApproveAuto: %v", err)
	}
	if approved.Revision != rev.Revision {
		t.Errorf("approved.Revision = %d, want %d (no new revision should be appended)", approved.Revision, rev.Revision)
	}

	revisions, err := svc.ListRevisions(instanceID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revisions) != 1 {
		t.Errorf("len(revisions) = %d, want 1 (no downgrade revision appended)", len(revisions))
	}

	findings, err := repo.NewFindingRepo(pool, clock.Real()).GetOwn("workflow_instance", instanceID)
	if err != nil {
		t.Fatalf("GetOwn: %v", err)
	}
	if _, ok := findings["_plan_premium_downgrade"]; ok {
		t.Error("did not expect a _plan_premium_downgrade finding for an under-cap manifest")
	}

	draft, err := svc.GetDraft(instanceID)
	if err != nil {
		t.Fatalf("GetDraft: %v", err)
	}
	if draft.Head.Status != model.PlanStatusApproved {
		t.Errorf("Head.Status = %q, want approved", draft.Head.Status)
	}
}

// TestApproveAuto_OverCap_AppendsDowngradeRevisionAndFinding asserts the
// mode=auto path end-to-end: a premium-heavy manifest is downgraded, appended
// as a new caller revision, a _plan_premium_downgrade finding is written, and
// the resulting (compliant) revision is approved + materialized.
//
// The premium-heavy manifest must arrive as if from the planner
// (reviseWithPlanner, author=planner) via fakePlannerRunner: a caller-supplied
// manifest (reviseWithManifest) already rejects a premium-heavy plan at
// revise time (canRevise=true) — ApproveAuto's downgrade path only matters
// for planner output, which is gated solely at the approve boundary.
func TestApproveAuto_OverCap_AppendsDowngradeRevisionAndFinding(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	insertFanoutTemplate(t, pool, planTestProjectID, planTestWorkflowID, "opus-worker", "opus-4-8", "cli_interactive")
	m := premiumHeavyManifest("opus-worker")
	canonical, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	runner := newFakePlannerRunner(pool, clock.Real(), planTestProjectID, "planner-sess-1")
	runner.manifest = canonical
	svc := NewPlanService(pool, clock.Real(), runner)

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Goal: "premium heavy",
	})
	if err != nil {
		t.Fatalf("revise (via planner): %v", err)
	}
	if rev.Author != model.PlanAuthorPlanner {
		t.Fatalf("rev.Author = %q, want %q (setup precondition)", rev.Author, model.PlanAuthorPlanner)
	}

	approved, err := svc.ApproveAuto(instanceID, rev.Revision)
	if err != nil {
		t.Fatalf("ApproveAuto: %v", err)
	}
	if approved.Revision != rev.Revision+1 {
		t.Errorf("approved.Revision = %d, want %d (a new downgrade revision must be appended)", approved.Revision, rev.Revision+1)
	}
	if approved.Author != model.PlanAuthorCaller {
		t.Errorf("approved.Author = %q, want %q (no PlanAuthorSystem const exists)", approved.Author, model.PlanAuthorCaller)
	}

	findings, err := repo.NewFindingRepo(pool, clock.Real()).GetOwn("workflow_instance", instanceID)
	if err != nil {
		t.Fatalf("GetOwn: %v", err)
	}
	raw, ok := findings["_plan_premium_downgrade"]
	if !ok {
		t.Fatal("expected a _plan_premium_downgrade finding on the workflow_instance")
	}
	var val struct {
		Cap        int      `json:"cap"`
		Downgraded []string `json:"downgraded"`
		Message    string   `json:"message"`
	}
	if err := json.Unmarshal(raw, &val); err != nil {
		t.Fatalf("unmarshal _plan_premium_downgrade: %v", err)
	}
	if val.Cap != DefaultDynwfMaxPremiumWorkers {
		t.Errorf("finding cap = %d, want %d", val.Cap, DefaultDynwfMaxPremiumWorkers)
	}
	if len(val.Downgraded) != 8 {
		t.Errorf("finding downgraded = %v (len %d), want 8 rebound node ids", val.Downgraded, len(val.Downgraded))
	}
	if val.Message == "" {
		t.Error("finding message is empty")
	}

	draft, err := svc.GetDraft(instanceID)
	if err != nil {
		t.Fatalf("GetDraft: %v", err)
	}
	if draft.Head.Status != model.PlanStatusApproved {
		t.Errorf("Head.Status = %q, want approved", draft.Head.Status)
	}
	if draft.Head.ApprovedRevision != rev.Revision+1 {
		t.Errorf("Head.ApprovedRevision = %d, want %d", draft.Head.ApprovedRevision, rev.Revision+1)
	}

	materialized, err := svc.Materialize(instanceID)
	if err != nil {
		t.Fatalf("Materialize (idempotent re-check): %v", err)
	}
	var premiumCount int
	for _, n := range materialized.Nodes {
		if n.AgentType == "opus-worker" {
			premiumCount++
		}
	}
	if premiumCount != DefaultDynwfMaxPremiumWorkers {
		t.Errorf("materialized premium node count = %d, want %d", premiumCount, DefaultDynwfMaxPremiumWorkers)
	}
	if len(materialized.Nodes) != 10 {
		t.Errorf("materialized total node count = %d, want 10 (downgrade rebinds templates, never drops nodes)", len(materialized.Nodes))
	}
}

// TestApproveAuto_NoPremiumTemplateAvailable_PropagatesErrorWithoutSideEffects
// asserts that when EnforcePremiumWorkerCap cannot find a downgrade target,
// ApproveAuto surfaces the error and leaves the plan head untouched (still
// draft, no revision appended, no finding written) rather than partially
// applying.
func TestApproveAuto_NoPremiumTemplateAvailable_PropagatesErrorWithoutSideEffects(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	mustExec(t, pool, `UPDATE agent_definitions SET model = 'opus-4-8' WHERE id = ?`, planTestTemplateID)
	m := premiumHeavyManifest(planTestTemplateID)
	canonical, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	runner := newFakePlannerRunner(pool, clock.Real(), planTestProjectID, "planner-sess-2")
	runner.manifest = canonical
	svc := NewPlanService(pool, clock.Real(), runner)

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Goal: "premium heavy, no downgrade target",
	})
	if err != nil {
		t.Fatalf("revise (via planner): %v", err)
	}

	if _, err := svc.ApproveAuto(instanceID, rev.Revision); err == nil {
		t.Fatal("expected an error when no non-premium template is available to downgrade to")
	}

	revisions, lerr := svc.ListRevisions(instanceID)
	if lerr != nil {
		t.Fatalf("ListRevisions: %v", lerr)
	}
	if len(revisions) != 1 {
		t.Errorf("len(revisions) = %d, want 1 (no revision should be appended on error)", len(revisions))
	}
	findings, ferr := repo.NewFindingRepo(pool, clock.Real()).GetOwn("workflow_instance", instanceID)
	if ferr != nil {
		t.Fatalf("GetOwn: %v", ferr)
	}
	if _, ok := findings["_plan_premium_downgrade"]; ok {
		t.Error("did not expect a _plan_premium_downgrade finding when the downgrade itself errored")
	}
	draft, derr := svc.GetDraft(instanceID)
	if derr != nil {
		t.Fatalf("GetDraft: %v", derr)
	}
	if draft.Head.Status != model.PlanStatusDraft {
		t.Errorf("Head.Status = %q after a failed ApproveAuto, want unchanged %q", draft.Head.Status, model.PlanStatusDraft)
	}
}

// TestApproveAuto_StaleRevisionOnUnderCapManifest_Rejected asserts that
// ApproveAuto against a revision the plan head has since moved past is
// rejected by the underlying Approve call (ErrStalePlanRevision) when no
// downgrade is needed — the cap check itself never masks a stale revision.
func TestApproveAuto_StaleRevisionOnUnderCapManifest_Rejected(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	rev1, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal 1", "do it"),
	})
	if err != nil {
		t.Fatalf("revise 1: %v", err)
	}
	if _, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: rev1.Revision, Manifest: validPlanManifestJSON("goal 2", "do it differently"),
	}); err != nil {
		t.Fatalf("revise 2: %v", err)
	}

	if _, err := svc.ApproveAuto(instanceID, rev1.Revision); !errors.Is(err, ErrStalePlanRevision) {
		t.Fatalf("ApproveAuto(%d) against a head that moved to revision 2: expected ErrStalePlanRevision, got %v", rev1.Revision, err)
	}
}
