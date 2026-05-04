package repo

import "testing"

func TestWorkflowChainStepRepo_ListSteps_OrderedByPosition(t *testing.T) {
	t.Parallel()
	r, sr, projectID := setupChainDB(t)

	c := makeChain("chain-ord", projectID, "Ordered")
	if err := r.CreateChain(c); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	// Insert out of order to verify sorting
	for _, s := range []struct {
		id  string
		pos int
	}{
		{"s3", 2}, {"s1", 0}, {"s2", 1},
	} {
		if err := sr.UpsertStep(makeStep(s.id, projectID, "chain-ord", s.pos)); err != nil {
			t.Fatalf("UpsertStep %s: %v", s.id, err)
		}
	}

	steps, err := sr.ListSteps("chain-ord")
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("ListSteps count = %d, want 3", len(steps))
	}
	for i, s := range steps {
		if s.Position != i {
			t.Errorf("steps[%d].Position = %d, want %d", i, s.Position, i)
		}
	}
}

func TestWorkflowChainStepRepo_UpsertStep_Update(t *testing.T) {
	t.Parallel()
	r, sr, projectID := setupChainDB(t)

	c := makeChain("chain-upd-step", projectID, "C")
	if err := r.CreateChain(c); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	s := makeStep("step-upd", projectID, "chain-upd-step", 0)
	if err := sr.UpsertStep(s); err != nil {
		t.Fatalf("UpsertStep insert: %v", err)
	}

	s.WorkflowName = "bugfix"
	s.ScopeType = "project"
	s.BaseInstructions = "updated instructions"
	s.RequireTicketHandoff = true
	if err := sr.UpsertStep(s); err != nil {
		t.Fatalf("UpsertStep update: %v", err)
	}

	steps, err := sr.ListSteps("chain-upd-step")
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("ListSteps count = %d, want 1", len(steps))
	}
	got := steps[0]
	if got.WorkflowName != "bugfix" {
		t.Errorf("WorkflowName = %q, want bugfix", got.WorkflowName)
	}
	if got.ScopeType != "project" {
		t.Errorf("ScopeType = %q, want project", got.ScopeType)
	}
	if got.BaseInstructions != "updated instructions" {
		t.Errorf("BaseInstructions = %q, want updated instructions", got.BaseInstructions)
	}
	if !got.RequireTicketHandoff {
		t.Errorf("RequireTicketHandoff = false, want true")
	}
}

func TestWorkflowChainStepRepo_BulkReorder(t *testing.T) {
	t.Parallel()
	r, sr, projectID := setupChainDB(t)

	c := makeChain("chain-reorder", projectID, "Reorder")
	if err := r.CreateChain(c); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	// Use high initial positions (5, 10) so the new positions (0, 1)
	// assigned by BulkReorder don't conflict with the existing ones.
	s1 := makeStep("step-r1", projectID, "chain-reorder", 5)
	s2 := makeStep("step-r2", projectID, "chain-reorder", 10)
	if err := sr.UpsertStep(s1); err != nil {
		t.Fatalf("UpsertStep s1: %v", err)
	}
	if err := sr.UpsertStep(s2); err != nil {
		t.Fatalf("UpsertStep s2: %v", err)
	}

	// Reorder: s2 first (position=0), s1 second (position=1)
	if err := sr.BulkReorder("chain-reorder", []string{"step-r2", "step-r1"}); err != nil {
		t.Fatalf("BulkReorder: %v", err)
	}

	steps, err := sr.ListSteps("chain-reorder")
	if err != nil {
		t.Fatalf("ListSteps after reorder: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("ListSteps count = %d, want 2", len(steps))
	}
	if steps[0].ID != "step-r2" {
		t.Errorf("steps[0].ID = %q, want step-r2", steps[0].ID)
	}
	if steps[1].ID != "step-r1" {
		t.Errorf("steps[1].ID = %q, want step-r1", steps[1].ID)
	}
}

func TestWorkflowChainStepRepo_DeleteStep(t *testing.T) {
	t.Parallel()
	r, sr, projectID := setupChainDB(t)

	c := makeChain("chain-del-step", projectID, "C")
	if err := r.CreateChain(c); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	s := makeStep("step-del", projectID, "chain-del-step", 0)
	if err := sr.UpsertStep(s); err != nil {
		t.Fatalf("UpsertStep: %v", err)
	}
	if err := sr.DeleteStep("step-del"); err != nil {
		t.Fatalf("DeleteStep first: %v", err)
	}
	if err := sr.DeleteStep("step-del"); err == nil {
		t.Fatalf("DeleteStep second: expected error, got nil")
	}
}
