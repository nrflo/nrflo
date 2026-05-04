package repo

import (
	"testing"

	"be/internal/model"
)

func TestWorkflowChainRunRepo_MaterializeRunSteps_PendingStatus(t *testing.T) {
	t.Parallel()
	rr, sr, projectID, chainID := setupChainRunDB(t)

	run := makeChainRun("run-mat", projectID, chainID, "pending")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	steps := make([]*model.WorkflowChainStep, 3)
	for i := 0; i < 3; i++ {
		sid := "mat-step-" + string(rune('a'+i))
		s := makeStep(sid, projectID, chainID, i)
		if err := sr.UpsertStep(s); err != nil {
			t.Fatalf("UpsertStep %s: %v", sid, err)
		}
		steps[i] = s
	}

	created, err := rr.MaterializeRunSteps("run-mat", steps)
	if err != nil {
		t.Fatalf("MaterializeRunSteps: %v", err)
	}
	if len(created) != 3 {
		t.Fatalf("MaterializeRunSteps count = %d, want 3", len(created))
	}
	for _, rs := range created {
		if rs.Status != "pending" {
			t.Errorf("run step %s: Status = %q, want pending", rs.ID, rs.Status)
		}
		if rs.ChainRunID != "run-mat" {
			t.Errorf("run step %s: ChainRunID = %q, want run-mat", rs.ID, rs.ChainRunID)
		}
		if rs.ID == "" {
			t.Errorf("run step: ID is empty")
		}
	}
}

func TestWorkflowChainRunRepo_GetNextPendingStep_LowestPosition(t *testing.T) {
	t.Parallel()
	rr, sr, projectID, chainID := setupChainRunDB(t)

	run := makeChainRun("run-pend", projectID, chainID, "pending")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	steps := make([]*model.WorkflowChainStep, 3)
	for i := 0; i < 3; i++ {
		sid := "pend-step-" + string(rune('a'+i))
		s := makeStep(sid, projectID, chainID, i)
		if err := sr.UpsertStep(s); err != nil {
			t.Fatalf("UpsertStep %s: %v", sid, err)
		}
		steps[i] = s
	}

	created, err := rr.MaterializeRunSteps("run-pend", steps)
	if err != nil {
		t.Fatalf("MaterializeRunSteps: %v", err)
	}

	next, err := rr.GetNextPendingStep("run-pend")
	if err != nil {
		t.Fatalf("GetNextPendingStep: %v", err)
	}
	if next == nil {
		t.Fatalf("GetNextPendingStep: got nil, want step at position 0")
	}
	if next.Position != 0 {
		t.Errorf("Position = %d, want 0", next.Position)
	}

	// Mark position 0 completed — next should return position 1
	if err := rr.UpdateRunStepStatus(created[0].ID, "completed"); err != nil {
		t.Fatalf("UpdateRunStepStatus completed: %v", err)
	}

	next2, err := rr.GetNextPendingStep("run-pend")
	if err != nil {
		t.Fatalf("GetNextPendingStep after completion: %v", err)
	}
	if next2 == nil {
		t.Fatalf("GetNextPendingStep: got nil, want step at position 1")
	}
	if next2.Position != 1 {
		t.Errorf("Position after completion = %d, want 1", next2.Position)
	}

	// Mark all completed — next should return nil
	if err := rr.UpdateRunStepStatus(created[1].ID, "completed"); err != nil {
		t.Fatalf("UpdateRunStepStatus 1: %v", err)
	}
	if err := rr.UpdateRunStepStatus(created[2].ID, "completed"); err != nil {
		t.Fatalf("UpdateRunStepStatus 2: %v", err)
	}

	nextNil, err := rr.GetNextPendingStep("run-pend")
	if err != nil {
		t.Fatalf("GetNextPendingStep all done: %v", err)
	}
	if nextNil != nil {
		t.Errorf("GetNextPendingStep all done: got %v, want nil", nextNil)
	}
}

func TestWorkflowChainRunRepo_UpdateRunStepStatus_Running_SetsStartedAt(t *testing.T) {
	t.Parallel()
	rr, sr, projectID, chainID := setupChainRunDB(t)

	run := makeChainRun("run-stts", projectID, chainID, "pending")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	s := makeStep("stts-step", projectID, chainID, 0)
	if err := sr.UpsertStep(s); err != nil {
		t.Fatalf("UpsertStep: %v", err)
	}
	created, err := rr.MaterializeRunSteps("run-stts", []*model.WorkflowChainStep{s})
	if err != nil {
		t.Fatalf("MaterializeRunSteps: %v", err)
	}
	stepID := created[0].ID

	if err := rr.UpdateRunStepStatus(stepID, "running"); err != nil {
		t.Fatalf("UpdateRunStepStatus running: %v", err)
	}

	row := rr.db.QueryRow(
		`SELECT status, started_at, ended_at FROM workflow_chain_run_steps WHERE id = ?`, stepID)
	var status string
	var startedAt, endedAt interface{}
	if err := row.Scan(&status, &startedAt, &endedAt); err != nil {
		t.Fatalf("scan after running: %v", err)
	}
	if status != "running" {
		t.Errorf("status = %q, want running", status)
	}
	if startedAt == nil {
		t.Errorf("started_at is nil after running transition")
	}
	if endedAt != nil {
		t.Errorf("ended_at = %v, want nil after running transition", endedAt)
	}
}

func TestWorkflowChainRunRepo_UpdateRunStepStatus_Completed_SetsEndedAt(t *testing.T) {
	t.Parallel()
	rr, sr, projectID, chainID := setupChainRunDB(t)

	run := makeChainRun("run-cend", projectID, chainID, "pending")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	s := makeStep("cend-step", projectID, chainID, 0)
	if err := sr.UpsertStep(s); err != nil {
		t.Fatalf("UpsertStep: %v", err)
	}
	created, err := rr.MaterializeRunSteps("run-cend", []*model.WorkflowChainStep{s})
	if err != nil {
		t.Fatalf("MaterializeRunSteps: %v", err)
	}
	stepID := created[0].ID

	if err := rr.UpdateRunStepStatus(stepID, "completed"); err != nil {
		t.Fatalf("UpdateRunStepStatus completed: %v", err)
	}

	row := rr.db.QueryRow(
		`SELECT status, ended_at FROM workflow_chain_run_steps WHERE id = ?`, stepID)
	var status string
	var endedAt interface{}
	if err := row.Scan(&status, &endedAt); err != nil {
		t.Fatalf("scan after completed: %v", err)
	}
	if status != "completed" {
		t.Errorf("status = %q, want completed", status)
	}
	if endedAt == nil {
		t.Errorf("ended_at is nil after completed transition")
	}
}

func TestWorkflowChainRunRepo_UpdateRunStepStatus_NotFound(t *testing.T) {
	t.Parallel()
	rr, _, _, _ := setupChainRunDB(t)

	if err := rr.UpdateRunStepStatus("no-such-step", "running"); err == nil {
		t.Fatalf("UpdateRunStepStatus missing: expected error, got nil")
	}
}

func TestWorkflowChainRunRepo_SetRunStepInstance_RoundTrip(t *testing.T) {
	t.Parallel()
	rr, sr, projectID, chainID := setupChainRunDB(t)

	run := makeChainRun("run-inst", projectID, chainID, "running")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	s := makeStep("inst-step", projectID, chainID, 0)
	if err := sr.UpsertStep(s); err != nil {
		t.Fatalf("UpsertStep: %v", err)
	}
	created, err := rr.MaterializeRunSteps("run-inst", []*model.WorkflowChainStep{s})
	if err != nil {
		t.Fatalf("MaterializeRunSteps: %v", err)
	}
	stepID := created[0].ID

	if err := rr.SetRunStepInstance(stepID, "wfi-123", "ticket-456", "custom instructions"); err != nil {
		t.Fatalf("SetRunStepInstance: %v", err)
	}

	row := rr.db.QueryRow(
		`SELECT workflow_instance_id, ticket_id, instructions_used FROM workflow_chain_run_steps WHERE id = ?`, stepID)
	var instanceID, ticketID, instructions string
	if err := row.Scan(&instanceID, &ticketID, &instructions); err != nil {
		t.Fatalf("scan SetRunStepInstance: %v", err)
	}
	if instanceID != "wfi-123" {
		t.Errorf("workflow_instance_id = %q, want wfi-123", instanceID)
	}
	if ticketID != "ticket-456" {
		t.Errorf("ticket_id = %q, want ticket-456", ticketID)
	}
	if instructions != "custom instructions" {
		t.Errorf("instructions_used = %q, want custom instructions", instructions)
	}

	if err := rr.SetRunStepInstance("no-such-step", "x", "y", "z"); err == nil {
		t.Fatalf("SetRunStepInstance missing: expected error, got nil")
	}
}

func TestWorkflowChainRunRepo_GetActiveRuns(t *testing.T) {
	t.Parallel()
	rr, _, projectID, chainID := setupChainRunDB(t)

	for _, r := range []struct {
		id     string
		status string
	}{
		{"ar-pending", "pending"},
		{"ar-running1", "running"},
		{"ar-running2", "running"},
		{"ar-completed", "completed"},
	} {
		if err := rr.CreateRun(makeChainRun(r.id, projectID, chainID, r.status)); err != nil {
			t.Fatalf("CreateRun %s: %v", r.id, err)
		}
	}

	active, err := rr.GetActiveRuns()
	if err != nil {
		t.Fatalf("GetActiveRuns: %v", err)
	}

	seen := map[string]bool{}
	for _, run := range active {
		seen[run.ID] = true
		if run.Status != "running" {
			t.Errorf("GetActiveRuns: run %q has status %q, want running", run.ID, run.Status)
		}
	}
	if !seen["ar-running1"] {
		t.Errorf("ar-running1 missing from GetActiveRuns")
	}
	if !seen["ar-running2"] {
		t.Errorf("ar-running2 missing from GetActiveRuns")
	}
	if seen["ar-pending"] {
		t.Errorf("ar-pending should not appear in GetActiveRuns")
	}
	if seen["ar-completed"] {
		t.Errorf("ar-completed should not appear in GetActiveRuns")
	}
}
