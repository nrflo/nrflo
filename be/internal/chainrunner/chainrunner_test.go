package chainrunner

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// newChainRunnerTestDB creates an isolated test DB and returns its file path.
func newChainRunnerTestDB(t *testing.T) (string, *db.Pool) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "chainrunner_test.db")
	if err := chainCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return dbPath, pool
}

// seedChainRunWithSteps seeds a project, chain, run with numSteps materialized steps.
func seedChainRunWithSteps(t *testing.T, pool *db.Pool, runID, status string, numSteps int) {
	t.Helper()
	clk := clock.Real()

	_, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at) VALUES ('proj-cr-test', 'T', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	cr := repo.NewWorkflowChainRepo(pool, clk)
	chainID := "chain-cr-test-" + runID
	if err := cr.CreateChain(&model.WorkflowChain{
		ID:        chainID,
		ProjectID: "proj-cr-test",
		Name:      "Test",
	}); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	sr := repo.NewWorkflowChainStepRepo(pool, clk)
	steps := make([]*model.WorkflowChainStep, numSteps)
	for i := 0; i < numSteps; i++ {
		steps[i] = &model.WorkflowChainStep{
			ID:           runID + "-step-" + string(rune('0'+i)),
			ProjectID:    "proj-cr-test",
			ChainID:      chainID,
			Position:     i,
			WorkflowName: "feature",
			ScopeType:    "project",
		}
		if err := sr.UpsertStep(steps[i]); err != nil {
			t.Fatalf("UpsertStep %d: %v", i, err)
		}
	}

	rr := repo.NewWorkflowChainRunRepo(pool, clk)
	run := &model.WorkflowChainRun{
		ID:        runID,
		ProjectID: "proj-cr-test",
		ChainID:   chainID,
		Status:    status,
	}
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := rr.MaterializeRunSteps(runID, steps); err != nil {
		t.Fatalf("MaterializeRunSteps: %v", err)
	}
}

func TestFailAllRunning_MarksRunningAsFailed(t *testing.T) {
	t.Parallel()
	dbPath, pool := newChainRunnerTestDB(t)

	// Seed one running run with 2 steps
	seedChainRunWithSteps(t, pool, "zombie-run-1", "running", 2)

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(hub.Stop)

	runner := New(nil, dbPath, hub, clock.Real())
	runner.FailAllRunning()

	rr := repo.NewWorkflowChainRunRepo(pool, clock.Real())
	run, err := rr.GetRun("zombie-run-1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != "failed" {
		t.Errorf("Status = %q, want failed", run.Status)
	}

	steps, err := rr.ListRunSteps("zombie-run-1")
	if err != nil {
		t.Fatalf("ListRunSteps: %v", err)
	}
	for _, s := range steps {
		if s.Status != "canceled" {
			t.Errorf("step %d Status = %q, want canceled", s.Position, s.Status)
		}
	}
}

func TestFailAllRunning_SkipsNonRunning(t *testing.T) {
	t.Parallel()
	dbPath, pool := newChainRunnerTestDB(t)

	seedChainRunWithSteps(t, pool, "pending-run", "pending", 1)
	seedChainRunWithSteps(t, pool, "completed-run", "completed", 1)

	runner := New(nil, dbPath, nil, clock.Real())
	runner.FailAllRunning() // should be a no-op for non-running runs

	rr := repo.NewWorkflowChainRunRepo(pool, clock.Real())

	pending, _ := rr.GetRun("pending-run")
	if pending.Status != "pending" {
		t.Errorf("pending-run Status = %q, want pending", pending.Status)
	}

	completed, _ := rr.GetRun("completed-run")
	if completed.Status != "completed" {
		t.Errorf("completed-run Status = %q, want completed", completed.Status)
	}
}

func TestFailAllRunning_MultipleZombies(t *testing.T) {
	t.Parallel()
	dbPath, pool := newChainRunnerTestDB(t)

	seedChainRunWithSteps(t, pool, "zombie-a", "running", 1)
	seedChainRunWithSteps(t, pool, "zombie-b", "running", 2)

	runner := New(nil, dbPath, nil, clock.Real())
	runner.FailAllRunning()

	rr := repo.NewWorkflowChainRunRepo(pool, clock.Real())
	for _, runID := range []string{"zombie-a", "zombie-b"} {
		run, err := rr.GetRun(runID)
		if err != nil {
			t.Fatalf("GetRun %s: %v", runID, err)
		}
		if run.Status != "failed" {
			t.Errorf("%s Status = %q, want failed", runID, run.Status)
		}
		steps, _ := rr.ListRunSteps(runID)
		for _, s := range steps {
			if s.Status != "canceled" {
				t.Errorf("%s step %d Status = %q, want canceled", runID, s.Position, s.Status)
			}
		}
	}
}

func TestRunner_IsRunning_FalseWhenNotStarted(t *testing.T) {
	t.Parallel()
	dbPath, _ := newChainRunnerTestDB(t)

	runner := New(nil, dbPath, nil, clock.Real())
	if runner.IsRunning("any-run-id") {
		t.Error("IsRunning should be false for unstrarted run")
	}
}

func TestRunner_Cancel_PendingRun(t *testing.T) {
	t.Parallel()
	dbPath, pool := newChainRunnerTestDB(t)

	seedChainRunWithSteps(t, pool, "cancel-pending", "pending", 1)

	runner := New(nil, dbPath, nil, clock.Real())

	if err := runner.Cancel("cancel-pending"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	rr := repo.NewWorkflowChainRunRepo(pool, clock.Real())
	run, err := rr.GetRun("cancel-pending")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != "canceled" {
		t.Errorf("Status = %q, want canceled", run.Status)
	}
}

func TestRunner_Cancel_NotRunning(t *testing.T) {
	t.Parallel()
	dbPath, pool := newChainRunnerTestDB(t)

	seedChainRunWithSteps(t, pool, "cancel-completed", "completed", 1)

	runner := New(nil, dbPath, nil, clock.Real())

	if err := runner.Cancel("cancel-completed"); err == nil {
		t.Fatal("expected error canceling completed run, got nil")
	}
}

func TestRunner_Start_RejectsNonPending(t *testing.T) {
	t.Parallel()
	dbPath, pool := newChainRunnerTestDB(t)

	seedChainRunWithSteps(t, pool, "start-completed", "completed", 1)

	runner := New(nil, dbPath, nil, clock.Real())

	if err := runner.Start(context.Background(), "start-completed"); err == nil {
		t.Fatal("expected error starting non-pending run, got nil")
	}
}

func TestRunner_WaitAll_NoRuns(t *testing.T) {
	t.Parallel()
	dbPath, _ := newChainRunnerTestDB(t)
	runner := New(nil, dbPath, nil, clock.Real())
	// Should return immediately when no runs are active.
	runner.WaitAll(100 * time.Millisecond)
}
