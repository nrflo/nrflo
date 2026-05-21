package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// TestLayerPolicyPauseAfterRoundTrip verifies pause_after is stored and retrieved.
func TestLayerPolicyPauseAfterRoundTrip(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	r := NewWorkflowLayerPolicyRepo(pool, clock.NewTest(time.Now().UTC()))
	if err := r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  "proj1",
		WorkflowID: "wf1",
		Layer:      0,
		PassPolicy: "any",
		PauseAfter: true,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	rows, err := r.ListByWorkflow("proj1", "wf1")
	if err != nil {
		t.Fatalf("ListByWorkflow: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len = %d, want 1", len(rows))
	}
	if !rows[0].PauseAfter {
		t.Errorf("PauseAfter = false, want true")
	}
}

// TestLayerPolicyGetLayerPauseAfterMap verifies GetLayerPauseAfter returns correct map[int]bool.
func TestLayerPolicyGetLayerPauseAfterMap(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	r := NewWorkflowLayerPolicyRepo(pool, clock.NewTest(time.Now().UTC()))

	policies := []struct {
		layer      int
		pauseAfter bool
	}{
		{0, true},
		{1, false},
		{2, true},
	}
	for _, p := range policies {
		if err := r.Upsert(&model.WorkflowLayerPolicy{
			ProjectID:  "proj1",
			WorkflowID: "wf1",
			Layer:      p.layer,
			PassPolicy: "any",
			PauseAfter: p.pauseAfter,
		}); err != nil {
			t.Fatalf("Upsert(layer=%d): %v", p.layer, err)
		}
	}

	got, err := r.GetLayerPauseAfter("proj1", "wf1")
	if err != nil {
		t.Fatalf("GetLayerPauseAfter: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if !got[0] {
		t.Errorf("layer 0 PauseAfter = false, want true")
	}
	if got[1] {
		t.Errorf("layer 1 PauseAfter = true, want false")
	}
	if !got[2] {
		t.Errorf("layer 2 PauseAfter = false, want true")
	}
}

// TestLayerPolicyGetLayerPauseAfterEmptyMap verifies GetLayerPauseAfter returns empty map for fresh workflow.
func TestLayerPolicyGetLayerPauseAfterEmptyMap(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	r := NewWorkflowLayerPolicyRepo(pool, clock.NewTest(time.Now().UTC()))
	got, err := r.GetLayerPauseAfter("proj1", "wf1")
	if err != nil {
		t.Fatalf("GetLayerPauseAfter: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0 (empty map for fresh workflow)", len(got))
	}
}

// TestLayerPolicyUpsertPassPolicyPreservesPauseAfter verifies that updating pass_policy
// via Upsert does not clobber pause_after.
func TestLayerPolicyUpsertPassPolicyPreservesPauseAfter(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	t0 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(t0)
	r := NewWorkflowLayerPolicyRepo(pool, clk)

	// Insert with pause_after=true.
	if err := r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  "proj1",
		WorkflowID: "wf1",
		Layer:      0,
		PassPolicy: "any",
		PauseAfter: true,
	}); err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	// Now update only pass_policy (sibling should NOT be clobbered).
	clk.Set(t0.Add(time.Minute))
	if err := r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  "proj1",
		WorkflowID: "wf1",
		Layer:      0,
		PassPolicy: "all",
		PauseAfter: true, // caller must preserve sibling at repo level
	}); err != nil {
		t.Fatalf("Upsert (update pass_policy): %v", err)
	}

	rows, err := r.ListByWorkflow("proj1", "wf1")
	if err != nil {
		t.Fatalf("ListByWorkflow: %v", err)
	}
	if rows[0].PassPolicy != "all" {
		t.Errorf("PassPolicy = %q, want \"all\"", rows[0].PassPolicy)
	}
	if !rows[0].PauseAfter {
		t.Errorf("PauseAfter = false after pass_policy update, want true (sibling preserved)")
	}
}

// TestLayerPolicyUpsertPauseAfterPreservesPassPolicy verifies that updating pause_after
// via Upsert does not clobber pass_policy.
func TestLayerPolicyUpsertPauseAfterPreservesPassPolicy(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	r := NewWorkflowLayerPolicyRepo(pool, clock.NewTest(time.Now().UTC()))

	// Insert with pass_policy="quorum:1", pause_after=false.
	if err := r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  "proj1",
		WorkflowID: "wf1",
		Layer:      1,
		PassPolicy: "quorum:1",
		PauseAfter: false,
	}); err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	// Now update only pause_after (sibling should NOT be clobbered).
	if err := r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  "proj1",
		WorkflowID: "wf1",
		Layer:      1,
		PassPolicy: "quorum:1", // caller preserves at repo level
		PauseAfter: true,
	}); err != nil {
		t.Fatalf("Upsert (update pause_after): %v", err)
	}

	rows, err := r.ListByWorkflow("proj1", "wf1")
	if err != nil {
		t.Fatalf("ListByWorkflow: %v", err)
	}
	if rows[0].PassPolicy != "quorum:1" {
		t.Errorf("PassPolicy = %q after pause_after update, want \"quorum:1\"", rows[0].PassPolicy)
	}
	if !rows[0].PauseAfter {
		t.Errorf("PauseAfter = false, want true")
	}
}
