package service

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/types"
)

// countInstanceNodes returns the number of materialized workflow_instance_nodes
// rows for an instance.
func countInstanceNodes(t *testing.T, pool *db.Pool, instanceID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM workflow_instance_nodes WHERE instance_id = ?`, instanceID,
	).Scan(&count); err != nil {
		t.Fatalf("count workflow_instance_nodes: %v", err)
	}
	return count
}

// TestPlanMaterialize_NoApprovedPlan_Errors covers both "no plan at all" and
// "draft only, never approved" — Materialize must refuse both with a message
// identifying there's nothing approved to materialize.
func TestPlanMaterialize_NoApprovedPlan_Errors(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	if _, err := svc.Materialize(instanceID); err == nil {
		t.Error("Materialize with no plan at all: expected error, got nil")
	} else if !strings.Contains(err.Error(), "no plan") {
		t.Errorf("expected error identifying no plan for the instance, got: %v", err)
	}

	if _, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	}); err != nil {
		t.Fatalf("revise: %v", err)
	}

	if _, err := svc.Materialize(instanceID); err == nil {
		t.Error("Materialize on a draft (never approved) plan: expected error, got nil")
	} else if !strings.Contains(err.Error(), "approved") {
		t.Errorf("expected error mentioning approved revision, got: %v", err)
	}
}

// TestPlanMaterialize_Idempotent_SecondCallNoNewRows is the acceptance-critical
// "second call takes the RowsAffected==0 path" case: after Approve has already
// materialized once, two further explicit Materialize calls must both return
// the same result and never add rows or move the materialization stamp.
func TestPlanMaterialize_Idempotent_SecondCallNoNewRows(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)
	planRepo := repo.NewPlanRepo(pool, clock.Real())

	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if _, err := svc.Approve(instanceID, rev.Revision); err != nil {
		t.Fatalf("approve (materializes once): %v", err)
	}

	countAfterApprove := countInstanceNodes(t, pool, instanceID)
	if countAfterApprove == 0 {
		t.Fatal("approve did not materialize any nodes")
	}

	result1, err := svc.Materialize(instanceID)
	if err != nil {
		t.Fatalf("first explicit Materialize: %v", err)
	}
	head1, err := planRepo.GetHead(instanceID)
	if err != nil {
		t.Fatalf("GetHead after first explicit Materialize: %v", err)
	}
	countAfterFirst := countInstanceNodes(t, pool, instanceID)

	result2, err := svc.Materialize(instanceID)
	if err != nil {
		t.Fatalf("second explicit Materialize: %v", err)
	}
	head2, err := planRepo.GetHead(instanceID)
	if err != nil {
		t.Fatalf("GetHead after second explicit Materialize: %v", err)
	}
	countAfterSecond := countInstanceNodes(t, pool, instanceID)

	if countAfterFirst != countAfterApprove || countAfterSecond != countAfterApprove {
		t.Errorf("workflow_instance_nodes row count changed across repeated Materialize calls: approve=%d, first=%d, second=%d",
			countAfterApprove, countAfterFirst, countAfterSecond)
	}
	if !reflect.DeepEqual(result1.Nodes, result2.Nodes) {
		t.Errorf("Nodes differ between calls:\ncall1: %+v\ncall2: %+v", result1.Nodes, result2.Nodes)
	}
	if !reflect.DeepEqual(result1.LayerPolicies, result2.LayerPolicies) {
		t.Errorf("LayerPolicies differ between calls:\ncall1: %+v\ncall2: %+v", result1.LayerPolicies, result2.LayerPolicies)
	}
	if head1.MaterializedRevision != head2.MaterializedRevision || head1.MaterializedHash != head2.MaterializedHash {
		t.Errorf("materialized stamp changed across calls: head1=%+v, head2=%+v", head1, head2)
	}
}

// TestPlanMaterialize_ConcurrentCalls_ExactlyOneNodeSet mirrors
// repo/plan_concurrency_test.go's goroutine-race shape for the conditional
// UPDATE hash-stamp guard: many goroutines racing to re-materialize the same
// already-approved revision must never insert more than one set of nodes.
func TestPlanMaterialize_ConcurrentCalls_ExactlyOneNodeSet(t *testing.T) {
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

	const writers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Materialize(instanceID); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent Materialize: %v", err)
	}

	// validPlanManifestJSON has exactly one node ("step1") — the final count
	// must be exactly that, not writers-times that.
	if got := countInstanceNodes(t, pool, instanceID); got != 1 {
		t.Errorf("workflow_instance_nodes count = %d, want 1 (manifest node count, not %d x that)", got, writers)
	}
}
