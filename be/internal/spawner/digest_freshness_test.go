package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"

	"github.com/google/uuid"
)

// TestFreshSlotDigest_BoundarySemantics covers the explicit freshness
// boundary: UpdatedAt == sessionStart is fresh, one nanosecond before is
// stale, empty content is stale, and a missing slot is stale.
func TestFreshSlotDigest_BoundarySemantics(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	clk := clock.NewTest(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	nodeID := "test-phase"

	tests := []struct {
		name         string
		seedDigest   bool
		content      string
		updatedAt    time.Time
		sessionStart time.Time
		wantFresh    bool
	}{
		{
			name:         "updated_at equal to session start is fresh",
			seedDigest:   true,
			content:      "DIGEST-TEXT",
			updatedAt:    clk.Now(),
			sessionStart: clk.Now(),
			wantFresh:    true,
		},
		{
			name:         "updated_at one nanosecond before session start is stale",
			seedDigest:   true,
			content:      "DIGEST-TEXT",
			updatedAt:    clk.Now(),
			sessionStart: clk.Now().Add(time.Nanosecond),
			wantFresh:    false,
		},
		{
			name:         "updated_at after session start is fresh",
			seedDigest:   true,
			content:      "DIGEST-TEXT",
			updatedAt:    clk.Now(),
			sessionStart: clk.Now().Add(-time.Minute),
			wantFresh:    true,
		},
		{
			name:         "empty content is stale",
			seedDigest:   true,
			content:      "",
			updatedAt:    clk.Now(),
			sessionStart: clk.Now(),
			wantFresh:    false,
		},
		{
			name:         "missing slot is stale",
			seedDigest:   false,
			sessionStart: clk.Now(),
			wantFresh:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wfiID := uuid.New().String()
			seedClk := clock.NewTest(tc.updatedAt)
			if tc.seedDigest {
				digestRepo := repo.NewRefineryDigestRepo(env.database, seedClk)
				if _, err := digestRepo.UpsertSlot(wfiID, nodeID, env.projectID, tc.content); err != nil {
					t.Fatalf("UpsertSlot: %v", err)
				}
			}

			content, ok := freshSlotDigest(db.WrapAsPool(env.database), clk, wfiID, nodeID, tc.sessionStart)

			if ok != tc.wantFresh {
				t.Errorf("freshSlotDigest() ok = %v, want %v", ok, tc.wantFresh)
			}
			if tc.wantFresh && content != tc.content {
				t.Errorf("freshSlotDigest() content = %q, want %q", content, tc.content)
			}
			if !tc.wantFresh && content != "" {
				t.Errorf("freshSlotDigest() content = %q, want empty on stale/missing", content)
			}
		})
	}
}

// TestFreshSlotDigest_GuardClauses covers the nil-pool and empty-id guards
// that short-circuit before touching the repo.
func TestFreshSlotDigest_GuardClauses(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	clk := clock.Real()
	wfiID := uuid.New().String()
	nodeID := "test-phase"

	digestRepo := repo.NewRefineryDigestRepo(env.database, clk)
	if _, err := digestRepo.UpsertSlot(wfiID, nodeID, env.projectID, "DIGEST-TEXT"); err != nil {
		t.Fatalf("UpsertSlot: %v", err)
	}
	sessionStart := clk.Now().Add(-time.Minute)

	tests := []struct {
		name               string
		pool               *db.Pool
		workflowInstanceID string
		nodeID             string
	}{
		{name: "nil pool", pool: nil, workflowInstanceID: wfiID, nodeID: nodeID},
		{name: "empty workflow instance id", pool: db.WrapAsPool(env.database), workflowInstanceID: "", nodeID: nodeID},
		{name: "empty node id", pool: db.WrapAsPool(env.database), workflowInstanceID: wfiID, nodeID: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content, ok := freshSlotDigest(tc.pool, clk, tc.workflowInstanceID, tc.nodeID, sessionStart)
			if ok {
				t.Errorf("freshSlotDigest() ok = true, want false")
			}
			if content != "" {
				t.Errorf("freshSlotDigest() content = %q, want empty", content)
			}
		})
	}
}

// TestFreshSlotDigest_DifferentSlotIsolated verifies the freshness check is
// scoped to the exact (workflow_instance_id, node_id) slot key.
func TestFreshSlotDigest_DifferentSlotIsolated(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	clk := clock.Real()
	wfiID := uuid.New().String()

	digestRepo := repo.NewRefineryDigestRepo(env.database, clk)
	if _, err := digestRepo.UpsertSlot(wfiID, "phase-a", env.projectID, "DIGEST-TEXT"); err != nil {
		t.Fatalf("UpsertSlot: %v", err)
	}

	sessionStart := clk.Now().Add(-time.Minute)

	content, ok := freshSlotDigest(db.WrapAsPool(env.database), clk, wfiID, "phase-b", sessionStart)
	if ok {
		t.Error("freshSlotDigest() found a digest for a different node_id slot")
	}
	if content != "" {
		t.Errorf("freshSlotDigest() content = %q, want empty for wrong slot", content)
	}

	otherWfiID := uuid.New().String()
	content, ok = freshSlotDigest(db.WrapAsPool(env.database), clk, otherWfiID, "phase-a", sessionStart)
	if ok {
		t.Error("freshSlotDigest() found a digest for a different workflow_instance_id slot")
	}
	if content != "" {
		t.Errorf("freshSlotDigest() content = %q, want empty for wrong slot", content)
	}
}
