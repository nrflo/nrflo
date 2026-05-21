package service

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
)

// TestBuildV4State_PauseResult_Present verifies that when a _pause WFI finding is set,
// buildV4State includes pause_result with the expected sub-fields.
func TestBuildV4State_PauseResult_Present(t *testing.T) {
	t.Parallel()
	pool, svc := setupEndlessLoopTestEnv(t)

	wfiID := "wfi-pause-present"
	insertWFI(t, pool, wfiID, "wf-proj", "project", false, false)

	pause := map[string]interface{}{
		"paused_after_layer": float64(0),
		"resume_layer":       float64(1),
		"event": map[string]interface{}{
			"kind":        "command",
			"target":      "echo paused",
			"exit_code":   float64(0),
			"status":      "ok",
			"output_tail": "paused\n",
		},
		"timestamp": "2026-05-21T00:00:00Z",
	}
	raw, err := json.Marshal(pause)
	if err != nil {
		t.Fatalf("marshal _pause: %v", err)
	}
	fr := repo.NewFindingRepo(pool, clock.Real())
	if err := fr.Upsert("workflow_instance", wfiID, "_pause", json.RawMessage(raw), repo.Denorm{}, repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("Upsert _pause: %v", err)
	}

	wi, err := svc.GetProjectWorkflowInstance("proj1", "wf-proj")
	if err != nil {
		t.Fatalf("GetProjectWorkflowInstance: %v", err)
	}

	state := svc.buildV4State(wi)

	prResult, ok := state["pause_result"]
	if !ok {
		t.Fatal("pause_result key must be present when _pause finding is set")
	}
	prMap, ok := prResult.(map[string]interface{})
	if !ok {
		t.Fatalf("pause_result is not a map, got %T", prResult)
	}
	if prMap["resume_layer"] != float64(1) {
		t.Errorf("pause_result.resume_layer = %v, want %v", prMap["resume_layer"], float64(1))
	}
	if prMap["paused_after_layer"] != float64(0) {
		t.Errorf("pause_result.paused_after_layer = %v, want %v", prMap["paused_after_layer"], float64(0))
	}
}

// TestBuildV4State_PauseResult_Absent verifies that pause_result key is NOT present
// in buildV4State output when no _pause finding has been set.
func TestBuildV4State_PauseResult_Absent(t *testing.T) {
	t.Parallel()
	pool, svc := setupEndlessLoopTestEnv(t)

	wfiID := "wfi-pause-absent"
	insertWFI(t, pool, wfiID, "wf-proj", "project", false, false)
	// No _pause finding upserted.

	wi, err := svc.GetProjectWorkflowInstance("proj1", "wf-proj")
	if err != nil {
		t.Fatalf("GetProjectWorkflowInstance: %v", err)
	}

	state := svc.buildV4State(wi)

	if _, ok := state["pause_result"]; ok {
		t.Error("pause_result must NOT be present when no _pause finding is set")
	}
}
