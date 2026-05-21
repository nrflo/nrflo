package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestWorkflowPauseFieldsRoundTrip verifies Create/Get/List round-trip for pause_event columns.
func TestWorkflowPauseFieldsRoundTrip(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	seedProjectDB(t, database, "proj")
	r := NewWorkflowRepo(database, clock.Real())

	wf := &model.Workflow{
		ID:                 "wf-pause-rt",
		ProjectID:          "proj",
		ScopeType:          "ticket",
		PauseEventCommand:  "echo pause",
		PauseEventScriptID: "ps-pause",
	}
	if err := r.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("proj", "wf-pause-rt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PauseEventCommand != "echo pause" {
		t.Errorf("Get PauseEventCommand = %q, want %q", got.PauseEventCommand, "echo pause")
	}
	if got.PauseEventScriptID != "ps-pause" {
		t.Errorf("Get PauseEventScriptID = %q, want %q", got.PauseEventScriptID, "ps-pause")
	}

	list, err := r.List("proj")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}
	if list[0].PauseEventCommand != "echo pause" {
		t.Errorf("List PauseEventCommand = %q, want %q", list[0].PauseEventCommand, "echo pause")
	}
	if list[0].PauseEventScriptID != "ps-pause" {
		t.Errorf("List PauseEventScriptID = %q, want %q", list[0].PauseEventScriptID, "ps-pause")
	}
}

// TestWorkflowPauseFieldsDefaultEmpty verifies that omitted pause_event fields default to "".
func TestWorkflowPauseFieldsDefaultEmpty(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	seedProjectDB(t, database, "proj")
	r := NewWorkflowRepo(database, clock.Real())

	wf := &model.Workflow{ID: "wf-pause-empty", ProjectID: "proj", ScopeType: "ticket"}
	if err := r.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("proj", "wf-pause-empty")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PauseEventCommand != "" {
		t.Errorf("default PauseEventCommand = %q, want empty", got.PauseEventCommand)
	}
	if got.PauseEventScriptID != "" {
		t.Errorf("default PauseEventScriptID = %q, want empty", got.PauseEventScriptID)
	}
}

// TestWorkflowPauseFieldsUpdateCommand verifies updating PauseEventCommand leaves PauseEventScriptID untouched.
func TestWorkflowPauseFieldsUpdateCommand(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	seedProjectDB(t, database, "proj")
	r := NewWorkflowRepo(database, clock.Real())

	wf := &model.Workflow{
		ID:                 "wf-pause-upd",
		ProjectID:          "proj",
		ScopeType:          "ticket",
		PauseEventScriptID: "ps-original",
	}
	if err := r.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	cmd := "new-pause-cmd"
	if err := r.Update("proj", "wf-pause-upd", &WorkflowUpdateFields{
		PauseEventCommand: &cmd,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := r.Get("proj", "wf-pause-upd")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.PauseEventCommand != "new-pause-cmd" {
		t.Errorf("PauseEventCommand = %q, want %q", got.PauseEventCommand, "new-pause-cmd")
	}
	if got.PauseEventScriptID != "ps-original" {
		t.Errorf("PauseEventScriptID changed unexpectedly: %q", got.PauseEventScriptID)
	}
}

// TestWorkflowPauseFieldsUpdateScriptID verifies updating PauseEventScriptID leaves PauseEventCommand untouched.
func TestWorkflowPauseFieldsUpdateScriptID(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	seedProjectDB(t, database, "proj")
	r := NewWorkflowRepo(database, clock.Real())

	wf := &model.Workflow{
		ID:                "wf-pause-sid",
		ProjectID:         "proj",
		ScopeType:         "ticket",
		PauseEventCommand: "original-cmd",
	}
	if err := r.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	sid := "ps-new-script"
	if err := r.Update("proj", "wf-pause-sid", &WorkflowUpdateFields{
		PauseEventScriptID: &sid,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := r.Get("proj", "wf-pause-sid")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.PauseEventScriptID != "ps-new-script" {
		t.Errorf("PauseEventScriptID = %q, want %q", got.PauseEventScriptID, "ps-new-script")
	}
	if got.PauseEventCommand != "original-cmd" {
		t.Errorf("PauseEventCommand changed unexpectedly: %q", got.PauseEventCommand)
	}
}

// TestWorkflowPauseFieldsNilPtrNoChange verifies nil pointer leaves pause fields unchanged.
func TestWorkflowPauseFieldsNilPtrNoChange(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	seedProjectDB(t, database, "proj")
	r := NewWorkflowRepo(database, clock.Real())

	wf := &model.Workflow{
		ID:                "wf-pause-nil",
		ProjectID:         "proj",
		ScopeType:         "ticket",
		PauseEventCommand: "keep-me",
	}
	if err := r.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	desc := "updated-desc"
	if err := r.Update("proj", "wf-pause-nil", &WorkflowUpdateFields{
		Description:       &desc,
		PauseEventCommand: nil,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := r.Get("proj", "wf-pause-nil")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Description != "updated-desc" {
		t.Errorf("Description = %q, want %q", got.Description, "updated-desc")
	}
	if got.PauseEventCommand != "keep-me" {
		t.Errorf("PauseEventCommand changed by nil ptr: %q", got.PauseEventCommand)
	}
}

// TestWorkflowPauseFieldsClearCommand verifies clearing PauseEventCommand via empty string.
func TestWorkflowPauseFieldsClearCommand(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	seedProjectDB(t, database, "proj")
	r := NewWorkflowRepo(database, clock.Real())

	wf := &model.Workflow{
		ID:                "wf-pause-clr",
		ProjectID:         "proj",
		ScopeType:         "ticket",
		PauseEventCommand: "to-be-cleared",
	}
	if err := r.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	empty := ""
	if err := r.Update("proj", "wf-pause-clr", &WorkflowUpdateFields{
		PauseEventCommand: &empty,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := r.Get("proj", "wf-pause-clr")
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if got.PauseEventCommand != "" {
		t.Errorf("PauseEventCommand after clear = %q, want empty", got.PauseEventCommand)
	}
}
