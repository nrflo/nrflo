package repo

import (
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// setupTwoStepRun creates an isolated DB with a chain run containing 2 materialized
// steps and sets workflow_instance_id on step 0. Returns the repo, instance ID for
// step 0, and the run ID.
func setupTwoStepRun(t *testing.T) (*WorkflowChainRunRepo, string, string) {
	t.Helper()
	database := newTestDB(t)
	_, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-ts', 'T', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	cr := NewWorkflowChainRepo(database, clock.Real())
	if err := cr.CreateChain(makeChain("chain-ts", "proj-ts", "TS Chain")); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}
	sr := NewWorkflowChainStepRepo(database, clock.Real())
	steps := []*model.WorkflowChainStep{
		makeStep("ts-step-0", "proj-ts", "chain-ts", 0),
		makeStep("ts-step-1", "proj-ts", "chain-ts", 1),
	}
	for _, s := range steps {
		if err := sr.UpsertStep(s); err != nil {
			t.Fatalf("UpsertStep %s: %v", s.ID, err)
		}
	}
	rr := NewWorkflowChainRunRepo(database, clock.Real())
	run := makeChainRun("run-ts", "proj-ts", "chain-ts", "running")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	created, err := rr.MaterializeRunSteps("run-ts", steps)
	if err != nil {
		t.Fatalf("MaterializeRunSteps: %v", err)
	}
	const instanceID = "wfi-ts-step0"
	if err := rr.SetRunStepInstance(created[0].ID, instanceID, "ticket-ts", "init"); err != nil {
		t.Fatalf("SetRunStepInstance step0: %v", err)
	}
	// Mark step 0 as running so GetNextPendingStep skips it and returns step 1.
	if err := rr.UpdateRunStepStatus(created[0].ID, "running"); err != nil {
		t.Fatalf("UpdateRunStepStatus step0: %v", err)
	}
	return rr, instanceID, "run-ts"
}

func TestWorkflowChainRunRepo_GetRunStepByInstanceID_Found(t *testing.T) {
	t.Parallel()
	rr, instanceID, _ := setupTwoStepRun(t)

	step, err := rr.GetRunStepByInstanceID(instanceID)
	if err != nil {
		t.Fatalf("GetRunStepByInstanceID(%q): %v", instanceID, err)
	}
	if step == nil {
		t.Fatalf("GetRunStepByInstanceID: got nil")
	}
	if !step.WorkflowInstanceID.Valid || step.WorkflowInstanceID.String != instanceID {
		t.Errorf("WorkflowInstanceID = %v, want %q", step.WorkflowInstanceID, instanceID)
	}
	if step.Position != 0 {
		t.Errorf("Position = %d, want 0", step.Position)
	}
	if step.ChainRunID != "run-ts" {
		t.Errorf("ChainRunID = %q, want run-ts", step.ChainRunID)
	}
}

func TestWorkflowChainRunRepo_GetRunStepByInstanceID_NotFound(t *testing.T) {
	t.Parallel()
	rr, _, _ := setupTwoStepRun(t)

	_, err := rr.GetRunStepByInstanceID("no-such-instance")
	if err == nil {
		t.Fatal("expected error for missing instance_id, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found: %v", err)
	}
}

func TestWorkflowChainRunRepo_SetNextPendingStepInstructions_HappyPath(t *testing.T) {
	t.Parallel()
	rr, instanceID, runID := setupTwoStepRun(t)

	if err := rr.SetNextPendingStepInstructions(instanceID, "next instructions"); err != nil {
		t.Fatalf("SetNextPendingStepInstructions: %v", err)
	}

	next, err := rr.GetNextPendingStep(runID)
	if err != nil {
		t.Fatalf("GetNextPendingStep: %v", err)
	}
	if next == nil {
		t.Fatal("expected pending step after instructions update, got nil")
	}
	if next.Position != 1 {
		t.Errorf("Position = %d, want 1", next.Position)
	}
	if next.InstructionsUsed != "next instructions" {
		t.Errorf("InstructionsUsed = %q, want next instructions", next.InstructionsUsed)
	}
}

func TestWorkflowChainRunRepo_SetNextPendingStepInstructions_UnknownInstance(t *testing.T) {
	t.Parallel()
	rr, _, _ := setupTwoStepRun(t)

	err := rr.SetNextPendingStepInstructions("no-such-instance", "instructions")
	if err == nil {
		t.Fatal("expected error for unknown instance_id, got nil")
	}
}

func TestWorkflowChainRunRepo_SetNextPendingStepInstructions_NoNextStep(t *testing.T) {
	t.Parallel()
	// Single-step run — no position+1 pending step exists.
	database := newTestDB(t)
	_, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-nns', 'T', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	cr := NewWorkflowChainRepo(database, clock.Real())
	if err := cr.CreateChain(makeChain("chain-nns", "proj-nns", "C")); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}
	sr := NewWorkflowChainStepRepo(database, clock.Real())
	s := makeStep("nns-step-0", "proj-nns", "chain-nns", 0)
	if err := sr.UpsertStep(s); err != nil {
		t.Fatalf("UpsertStep: %v", err)
	}
	rr := NewWorkflowChainRunRepo(database, clock.Real())
	run := makeChainRun("run-nns", "proj-nns", "chain-nns", "running")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	created, err := rr.MaterializeRunSteps("run-nns", []*model.WorkflowChainStep{s})
	if err != nil {
		t.Fatalf("MaterializeRunSteps: %v", err)
	}
	if err := rr.SetRunStepInstance(created[0].ID, "wfi-nns-0", "", ""); err != nil {
		t.Fatalf("SetRunStepInstance: %v", err)
	}

	if err := rr.SetNextPendingStepInstructions("wfi-nns-0", "instructions"); err == nil {
		t.Fatal("expected error when no next pending step exists, got nil")
	}
}

func TestWorkflowChainRunRepo_SetNextPendingStepTicket_HappyPath(t *testing.T) {
	t.Parallel()
	rr, instanceID, runID := setupTwoStepRun(t)

	if err := rr.SetNextPendingStepTicket(instanceID, "TICKET-99"); err != nil {
		t.Fatalf("SetNextPendingStepTicket: %v", err)
	}

	steps, err := rr.ListRunSteps(runID)
	if err != nil {
		t.Fatalf("ListRunSteps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(steps))
	}
	if !steps[1].TicketID.Valid || steps[1].TicketID.String != "TICKET-99" {
		t.Errorf("step[1].TicketID = %v, want TICKET-99", steps[1].TicketID)
	}
}

func TestWorkflowChainRunRepo_SetNextPendingStepTicket_UnknownInstance(t *testing.T) {
	t.Parallel()
	rr, _, _ := setupTwoStepRun(t)

	if err := rr.SetNextPendingStepTicket("no-such-instance", "TICKET-1"); err == nil {
		t.Fatal("expected error for unknown instance_id, got nil")
	}
}

func TestWorkflowChainRunRepo_ListRunSteps_OrderedByPosition(t *testing.T) {
	t.Parallel()
	rr, _, runID := setupTwoStepRun(t)

	steps, err := rr.ListRunSteps(runID)
	if err != nil {
		t.Fatalf("ListRunSteps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(steps))
	}
	if steps[0].Position != 0 {
		t.Errorf("steps[0].Position = %d, want 0", steps[0].Position)
	}
	if steps[1].Position != 1 {
		t.Errorf("steps[1].Position = %d, want 1", steps[1].Position)
	}
	if steps[0].ChainRunID != runID {
		t.Errorf("steps[0].ChainRunID = %q, want %q", steps[0].ChainRunID, runID)
	}
}

func TestWorkflowChainRunRepo_ListRunSteps_Empty(t *testing.T) {
	t.Parallel()
	rr, _, _, _ := setupChainRunDB(t)

	run := makeChainRun("run-empty-ls", "proj-cr", "run-chain", "pending")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	steps, err := rr.ListRunSteps("run-empty-ls")
	if err != nil {
		t.Fatalf("ListRunSteps: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("len(steps) = %d, want 0", len(steps))
	}
}
