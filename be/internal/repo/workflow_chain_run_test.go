package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// setupChainRunDB creates a test DB with a project and chain, returning repos and IDs.
func setupChainRunDB(t *testing.T) (*WorkflowChainRunRepo, *WorkflowChainStepRepo, string, string) {
	t.Helper()
	database := newTestDB(t)
	_, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-cr', 'Test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	cr := NewWorkflowChainRepo(database, clock.Real())
	if err := cr.CreateChain(makeChain("run-chain", "proj-cr", "Run Chain")); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}
	runRepo := NewWorkflowChainRunRepo(database, clock.Real())
	stepRepo := NewWorkflowChainStepRepo(database, clock.Real())
	return runRepo, stepRepo, "proj-cr", "run-chain"
}

func makeChainRun(id, projectID, chainID, status string) *model.WorkflowChainRun {
	return &model.WorkflowChainRun{
		ID:                  id,
		ProjectID:           projectID,
		ChainID:             chainID,
		Status:              status,
		InitialInstructions: "start instructions",
		TriggeredBy:         "user",
		CurrentPosition:     0,
	}
}

func TestWorkflowChainRunRepo_CreateRun_GetRun(t *testing.T) {
	t.Parallel()
	rr, _, projectID, chainID := setupChainRunDB(t)

	run := makeChainRun("run-1", projectID, chainID, "pending")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	got, err := rr.GetRun("run-1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.ID != "run-1" {
		t.Errorf("ID = %q, want run-1", got.ID)
	}
	if got.ProjectID != projectID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, projectID)
	}
	if got.ChainID != chainID {
		t.Errorf("ChainID = %q, want %q", got.ChainID, chainID)
	}
	if got.Status != "pending" {
		t.Errorf("Status = %q, want pending", got.Status)
	}
	if got.InitialInstructions != "start instructions" {
		t.Errorf("InitialInstructions = %q, want start instructions", got.InitialInstructions)
	}
	if got.StartedAt != nil {
		t.Errorf("StartedAt = %v, want nil", got.StartedAt)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil", got.CompletedAt)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero")
	}
}

func TestWorkflowChainRunRepo_GetRun_NotFound(t *testing.T) {
	t.Parallel()
	rr, _, _, _ := setupChainRunDB(t)

	_, err := rr.GetRun("no-such-run")
	if err == nil {
		t.Fatalf("GetRun missing: expected error, got nil")
	}
}

func TestWorkflowChainRunRepo_ListRuns_StatusFilter(t *testing.T) {
	t.Parallel()
	rr, _, projectID, chainID := setupChainRunDB(t)

	for _, r := range []struct {
		id     string
		status string
	}{
		{"lr-1", "pending"},
		{"lr-2", "running"},
		{"lr-3", "completed"},
	} {
		if err := rr.CreateRun(makeChainRun(r.id, projectID, chainID, r.status)); err != nil {
			t.Fatalf("CreateRun %s: %v", r.id, err)
		}
	}

	all, err := rr.ListRuns(projectID, "")
	if err != nil {
		t.Fatalf("ListRuns all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListRuns all count = %d, want 3", len(all))
	}

	pending, err := rr.ListRuns(projectID, "pending")
	if err != nil {
		t.Fatalf("ListRuns pending: %v", err)
	}
	if len(pending) != 1 || pending[0].Status != "pending" {
		t.Errorf("ListRuns pending: got %v, want 1 pending run", pending)
	}

	none, err := rr.ListRuns("other-proj", "")
	if err != nil {
		t.Fatalf("ListRuns other-proj: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("ListRuns other-proj count = %d, want 0", len(none))
	}
}

func TestWorkflowChainRunRepo_UpdateRunStatus_Running_SetsStartedAt(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	database := newTestDB(t)
	_, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p-rs', 'T', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	cr := NewWorkflowChainRepo(database, clk)
	if err := cr.CreateChain(makeChain("rs-chain", "p-rs", "C")); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}
	rr := NewWorkflowChainRunRepo(database, clk)
	run := makeChainRun("run-rs", "p-rs", "rs-chain", "pending")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	clk.Advance(time.Second)
	if err := rr.UpdateRunStatus("run-rs", "running"); err != nil {
		t.Fatalf("UpdateRunStatus running: %v", err)
	}

	got, err := rr.GetRun("run-rs")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want running", got.Status)
	}
	if got.StartedAt == nil {
		t.Fatalf("StartedAt is nil, want non-nil")
	}
}

func TestWorkflowChainRunRepo_UpdateRunStatus_Completed_SetsCompletedAt(t *testing.T) {
	t.Parallel()
	rr, _, projectID, chainID := setupChainRunDB(t)

	run := makeChainRun("run-cmp", projectID, chainID, "running")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := rr.UpdateRunStatus("run-cmp", "completed"); err != nil {
		t.Fatalf("UpdateRunStatus completed: %v", err)
	}

	got, err := rr.GetRun("run-cmp")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if got.CompletedAt == nil {
		t.Fatalf("CompletedAt is nil, want non-nil")
	}
}

func TestWorkflowChainRunRepo_UpdateRunStatus_NotFound(t *testing.T) {
	t.Parallel()
	rr, _, _, _ := setupChainRunDB(t)

	if err := rr.UpdateRunStatus("no-such", "running"); err == nil {
		t.Fatalf("UpdateRunStatus missing: expected error, got nil")
	}
}

func TestWorkflowChainRunRepo_SetCurrentPosition(t *testing.T) {
	t.Parallel()
	rr, _, projectID, chainID := setupChainRunDB(t)

	run := makeChainRun("run-pos", projectID, chainID, "pending")
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	if err := rr.SetCurrentPosition("run-pos", 3); err != nil {
		t.Fatalf("SetCurrentPosition: %v", err)
	}

	got, err := rr.GetRun("run-pos")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.CurrentPosition != 3 {
		t.Errorf("CurrentPosition = %d, want 3", got.CurrentPosition)
	}

	if err := rr.SetCurrentPosition("no-such-run", 1); err == nil {
		t.Fatalf("SetCurrentPosition missing: expected error, got nil")
	}
}
