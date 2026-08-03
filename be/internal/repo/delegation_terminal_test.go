package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

func TestDelegation_MarkFanoutDone(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	d := &model.Delegation{ID: "wfi-4.fanout01", CallerSessionID: "c", WorkflowInstanceID: "wfi-4", ProjectID: "proj-1", Tier: "executor", Fanout: 1, Depth: 1}
	if err := r.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.MarkFanoutDone(d.ID); err != nil {
		t.Fatalf("MarkFanoutDone: %v", err)
	}
	got, err := r.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.FanoutDone {
		t.Error("FanoutDone = false, want true after MarkFanoutDone")
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want running (MarkFanoutDone does not change status)", got.Status)
	}
}

func TestDelegation_MarkCompleted_SetsStatusWithoutConsuming(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	d := &model.Delegation{ID: "wfi-5.term01", CallerSessionID: "c", WorkflowInstanceID: "wfi-5", ProjectID: "proj-1", Tier: "executor", Fanout: 1, Depth: 1}
	if err := r.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}

	won, err := r.MarkCompleted(d.ID, "completed")
	if err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	if !won {
		t.Fatal("MarkCompleted first call: won = false, want true")
	}

	got, err := r.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt = nil, want set")
	}
	if got.ConsumedAt != nil {
		t.Error("ConsumedAt set by MarkCompleted, want nil (completion is independent of consumption)")
	}

	// Second call loses the completed_at IS NULL guard: it must report it did
	// not win, and must not silently rewrite the row's status.
	won2, err := r.MarkCompleted(d.ID, "failed")
	if err != nil {
		t.Fatalf("MarkCompleted (second call): %v", err)
	}
	if won2 {
		t.Error("MarkCompleted second call: won = true, want false (already completed)")
	}
	got2, err := r.Get(d.ID)
	if err != nil {
		t.Fatalf("Get after second MarkCompleted: %v", err)
	}
	if got2.Status != "completed" {
		t.Errorf("Status after guarded second call = %q, want unchanged completed", got2.Status)
	}
}

func TestDelegation_MarkConsumed_ExactlyOnce(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	d := &model.Delegation{ID: "wfi-5.term02", CallerSessionID: "c", WorkflowInstanceID: "wfi-5", ProjectID: "proj-1", Tier: "executor", Fanout: 1, Depth: 1}
	if err := r.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := r.MarkCompleted(d.ID, "completed"); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}

	won, err := r.MarkConsumed(d.ID)
	if err != nil {
		t.Fatalf("MarkConsumed: %v", err)
	}
	if !won {
		t.Fatal("MarkConsumed first call: won = false, want true")
	}
	got, err := r.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ConsumedAt == nil {
		t.Error("ConsumedAt = nil, want set")
	}

	won2, err := r.MarkConsumed(d.ID)
	if err != nil {
		t.Fatalf("MarkConsumed (second call): %v", err)
	}
	if won2 {
		t.Error("MarkConsumed second call: won = true, want false (already consumed)")
	}
}

func TestDelegation_DepthForSession(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	d := &model.Delegation{ID: "wfi-6.depth01", CallerSessionID: "caller", WorkflowInstanceID: "wfi-6", ProjectID: "proj-1", Tier: "executor", Fanout: 2, Depth: 1}
	if err := r.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.SetWorkerSlot(d.ID, 0, "worker-a", ""); err != nil {
		t.Fatalf("SetWorkerSlot: %v", err)
	}
	if err := r.SetWorkerSlot(d.ID, 1, "worker-b", ""); err != nil {
		t.Fatalf("SetWorkerSlot: %v", err)
	}

	depth, err := r.DepthForSession("worker-a")
	if err != nil {
		t.Fatalf("DepthForSession(worker-a): %v", err)
	}
	if depth != 1 {
		t.Errorf("DepthForSession(worker-a) = %d, want 1 (the row's depth)", depth)
	}

	depth, err = r.DepthForSession("worker-b")
	if err != nil {
		t.Fatalf("DepthForSession(worker-b): %v", err)
	}
	if depth != 1 {
		t.Errorf("DepthForSession(worker-b) = %d, want 1", depth)
	}

	depth, err = r.DepthForSession("someone-not-tracked")
	if err != nil {
		t.Fatalf("DepthForSession(untracked): %v", err)
	}
	if depth != 0 {
		t.Errorf("DepthForSession(untracked) = %d, want 0", depth)
	}
}

// TestDelegation_DepthForSession_EmptyStringMatchesUnfilledSlots documents a
// known edge case rather than asserting a fix: a fresh Create seeds every
// worker_session_ids slot as "" (unfilled), and DepthForSession's json_each
// membership check cannot distinguish "unfilled slot" from "a real session
// id of empty string" — so DepthForSession("") resolves to the row's depth
// instead of 0. Not reachable in production (agent session ids are never
// empty), but recorded here so a future caller of DepthForSession("") does
// not get silently the wrong depth. See be_production_bugs finding.
func TestDelegation_DepthForSession_EmptyStringMatchesUnfilledSlots(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	d := &model.Delegation{ID: "wfi-7.depth02", CallerSessionID: "caller", WorkflowInstanceID: "wfi-7", ProjectID: "proj-1", Tier: "executor", Fanout: 3, Depth: 2}
	if err := r.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}

	depth, err := r.DepthForSession("")
	if err != nil {
		t.Fatalf("DepthForSession(\"\"): %v", err)
	}
	if depth != 2 {
		t.Errorf("DepthForSession(\"\") = %d, want 2 (current behavior: matches unfilled blank slots — see comment)", depth)
	}
}
