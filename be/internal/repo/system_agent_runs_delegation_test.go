package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// TestListSystemAgentRuns_DelegateWorkerPopulatesDelegationFields verifies a
// session named in a delegation's worker_session_ids gets the 5 delegation
// fields populated from the LEFT JOIN.
func TestListSystemAgentRuns_DelegateWorkerPopulatesDelegationFields(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	dr := NewDelegationRepo(database, clock.Real())
	del := &model.Delegation{
		ID:                 "deleg-1",
		CallerSessionID:    "sess-caller",
		WorkflowInstanceID: wfiID,
		ProjectID:          "proj",
		Tier:               "executor",
		Brief:              "do the thing",
		Fanout:             1,
		Depth:              1,
	}
	if err := dr.Create(del); err != nil {
		t.Fatalf("Create delegation: %v", err)
	}
	if err := dr.SetWorkerSlot("deleg-1", 0, "sess-worker-1", ""); err != nil {
		t.Fatalf("SetWorkerSlot: %v", err)
	}
	if err := dr.SetWorktree("deleg-1", "/tmp/nrflo/worktrees/nrdelegate-deleg-1", "nrdelegate/deleg-1", "abc123"); err != nil {
		t.Fatalf("SetWorktree: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	insertAgentSessionForRuns(t, database, wfiID, "sess-worker-1", "_t1_executor", nil, now)

	got, err := r.ListSystemAgentRuns(50, time.Time{})
	if err != nil {
		t.Fatalf("ListSystemAgentRuns: %v", err)
	}

	var worker *model.SystemAgentRun
	for _, run := range got {
		if run.SessionID == "sess-worker-1" {
			worker = run
		}
	}
	if worker == nil {
		t.Fatalf("sess-worker-1 not found in %+v", got)
	}
	if worker.DelegationID != "deleg-1" {
		t.Errorf("DelegationID = %q, want deleg-1", worker.DelegationID)
	}
	if worker.CallerSessionID != "sess-caller" {
		t.Errorf("CallerSessionID = %q, want sess-caller", worker.CallerSessionID)
	}
	if worker.DelegateTier != "executor" {
		t.Errorf("DelegateTier = %q, want executor", worker.DelegateTier)
	}
	if worker.Fanout != 1 {
		t.Errorf("Fanout = %d, want 1", worker.Fanout)
	}
	if worker.DelegationStatus != "running" {
		t.Errorf("DelegationStatus = %q, want running", worker.DelegationStatus)
	}
	if worker.DelegationBranch != "nrdelegate/deleg-1" {
		t.Errorf("DelegationBranch = %q, want nrdelegate/deleg-1 (joined from delegations.branch_name)", worker.DelegationBranch)
	}
}

// TestListSystemAgentRuns_NonIsolatedDelegationHasEmptyBranch verifies a
// delegation that never ran under worktree isolation (branch_name == ”)
// leaves SystemAgentRun.DelegationBranch empty rather than surfacing an
// empty-string branch as if it were set.
func TestListSystemAgentRuns_NonIsolatedDelegationHasEmptyBranch(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	dr := NewDelegationRepo(database, clock.Real())
	del := &model.Delegation{
		ID:                 "deleg-noiso",
		CallerSessionID:    "sess-caller-noiso",
		WorkflowInstanceID: wfiID,
		ProjectID:          "proj",
		Tier:               "extractor",
		Fanout:             1,
		Depth:              1,
	}
	if err := dr.Create(del); err != nil {
		t.Fatalf("Create delegation: %v", err)
	}
	if err := dr.SetWorkerSlot("deleg-noiso", 0, "sess-worker-noiso", ""); err != nil {
		t.Fatalf("SetWorkerSlot: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	insertAgentSessionForRuns(t, database, wfiID, "sess-worker-noiso", "_t2_extractor", nil, now)

	got, err := r.ListSystemAgentRuns(50, time.Time{})
	if err != nil {
		t.Fatalf("ListSystemAgentRuns: %v", err)
	}
	var worker *model.SystemAgentRun
	for _, run := range got {
		if run.SessionID == "sess-worker-noiso" {
			worker = run
		}
	}
	if worker == nil {
		t.Fatalf("sess-worker-noiso not found in %+v", got)
	}
	if worker.DelegationBranch != "" {
		t.Errorf("DelegationBranch = %q, want empty for a non-isolated delegation", worker.DelegationBranch)
	}
}

// TestListSystemAgentRuns_TwoWorkerDelegationNoRowMultiplication is a
// regression against the LEFT JOIN duplicating or dropping listing rows: a
// 2-worker delegation must yield exactly 2 rows, one per worker session.
func TestListSystemAgentRuns_TwoWorkerDelegationNoRowMultiplication(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	dr := NewDelegationRepo(database, clock.Real())
	del := &model.Delegation{
		ID:                 "deleg-2",
		CallerSessionID:    "sess-caller-2",
		WorkflowInstanceID: wfiID,
		ProjectID:          "proj",
		Tier:               "extractor",
		Brief:              "fan out",
		Fanout:             2,
		Depth:              1,
	}
	if err := dr.Create(del); err != nil {
		t.Fatalf("Create delegation: %v", err)
	}
	if err := dr.SetWorkerSlot("deleg-2", 0, "sess-worker-a", ""); err != nil {
		t.Fatalf("SetWorkerSlot 0: %v", err)
	}
	if err := dr.SetWorkerSlot("deleg-2", 1, "sess-worker-b", ""); err != nil {
		t.Fatalf("SetWorkerSlot 1: %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	insertAgentSessionForRuns(t, database, wfiID, "sess-worker-a", "_t2_extractor", nil, base.Format(time.RFC3339Nano))
	insertAgentSessionForRuns(t, database, wfiID, "sess-worker-b", "_t2_extractor", nil, base.Add(time.Minute).Format(time.RFC3339Nano))

	got, err := r.ListSystemAgentRuns(50, time.Time{})
	if err != nil {
		t.Fatalf("ListSystemAgentRuns: %v", err)
	}

	count := 0
	for _, run := range got {
		if run.SessionID == "sess-worker-a" || run.SessionID == "sess-worker-b" {
			count++
			if run.DelegationID != "deleg-2" {
				t.Errorf("run %s DelegationID = %q, want deleg-2", run.SessionID, run.DelegationID)
			}
			if run.Fanout != 2 {
				t.Errorf("run %s Fanout = %d, want 2", run.SessionID, run.Fanout)
			}
		}
	}
	if count != 2 {
		t.Fatalf("count of worker rows = %d, want exactly 2 (join must not multiply or drop rows)", count)
	}
}

// TestListSystemAgentRuns_UnspawnedFanoutSlotMatchesNothing verifies an
// unspawned fanout slot (worker_session_ids entry == "") is excluded from
// the json_each join (WHERE je.value <> ”) and does not attach a
// delegation to an unrelated session.
func TestListSystemAgentRuns_UnspawnedFanoutSlotMatchesNothing(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	dr := NewDelegationRepo(database, clock.Real())
	del := &model.Delegation{
		ID:                 "deleg-3",
		CallerSessionID:    "sess-caller-3",
		WorkflowInstanceID: wfiID,
		ProjectID:          "proj",
		Tier:               "executor",
		Brief:              "partial fanout",
		Fanout:             2,
		Depth:              1,
	}
	if err := dr.Create(del); err != nil {
		t.Fatalf("Create delegation: %v", err)
	}
	// Only slot 0 spawned; slot 1 stays "" (unspawned).
	if err := dr.SetWorkerSlot("deleg-3", 0, "sess-worker-spawned", ""); err != nil {
		t.Fatalf("SetWorkerSlot: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	insertAgentSessionForRuns(t, database, wfiID, "sess-worker-spawned", "_t1_executor", nil, now)
	// Unrelated ordinary tiered session that must never pick up deleg-3.
	tier1 := 1
	insertAgentSessionForRuns(t, database, wfiID, "sess-unrelated", "impl", &tier1, now)

	got, err := r.ListSystemAgentRuns(50, time.Time{})
	if err != nil {
		t.Fatalf("ListSystemAgentRuns: %v", err)
	}

	var spawned, unrelated *model.SystemAgentRun
	for _, run := range got {
		switch run.SessionID {
		case "sess-worker-spawned":
			spawned = run
		case "sess-unrelated":
			unrelated = run
		}
	}
	if spawned == nil {
		t.Fatalf("sess-worker-spawned not found in %+v", got)
	}
	if spawned.DelegationID != "deleg-3" {
		t.Errorf("spawned.DelegationID = %q, want deleg-3", spawned.DelegationID)
	}
	if unrelated == nil {
		t.Fatalf("sess-unrelated not found in %+v", got)
	}
	if unrelated.DelegationID != "" {
		t.Errorf("unrelated.DelegationID = %q, want empty (unspawned slot must not attach)", unrelated.DelegationID)
	}
	if unrelated.Fanout != 0 {
		t.Errorf("unrelated.Fanout = %d, want 0", unrelated.Fanout)
	}
}

// TestListSystemAgentRuns_NonDelegateRowHasEmptyDelegationFields verifies a
// plain tiered system-agent row (no delegations row referencing it) returns
// zero-value delegation fields, and that a delegations table with zero rows
// still lists normally.
func TestListSystemAgentRuns_NonDelegateRowHasEmptyDelegationFields(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	tier2 := 2
	now := time.Now().UTC().Format(time.RFC3339Nano)
	insertAgentSessionForRuns(t, database, wfiID, "sess-plain", "impl", &tier2, now)

	got, err := r.ListSystemAgentRuns(50, time.Time{})
	if err != nil {
		t.Fatalf("ListSystemAgentRuns: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	run := got[0]
	if run.SessionID != "sess-plain" {
		t.Fatalf("SessionID = %q, want sess-plain", run.SessionID)
	}
	if run.DelegationID != "" || run.CallerSessionID != "" || run.DelegateTier != "" || run.DelegationStatus != "" {
		t.Errorf("delegation fields = %+v, want all empty for a non-delegate row", run)
	}
	if run.Fanout != 0 {
		t.Errorf("Fanout = %d, want 0", run.Fanout)
	}
}
