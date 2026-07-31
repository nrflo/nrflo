package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestDelegation_SetWorktree_RoundTrip verifies prepareAndPersistDelegateWorktree's
// SetWorktree call round-trips through Get, and that a fresh row (before any
// SetWorktree call) reports all four worktree columns empty.
func TestDelegation_SetWorktree_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	d := &model.Delegation{
		ID:                 "wfi-wt.abcd1234",
		CallerSessionID:    "caller-sess",
		WorkflowInstanceID: "wfi-wt",
		ProjectID:          "proj-1",
		Tier:               "executor",
		Brief:              "implement the thing",
		Fanout:             1,
		Depth:              1,
	}
	if err := r.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}

	fresh, err := r.Get(d.ID)
	if err != nil {
		t.Fatalf("Get before SetWorktree: %v", err)
	}
	if fresh.WorktreePath != "" || fresh.BranchName != "" || fresh.BaseCommit != "" || fresh.Summary != "" {
		t.Errorf("fresh row worktree fields = %+v, want all empty", fresh)
	}

	if err := r.SetWorktree(d.ID, "/tmp/nrflo/worktrees/nrdelegate-wfi-wt-abcd1234", "nrdelegate/wfi-wt-abcd1234", "deadbeef"); err != nil {
		t.Fatalf("SetWorktree: %v", err)
	}

	got, err := r.Get(d.ID)
	if err != nil {
		t.Fatalf("Get after SetWorktree: %v", err)
	}
	if got.WorktreePath != "/tmp/nrflo/worktrees/nrdelegate-wfi-wt-abcd1234" {
		t.Errorf("WorktreePath = %q, want the seeded path", got.WorktreePath)
	}
	if got.BranchName != "nrdelegate/wfi-wt-abcd1234" {
		t.Errorf("BranchName = %q, want the seeded branch", got.BranchName)
	}
	if got.BaseCommit != "deadbeef" {
		t.Errorf("BaseCommit = %q, want deadbeef", got.BaseCommit)
	}
	if got.Summary != "" {
		t.Errorf("Summary = %q, want still empty (SetWorktree doesn't touch it)", got.Summary)
	}
}

// TestDelegation_SetWorktreeSummary_RoundTrip verifies finalizeDelegateWorktree's
// SetWorktreeSummary call persists independently of SetWorktree, and that
// clearing branch_name to "" (the empty-commit path) round-trips too.
func TestDelegation_SetWorktreeSummary_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	d := &model.Delegation{
		ID:                 "wfi-wt2.efgh5678",
		CallerSessionID:    "caller-sess",
		WorkflowInstanceID: "wfi-wt2",
		ProjectID:          "proj-1",
		Tier:               "executor",
		Fanout:             1,
		Depth:              1,
	}
	if err := r.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.SetWorktree(d.ID, "/tmp/wt", "nrdelegate/wfi-wt2-efgh5678", "cafefeed"); err != nil {
		t.Fatalf("SetWorktree: %v", err)
	}

	summaryJSON := `{"committed":true,"changed_files":["a.go","b.go"],"diffstat":" 2 files changed"}`
	if err := r.SetWorktreeSummary(d.ID, summaryJSON); err != nil {
		t.Fatalf("SetWorktreeSummary: %v", err)
	}

	got, err := r.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Summary != summaryJSON {
		t.Errorf("Summary = %q, want %q", got.Summary, summaryJSON)
	}
	// Unrelated fields untouched.
	if got.BranchName != "nrdelegate/wfi-wt2-efgh5678" || got.BaseCommit != "cafefeed" {
		t.Errorf("SetWorktreeSummary must not touch branch/base_commit, got branch=%q base=%q", got.BranchName, got.BaseCommit)
	}

	// finalizeDelegateWorktree clears branch_name (keeping worktree_path and
	// base_commit) once CommitAndCollect reports nothing was committed, so a
	// stale branch ref never surfaces a dangling merge hint.
	if err := r.SetWorktree(d.ID, "/tmp/wt", "", "cafefeed"); err != nil {
		t.Fatalf("SetWorktree (clear branch): %v", err)
	}
	cleared, err := r.Get(d.ID)
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if cleared.BranchName != "" {
		t.Errorf("BranchName = %q after clear, want empty", cleared.BranchName)
	}
	if cleared.WorktreePath != "/tmp/wt" {
		t.Errorf("WorktreePath = %q after clearing branch, want preserved", cleared.WorktreePath)
	}
}
