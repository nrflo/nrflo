package service

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// midDraftSeedingRunner simulates the nrworkflow-4d0243 race: a caller seeds
// its own revision via revise_plan while the (minutes-long) planner session
// is still drafting. It lands a caller-authored revision 1 before delegating
// to the normal fake runner.
type midDraftSeedingRunner struct {
	inner *fakePlannerRunner
	pool  *db.Pool
	clk   clock.Clock
}

func (r midDraftSeedingRunner) RunPlanner(ctx context.Context, instanceID string, in PlannerInput) (string, error) {
	raw := validPlanManifestJSON("caller goal", "caller instructions")
	m, err := ParsePlanManifest(raw)
	if err != nil {
		return "", err
	}
	if _, err := repo.NewPlanRepo(r.pool, r.clk).Append(instanceID, string(raw), HashManifest(m), model.PlanAuthorCaller, "", "caller goal"); err != nil {
		return "", err
	}
	return r.inner.RunPlanner(ctx, instanceID, in)
}

// A caller revision landing mid-draft wins: the planner's output is dropped
// instead of appended on top of it, and Revise returns the caller's head.
func TestPlanRevise_CallerRevisionMidDraft_DropsPlannerDraft(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	runner := midDraftSeedingRunner{
		inner: newFakePlannerRunner(pool, clock.Real(), planTestProjectID, "sess-race"),
		pool:  pool,
		clk:   clock.Real(),
	}
	svc := NewPlanService(pool, clock.Real(), runner)

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{Revision: 0, Goal: "planner goal"})
	if err != nil {
		t.Fatalf("Revise: %v", err)
	}
	if rev.Author != model.PlanAuthorCaller {
		t.Errorf("rev.Author = %q, want %q (the caller's mid-draft revision wins)", rev.Author, model.PlanAuthorCaller)
	}
	if rev.Revision != 1 {
		t.Errorf("rev.Revision = %d, want 1", rev.Revision)
	}
	revs, err := svc.ListRevisions(instanceID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 1 {
		t.Fatalf("len(revisions) = %d, want 1 (planner draft dropped, not appended as revision 2)", len(revs))
	}
}
