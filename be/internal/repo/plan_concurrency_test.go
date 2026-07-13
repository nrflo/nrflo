package repo

import (
	"fmt"
	"sync"
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestPlanRepo_Append_ConcurrentNoDuplicateRevisions is the acceptance-critical
// case for Append's tx + MAX(revision)+1 scheme: N goroutines racing to
// append against the same instance must land exactly N rows numbered
// 1..N with no duplicates and no gaps.
func TestPlanRepo_Append_ConcurrentNoDuplicateRevisions(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	instanceID := "inst-concurrent-append"
	seedPlanInstance(t, pool, instanceID)
	repo := NewPlanRepo(pool, clock.Real())

	const writers = 16
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := repo.Append(
				instanceID,
				fmt.Sprintf("manifest-%d", i),
				fmt.Sprintf("hash-%d", i),
				model.PlanAuthorPlanner,
				"",
				"goal",
			)
			if err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("Append: %v", err)
	}

	revisions, err := repo.ListRevisions(instanceID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revisions) != writers {
		t.Fatalf("len(revisions) = %d, want %d", len(revisions), writers)
	}

	seen := make(map[int]bool)
	for _, rev := range revisions {
		if seen[rev.Revision] {
			t.Errorf("duplicate revision number: %d", rev.Revision)
		}
		seen[rev.Revision] = true
	}
	for i := 1; i <= writers; i++ {
		if !seen[i] {
			t.Errorf("missing revision %d (gap in sequence)", i)
		}
	}
}
