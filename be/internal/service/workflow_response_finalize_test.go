package service

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
)

// TestBuildV4State_FinalizeResult_Present verifies that when a _finalize WFI
// finding is set, buildV4State includes finalize_result with the expected sub-fields.
func TestBuildV4State_FinalizeResult_Present(t *testing.T) {
	t.Parallel()
	pool, svc := setupEndlessLoopTestEnv(t)

	wfiID := "wfi-finalize-present"
	insertWFI(t, pool, wfiID, "wf-proj", "project", false, false)

	finalize := map[string]interface{}{
		"slot":        "finalize_success",
		"kind":        "command",
		"target":      "echo done",
		"exit_code":   float64(0),
		"status":      "pass",
		"output_tail": "done\n",
		"timestamp":   "2026-05-21T00:00:00Z",
	}
	raw, err := json.Marshal(finalize)
	if err != nil {
		t.Fatalf("marshal _finalize: %v", err)
	}
	fr := repo.NewFindingRepo(pool, clock.Real())
	if err := fr.Upsert("workflow_instance", wfiID, "_finalize", json.RawMessage(raw), repo.Denorm{}, repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("Upsert _finalize: %v", err)
	}

	wi, err := svc.GetProjectWorkflowInstance("proj1", "wf-proj")
	if err != nil {
		t.Fatalf("GetProjectWorkflowInstance: %v", err)
	}

	state := svc.buildV4State(wi)

	frResult, ok := state["finalize_result"]
	if !ok {
		t.Fatal("finalize_result key must be present when _finalize finding is set")
	}
	frMap, ok := frResult.(map[string]interface{})
	if !ok {
		t.Fatalf("finalize_result is not a map, got %T", frResult)
	}
	if frMap["status"] != "pass" {
		t.Errorf("finalize_result.status = %v, want %q", frMap["status"], "pass")
	}
	if frMap["exit_code"] != float64(0) {
		t.Errorf("finalize_result.exit_code = %v, want %v", frMap["exit_code"], float64(0))
	}
	if frMap["slot"] != "finalize_success" {
		t.Errorf("finalize_result.slot = %v, want %q", frMap["slot"], "finalize_success")
	}
}

// TestBuildV4State_FinalizeResult_Absent verifies that finalize_result key is
// NOT present in buildV4State output when no _finalize finding has been set.
func TestBuildV4State_FinalizeResult_Absent(t *testing.T) {
	t.Parallel()
	pool, svc := setupEndlessLoopTestEnv(t)

	wfiID := "wfi-finalize-absent"
	insertWFI(t, pool, wfiID, "wf-proj", "project", false, false)
	// No _finalize finding upserted.

	wi, err := svc.GetProjectWorkflowInstance("proj1", "wf-proj")
	if err != nil {
		t.Fatalf("GetProjectWorkflowInstance: %v", err)
	}

	state := svc.buildV4State(wi)

	if _, ok := state["finalize_result"]; ok {
		t.Error("finalize_result must NOT be present when no _finalize finding is set")
	}
}

// TestBuildV4State_FinalizeResult_InvalidJSON verifies that a malformed _finalize
// finding does not emit finalize_result (json.Unmarshal failure guard).
func TestBuildV4State_FinalizeResult_InvalidJSON(t *testing.T) {
	t.Parallel()
	pool, svc := setupEndlessLoopTestEnv(t)

	wfiID := "wfi-finalize-badjson"
	insertWFI(t, pool, wfiID, "wf-proj", "project", false, false)

	fr := repo.NewFindingRepo(pool, clock.Real())
	// Store a raw string (not a JSON object) so unmarshal into map[string]interface{} fails.
	if err := fr.Upsert("workflow_instance", wfiID, "_finalize", json.RawMessage(`"not-an-object"`), repo.Denorm{}, repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("Upsert _finalize: %v", err)
	}

	wi, err := svc.GetProjectWorkflowInstance("proj1", "wf-proj")
	if err != nil {
		t.Fatalf("GetProjectWorkflowInstance: %v", err)
	}

	state := svc.buildV4State(wi)

	if _, ok := state["finalize_result"]; ok {
		t.Error("finalize_result must NOT be present when _finalize JSON is not an object")
	}
}
