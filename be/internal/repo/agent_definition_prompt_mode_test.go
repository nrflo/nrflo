package repo

import (
	"database/sql"
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestAgentDefinition_PromptMode_DefaultsToFullWithNilSteps verifies that
// creating a def without setting PromptMode/Steps yields PromptMode=="full"
// and Steps==nil (nil, not a pointer to "").
func TestAgentDefinition_PromptMode_DefaultsToFullWithNilSteps(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-pm1", "wf-pm1")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if err := r.Create(&model.AgentDefinition{
		ID: "no-pm", ProjectID: "proj-pm1", WorkflowID: "wf-pm1",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "sonnet-5",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("proj-pm1", "wf-pm1", "no-pm")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptMode != "full" {
		t.Errorf("PromptMode = %q, want full (default when unset)", got.PromptMode)
	}
	if got.Steps != nil {
		t.Errorf("Steps = %v, want nil", got.Steps)
	}
}

// TestAgentDefinition_PromptMode_RoundTripsCreateGetList verifies PromptMode
// and Steps survive Create, Get, and List.
func TestAgentDefinition_PromptMode_RoundTripsCreateGetList(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-pm2", "wf-pm2")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	stepsJSON := `[{"step_id":"s1","title":"Step 1","instruction":"do the thing"}]`
	if err := r.Create(&model.AgentDefinition{
		ID: "with-pm", ProjectID: "proj-pm2", WorkflowID: "wf-pm2",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "sonnet-5",
		PromptMode: "stepwise", Steps: &stepsJSON,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("proj-pm2", "wf-pm2", "with-pm")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptMode != "stepwise" {
		t.Errorf("PromptMode = %q, want stepwise", got.PromptMode)
	}
	if got.Steps == nil || *got.Steps != stepsJSON {
		t.Errorf("Steps = %v, want %q", got.Steps, stepsJSON)
	}

	all, err := r.List("proj-pm2", "wf-pm2")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("List count = %d, want 1", len(all))
	}
	if all[0].PromptMode != "stepwise" || all[0].Steps == nil || *all[0].Steps != stepsJSON {
		t.Errorf("List[0] PromptMode/Steps = %q/%v, want stepwise/%q", all[0].PromptMode, all[0].Steps, stepsJSON)
	}
}

// TestAgentDefinition_PromptMode_UpdateWritesBothColumns verifies
// AgentDefUpdateFields.PromptMode/Steps write both columns, and Steps as
// sql.NullString{Valid:false} writes SQL NULL.
func TestAgentDefinition_PromptMode_UpdateWritesBothColumns(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-pm3", "wf-pm3")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if err := r.Create(&model.AgentDefinition{
		ID: "upd-pm", ProjectID: "proj-pm3", WorkflowID: "wf-pm3",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "sonnet-5",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	stepsJSON := `[{"step_id":"s1","title":"Step 1","instruction":"do the thing"}]`
	mode := "stepwise"
	if err := r.Update("proj-pm3", "wf-pm3", "upd-pm", &AgentDefUpdateFields{
		PromptMode: &mode,
		Steps:      &sql.NullString{String: stepsJSON, Valid: true},
	}); err != nil {
		t.Fatalf("Update (set stepwise+steps): %v", err)
	}
	got, err := r.Get("proj-pm3", "wf-pm3", "upd-pm")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptMode != "stepwise" {
		t.Errorf("PromptMode = %q, want stepwise", got.PromptMode)
	}
	if got.Steps == nil || *got.Steps != stepsJSON {
		t.Errorf("Steps = %v, want %q", got.Steps, stepsJSON)
	}

	full := "full"
	if err := r.Update("proj-pm3", "wf-pm3", "upd-pm", &AgentDefUpdateFields{
		PromptMode: &full,
		Steps:      &sql.NullString{Valid: false},
	}); err != nil {
		t.Fatalf("Update (revert to full, clear steps): %v", err)
	}
	got, err = r.Get("proj-pm3", "wf-pm3", "upd-pm")
	if err != nil {
		t.Fatalf("Get after revert: %v", err)
	}
	if got.PromptMode != "full" {
		t.Errorf("PromptMode after revert = %q, want full", got.PromptMode)
	}
	if got.Steps != nil {
		t.Errorf("Steps after Valid:false update = %v, want nil (SQL NULL)", got.Steps)
	}
}

// TestAgentDefinition_PromptMode_NilFieldsAreNoOp verifies that leaving
// PromptMode/Steps nil on AgentDefUpdateFields does not touch stored values.
func TestAgentDefinition_PromptMode_NilFieldsAreNoOp(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-pm4", "wf-pm4")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	stepsJSON := `[{"step_id":"s1","title":"Step 1","instruction":"do the thing"}]`
	if err := r.Create(&model.AgentDefinition{
		ID: "noop-pm", ProjectID: "proj-pm4", WorkflowID: "wf-pm4",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "sonnet-5",
		PromptMode: "stepwise", Steps: &stepsJSON,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tag := "unrelated"
	if err := r.Update("proj-pm4", "wf-pm4", "noop-pm", &AgentDefUpdateFields{Tag: &tag}); err != nil {
		t.Fatalf("Update (unrelated field): %v", err)
	}

	got, err := r.Get("proj-pm4", "wf-pm4", "noop-pm")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptMode != "stepwise" {
		t.Errorf("PromptMode after unrelated update = %q, want unchanged stepwise", got.PromptMode)
	}
	if got.Steps == nil || *got.Steps != stepsJSON {
		t.Errorf("Steps after unrelated update = %v, want unchanged %q", got.Steps, stepsJSON)
	}
}
