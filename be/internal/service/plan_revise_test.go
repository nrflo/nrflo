package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// TestPlanRevise_AppendOnlyHashStamped revises three times via the
// edited-manifest path with differing goals/instructions each time, and
// asserts: three append-only rows with revision numbers 1,2,3; all hashes
// differ; and revision 1's manifest is byte-identical after later revisions
// have landed.
func TestPlanRevise_AppendOnlyHashStamped(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	var revs []*model.PlanRevision
	for i, goal := range []string{"goal one", "goal two", "goal three"} {
		manifest := validPlanManifestJSON(goal, fmt.Sprintf("instructions %d", i+1))
		rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
			Revision: i,
			Manifest: manifest,
		})
		if err != nil {
			t.Fatalf("Revise #%d: %v", i+1, err)
		}
		revs = append(revs, rev)
	}

	for i, rev := range revs {
		if rev.Revision != i+1 {
			t.Errorf("revs[%d].Revision = %d, want %d", i, rev.Revision, i+1)
		}
	}
	if revs[0].Hash == revs[1].Hash || revs[1].Hash == revs[2].Hash || revs[0].Hash == revs[2].Hash {
		t.Errorf("expected distinct hashes across revisions, got %s, %s, %s", revs[0].Hash, revs[1].Hash, revs[2].Hash)
	}

	all, err := svc.ListRevisions(instanceID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListRevisions returned %d rows, want 3", len(all))
	}

	planRepo := repo.NewPlanRepo(pool, clock.Real())
	rev1Again, err := planRepo.GetRevision(instanceID, 1)
	if err != nil {
		t.Fatalf("GetRevision(1) after later revisions: %v", err)
	}
	if rev1Again.Manifest != revs[0].Manifest {
		t.Errorf("revision 1 manifest changed after later revisions landed:\ngot:  %s\nwant: %s", rev1Again.Manifest, revs[0].Manifest)
	}
}

// TestPlanRevise_StaleRevisionBothPaths exercises both the manifest-edit and
// planner-replan sub-paths with a wrong req.Revision, expecting the stale
// check in Revise itself (before dispatch) to reject both, and the fake
// runner to never be invoked.
func TestPlanRevise_StaleRevisionBothPaths(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	runner := newFakePlannerRunner(pool, clock.Real(), planTestProjectID, "sess-stale")
	svc := NewPlanService(pool, clock.Real(), runner)

	if _, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0,
		Manifest: validPlanManifestJSON("goal", "do it"),
	}); err != nil {
		t.Fatalf("initial revise: %v", err)
	}

	t.Run("manifest_edit_path", func(t *testing.T) {
		_, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
			Revision: 99,
			Manifest: validPlanManifestJSON("goal 2", "do it more"),
		})
		if !errors.Is(err, ErrStalePlanRevision) {
			t.Fatalf("expected ErrStalePlanRevision, got %v", err)
		}
	})

	t.Run("planner_replan_path", func(t *testing.T) {
		_, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
			Revision: 0,
			Goal:     "replanned goal",
			Feedback: "please redo",
		})
		if !errors.Is(err, ErrStalePlanRevision) {
			t.Fatalf("expected ErrStalePlanRevision, got %v", err)
		}
	})

	if runner.calls != 0 {
		t.Errorf("fake runner should not have been invoked on a stale-revision rejection, calls=%d", runner.calls)
	}
}

// TestPlanRevise_FirstReviseRequiresRevisionZero asserts a brand-new instance
// (no plan head yet) rejects any non-zero req.Revision and accepts zero.
func TestPlanRevise_FirstReviseRequiresRevisionZero(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	if _, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 1,
		Manifest: validPlanManifestJSON("goal", "do it"),
	}); !errors.Is(err, ErrStalePlanRevision) {
		t.Fatalf("Revision:1 on brand-new instance: expected ErrStalePlanRevision, got %v", err)
	}

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0,
		Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("Revision:0 on brand-new instance: %v", err)
	}
	if rev.Revision != 1 {
		t.Errorf("Revision = %d, want 1", rev.Revision)
	}
}

// TestPlanRevise_PlannerReplanPath asserts the planner-replan sub-path calls
// the injected PlannerRunner once with the caller-supplied Goal/Feedback/
// Answers, and that the resulting revision is stamped author=planner with the
// runner's session id.
func TestPlanRevise_PlannerReplanPath(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	runner := newFakePlannerRunner(pool, clock.Real(), planTestProjectID, "sess-replan")
	svc := NewPlanService(pool, clock.Real(), runner)

	answers := []types.PlanAnswer{{QuestionID: "q1", Answer: "yes"}}
	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0,
		Goal:     "planner goal",
		Feedback: "make it better",
		Answers:  answers,
	})
	if err != nil {
		t.Fatalf("Revise (planner path): %v", err)
	}

	if runner.calls != 1 {
		t.Fatalf("expected fake runner to be called once, got %d", runner.calls)
	}
	if runner.lastInput.Goal != "planner goal" {
		t.Errorf("PlannerInput.Goal = %q, want %q", runner.lastInput.Goal, "planner goal")
	}
	if runner.lastInput.Feedback != "make it better" {
		t.Errorf("PlannerInput.Feedback = %q, want %q", runner.lastInput.Feedback, "make it better")
	}
	if len(runner.lastInput.Answers) != 1 || runner.lastInput.Answers[0] != answers[0] {
		t.Errorf("PlannerInput.Answers = %+v, want %+v", runner.lastInput.Answers, answers)
	}
	if rev.Author != model.PlanAuthorPlanner {
		t.Errorf("rev.Author = %q, want %q", rev.Author, model.PlanAuthorPlanner)
	}
	if rev.PlannerSessionID != "sess-replan" {
		t.Errorf("rev.PlannerSessionID = %q, want %q", rev.PlannerSessionID, "sess-replan")
	}
}
