package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestWorkflowFinalizeFieldsRoundTrip verifies Create/Get/List round-trip for all 4 finalize fields.
func TestWorkflowFinalizeFieldsRoundTrip(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	seedProjectDB(t, database, "proj")
	r := NewWorkflowRepo(database, clock.Real())

	wf := &model.Workflow{
		ID:                      "wf-finalize",
		ProjectID:               "proj",
		ScopeType:               "ticket",
		FinalizeSuccessCommand:  "echo success",
		FinalizeSuccessScriptID: "ps-success",
		FinalizeFailureCommand:  "echo failure",
		FinalizeFailureScriptID: "ps-failure",
	}
	if err := r.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("proj", "wf-finalize")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FinalizeSuccessCommand != "echo success" {
		t.Errorf("Get FinalizeSuccessCommand = %q, want %q", got.FinalizeSuccessCommand, "echo success")
	}
	if got.FinalizeSuccessScriptID != "ps-success" {
		t.Errorf("Get FinalizeSuccessScriptID = %q, want %q", got.FinalizeSuccessScriptID, "ps-success")
	}
	if got.FinalizeFailureCommand != "echo failure" {
		t.Errorf("Get FinalizeFailureCommand = %q, want %q", got.FinalizeFailureCommand, "echo failure")
	}
	if got.FinalizeFailureScriptID != "ps-failure" {
		t.Errorf("Get FinalizeFailureScriptID = %q, want %q", got.FinalizeFailureScriptID, "ps-failure")
	}

	list, err := r.List("proj")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}
	lw := list[0]
	if lw.FinalizeSuccessCommand != "echo success" {
		t.Errorf("List FinalizeSuccessCommand = %q, want %q", lw.FinalizeSuccessCommand, "echo success")
	}
	if lw.FinalizeSuccessScriptID != "ps-success" {
		t.Errorf("List FinalizeSuccessScriptID = %q, want %q", lw.FinalizeSuccessScriptID, "ps-success")
	}
	if lw.FinalizeFailureCommand != "echo failure" {
		t.Errorf("List FinalizeFailureCommand = %q, want %q", lw.FinalizeFailureCommand, "echo failure")
	}
	if lw.FinalizeFailureScriptID != "ps-failure" {
		t.Errorf("List FinalizeFailureScriptID = %q, want %q", lw.FinalizeFailureScriptID, "ps-failure")
	}
}

// TestWorkflowFinalizeFieldsDefaultEmpty verifies that omitted finalize fields default to "".
func TestWorkflowFinalizeFieldsDefaultEmpty(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	seedProjectDB(t, database, "proj")
	r := NewWorkflowRepo(database, clock.Real())

	wf := &model.Workflow{ID: "wf-fin-empty", ProjectID: "proj", ScopeType: "ticket"}
	if err := r.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("proj", "wf-fin-empty")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FinalizeSuccessCommand != "" {
		t.Errorf("default FinalizeSuccessCommand = %q, want empty", got.FinalizeSuccessCommand)
	}
	if got.FinalizeSuccessScriptID != "" {
		t.Errorf("default FinalizeSuccessScriptID = %q, want empty", got.FinalizeSuccessScriptID)
	}
	if got.FinalizeFailureCommand != "" {
		t.Errorf("default FinalizeFailureCommand = %q, want empty", got.FinalizeFailureCommand)
	}
	if got.FinalizeFailureScriptID != "" {
		t.Errorf("default FinalizeFailureScriptID = %q, want empty", got.FinalizeFailureScriptID)
	}
}

// TestWorkflowFinalizeFieldsUpdateTriState verifies tri-state update semantics per field.
func TestWorkflowFinalizeFieldsUpdateTriState(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	seedProjectDB(t, database, "proj")
	r := NewWorkflowRepo(database, clock.Real())

	wf := &model.Workflow{
		ID:                     "wf-fin-upd",
		ProjectID:              "proj",
		ScopeType:              "ticket",
		FinalizeSuccessCommand: "original-cmd",
	}
	if err := r.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Step 1: update only FinalizeFailureCommand; success fields stay untouched.
	failCmd := "fail-cmd"
	if err := r.Update("proj", "wf-fin-upd", &WorkflowUpdateFields{
		FinalizeFailureCommand: &failCmd,
	}); err != nil {
		t.Fatalf("Update step 1: %v", err)
	}
	got, err := r.Get("proj", "wf-fin-upd")
	if err != nil {
		t.Fatalf("Get step 1: %v", err)
	}
	if got.FinalizeSuccessCommand != "original-cmd" {
		t.Errorf("step 1: FinalizeSuccessCommand = %q, want %q", got.FinalizeSuccessCommand, "original-cmd")
	}
	if got.FinalizeFailureCommand != "fail-cmd" {
		t.Errorf("step 1: FinalizeFailureCommand = %q, want %q", got.FinalizeFailureCommand, "fail-cmd")
	}

	// Step 2: set FinalizeSuccessScriptID; verify other fields untouched.
	scriptID := "ps-success-id"
	if err := r.Update("proj", "wf-fin-upd", &WorkflowUpdateFields{
		FinalizeSuccessScriptID: &scriptID,
	}); err != nil {
		t.Fatalf("Update step 2: %v", err)
	}
	got, err = r.Get("proj", "wf-fin-upd")
	if err != nil {
		t.Fatalf("Get step 2: %v", err)
	}
	if got.FinalizeSuccessScriptID != "ps-success-id" {
		t.Errorf("step 2: FinalizeSuccessScriptID = %q, want %q", got.FinalizeSuccessScriptID, "ps-success-id")
	}
	if got.FinalizeSuccessCommand != "original-cmd" {
		t.Errorf("step 2: FinalizeSuccessCommand changed unexpectedly: %q", got.FinalizeSuccessCommand)
	}

	// Step 3: clear FinalizeSuccessCommand via &""; script_id from step 2 must remain.
	empty := ""
	if err := r.Update("proj", "wf-fin-upd", &WorkflowUpdateFields{
		FinalizeSuccessCommand: &empty,
	}); err != nil {
		t.Fatalf("Update step 3: %v", err)
	}
	got, err = r.Get("proj", "wf-fin-upd")
	if err != nil {
		t.Fatalf("Get step 3: %v", err)
	}
	if got.FinalizeSuccessCommand != "" {
		t.Errorf("step 3: FinalizeSuccessCommand = %q, want empty", got.FinalizeSuccessCommand)
	}
	if got.FinalizeSuccessScriptID != "ps-success-id" {
		t.Errorf("step 3: FinalizeSuccessScriptID changed unexpectedly: %q", got.FinalizeSuccessScriptID)
	}

	// Step 4: nil pointer in update — must not touch FinalizeSuccessCommand.
	desc := "new-desc"
	if err := r.Update("proj", "wf-fin-upd", &WorkflowUpdateFields{
		Description:            &desc,
		FinalizeSuccessCommand: nil,
	}); err != nil {
		t.Fatalf("Update step 4: %v", err)
	}
	got, err = r.Get("proj", "wf-fin-upd")
	if err != nil {
		t.Fatalf("Get step 4: %v", err)
	}
	if got.Description != "new-desc" {
		t.Errorf("step 4: Description = %q, want %q", got.Description, "new-desc")
	}
	if got.FinalizeSuccessCommand != "" {
		t.Errorf("step 4: FinalizeSuccessCommand changed by nil ptr: %q", got.FinalizeSuccessCommand)
	}
}
