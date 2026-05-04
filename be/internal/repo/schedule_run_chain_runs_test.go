package repo

import (
	"encoding/json"
	"testing"

	"be/internal/model"
)

// TestScheduleRunRepo_Insert_ChainRunsRoundTrip verifies that ChainRuns
// round-trips through Insert → Get correctly.
func TestScheduleRunRepo_Insert_ChainRunsRoundTrip(t *testing.T) {
	t.Parallel()
	env := setupScheduleRunDB(t)

	run := &model.ScheduleRun{
		ID:              "run-chain-rt",
		ScheduledTaskID: env.taskID,
		ProjectID:       env.projectID,
		Status:          "triggered",
		Workflows:       []model.ScheduleRunWorkflow{},
		ChainRuns: []model.ScheduleRunChain{
			{ChainID: "chain-1", ChainRunID: "cr-abc"},
			{ChainID: "chain-2", ChainRunID: "cr-def", Error: "some error"},
		},
	}
	if err := env.runRepo.Insert(run); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := env.runRepo.Get("run-chain-rt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.ChainRuns) != 2 {
		t.Fatalf("ChainRuns len = %d, want 2", len(got.ChainRuns))
	}
	if got.ChainRuns[0].ChainID != "chain-1" || got.ChainRuns[0].ChainRunID != "cr-abc" {
		t.Errorf("ChainRuns[0] = %+v, want {chain-1 cr-abc}", got.ChainRuns[0])
	}
	if got.ChainRuns[1].ChainRunID != "cr-def" {
		t.Errorf("ChainRuns[1].ChainRunID = %q, want cr-def", got.ChainRuns[1].ChainRunID)
	}
	if got.ChainRuns[1].Error != "some error" {
		t.Errorf("ChainRuns[1].Error = %q, want 'some error'", got.ChainRuns[1].Error)
	}
}

// TestScheduleRunRepo_Insert_NilChainRuns_DefaultsEmpty verifies that nil ChainRuns
// becomes an empty non-nil slice after Insert → Get.
func TestScheduleRunRepo_Insert_NilChainRuns_DefaultsEmpty(t *testing.T) {
	t.Parallel()
	env := setupScheduleRunDB(t)

	run := makeRun("run-nil-chains", env.taskID, env.projectID, "triggered")
	run.ChainRuns = nil
	if err := env.runRepo.Insert(run); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := env.runRepo.Get("run-nil-chains")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ChainRuns == nil {
		t.Error("ChainRuns = nil, want empty slice")
	}
	if len(got.ChainRuns) != 0 {
		t.Errorf("ChainRuns len = %d, want 0", len(got.ChainRuns))
	}
}

// TestScheduleRunRepo_UpdateStatusFull_PersistsChainRuns verifies that
// UpdateStatusFull writes both workflow and chain run JSON to the DB.
func TestScheduleRunRepo_UpdateStatusFull_PersistsChainRuns(t *testing.T) {
	t.Parallel()
	env := setupScheduleRunDB(t)

	run := makeRun("run-full-upd", env.taskID, env.projectID, "pending")
	if err := env.runRepo.Insert(run); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	wfs := []model.ScheduleRunWorkflow{{Workflow: "feature", InstanceID: "inst-abc"}}
	chains := []model.ScheduleRunChain{
		{ChainID: "chain-1", ChainRunID: "cr-123"},
		{ChainID: "chain-2", Error: "failed to create"},
	}
	wfsJSON, _ := json.Marshal(wfs)
	chainJSON, _ := json.Marshal(chains)

	if err := env.runRepo.UpdateStatusFull("run-full-upd", "triggered", string(wfsJSON), string(chainJSON), ""); err != nil {
		t.Fatalf("UpdateStatusFull: %v", err)
	}

	got, err := env.runRepo.Get("run-full-upd")
	if err != nil {
		t.Fatalf("Get after UpdateStatusFull: %v", err)
	}
	if got.Status != "triggered" {
		t.Errorf("Status = %q, want triggered", got.Status)
	}
	if len(got.Workflows) != 1 || got.Workflows[0].InstanceID != "inst-abc" {
		t.Errorf("Workflows = %+v, unexpected", got.Workflows)
	}
	if len(got.ChainRuns) != 2 {
		t.Fatalf("ChainRuns len = %d, want 2", len(got.ChainRuns))
	}
	if got.ChainRuns[0].ChainRunID != "cr-123" {
		t.Errorf("ChainRuns[0].ChainRunID = %q, want cr-123", got.ChainRuns[0].ChainRunID)
	}
	if got.ChainRuns[1].Error != "failed to create" {
		t.Errorf("ChainRuns[1].Error = %q, want 'failed to create'", got.ChainRuns[1].Error)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty", got.Error)
	}
}

// TestScheduleRunRepo_UpdateStatusFull_WithErrorMsg verifies that the error
// message field is also persisted by UpdateStatusFull.
func TestScheduleRunRepo_UpdateStatusFull_WithErrorMsg(t *testing.T) {
	t.Parallel()
	env := setupScheduleRunDB(t)

	run := makeRun("run-full-err", env.taskID, env.projectID, "pending")
	if err := env.runRepo.Insert(run); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := env.runRepo.UpdateStatusFull("run-full-err", "failed", "[]", "[]", "connection timeout"); err != nil {
		t.Fatalf("UpdateStatusFull: %v", err)
	}

	got, err := env.runRepo.Get("run-full-err")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	if got.Error != "connection timeout" {
		t.Errorf("Error = %q, want 'connection timeout'", got.Error)
	}
}

// TestScheduleRunRepo_UpdateStatusFull_NotFound verifies that UpdateStatusFull
// returns an error when no run with that ID exists.
func TestScheduleRunRepo_UpdateStatusFull_NotFound(t *testing.T) {
	t.Parallel()
	env := setupScheduleRunDB(t)

	err := env.runRepo.UpdateStatusFull("no-such-run", "triggered", "[]", "[]", "")
	if err == nil {
		t.Fatal("UpdateStatusFull missing run: expected error, got nil")
	}
}

// TestScheduleRunRepo_UpdateStatusFull_OverwritesChainRuns verifies that a second
// UpdateStatusFull call replaces the existing chain_runs JSON.
func TestScheduleRunRepo_UpdateStatusFull_OverwritesChainRuns(t *testing.T) {
	t.Parallel()
	env := setupScheduleRunDB(t)

	run := &model.ScheduleRun{
		ID:              "run-overwrite",
		ScheduledTaskID: env.taskID,
		ProjectID:       env.projectID,
		Status:          "pending",
		Workflows:       []model.ScheduleRunWorkflow{},
		ChainRuns:       []model.ScheduleRunChain{{ChainID: "old-chain", ChainRunID: "old-run"}},
	}
	if err := env.runRepo.Insert(run); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	newChains := []model.ScheduleRunChain{{ChainID: "new-chain", ChainRunID: "new-run"}}
	chainJSON, _ := json.Marshal(newChains)
	if err := env.runRepo.UpdateStatusFull("run-overwrite", "triggered", "[]", string(chainJSON), ""); err != nil {
		t.Fatalf("UpdateStatusFull: %v", err)
	}

	got, err := env.runRepo.Get("run-overwrite")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.ChainRuns) != 1 || got.ChainRuns[0].ChainID != "new-chain" {
		t.Errorf("ChainRuns = %+v, want [{new-chain new-run}]", got.ChainRuns)
	}
}
