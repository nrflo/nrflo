package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/types"
)

// TestCreateWorkflowDef_NextOnSuccess_SelfRef verifies that a workflow cannot reference itself.
func TestCreateWorkflowDef_NextOnSuccess_SelfRef(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "wf1",
		ScopeType:             "project",
		NextWorkflowOnSuccess: "wf1",
	})
	if err == nil {
		t.Fatal("expected error for self-referencing next_workflow_on_success, got nil")
	}
	if !strings.Contains(err.Error(), "cannot reference itself") {
		t.Errorf("expected error containing 'cannot reference itself', got: %v", err)
	}
}

// TestCreateWorkflowDef_NextOnSuccess_SelfRef_CaseInsensitive verifies case-insensitive self-ref rejection.
func TestCreateWorkflowDef_NextOnSuccess_SelfRef_CaseInsensitive(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	// ID stored as lowercase "wf-ci", target supplied as uppercase
	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "wf-ci",
		ScopeType:             "project",
		NextWorkflowOnSuccess: "WF-CI",
	})
	if err == nil {
		t.Fatal("expected error for case-insensitive self-ref, got nil")
	}
	if !strings.Contains(err.Error(), "cannot reference itself") {
		t.Errorf("expected error containing 'cannot reference itself', got: %v", err)
	}
}

// TestCreateWorkflowDef_NextOnSuccess_NonExistent verifies that referencing a missing workflow is rejected.
func TestCreateWorkflowDef_NextOnSuccess_NonExistent(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "src",
		ScopeType:             "project",
		NextWorkflowOnSuccess: "missing-wf",
	})
	if err == nil {
		t.Fatal("expected error for non-existent target, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected error containing 'does not exist', got: %v", err)
	}
}

// TestCreateWorkflowDef_NextOnSuccess_TicketScoped verifies that ticket-scoped targets are rejected.
func TestCreateWorkflowDef_NextOnSuccess_TicketScoped(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	// Create the target as ticket-scoped.
	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:        "ticket-target",
		ScopeType: "ticket",
	})
	if err != nil {
		t.Fatalf("setup create ticket-scoped target: %v", err)
	}

	_, err = svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "src",
		ScopeType:             "project",
		NextWorkflowOnSuccess: "ticket-target",
	})
	if err == nil {
		t.Fatal("expected error for ticket-scoped target, got nil")
	}
	if !strings.Contains(err.Error(), "not project-scoped") {
		t.Errorf("expected error containing 'not project-scoped', got: %v", err)
	}
}

// TestCreateWorkflowDef_NextOnSuccess_ProjectScoped_OK verifies the happy path.
func TestCreateWorkflowDef_NextOnSuccess_ProjectScoped_OK(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	// Create the target as project-scoped.
	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:        "proj-target",
		ScopeType: "project",
	})
	if err != nil {
		t.Fatalf("setup create project-scoped target: %v", err)
	}

	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "src",
		ScopeType:             "project",
		NextWorkflowOnSuccess: "proj-target",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef with valid project-scoped target: %v", err)
	}
	if wf == nil {
		t.Fatal("expected non-nil workflow")
	}

	// Verify Get returns the field.
	def, err := svc.GetWorkflowDef("proj1", "src")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if def.NextWorkflowOnSuccess != "proj-target" {
		t.Errorf("GetWorkflowDef NextWorkflowOnSuccess = %q, want %q", def.NextWorkflowOnSuccess, "proj-target")
	}
}

// TestCreateWorkflowDef_NextOnSuccess_Empty verifies that empty string is accepted (no target).
func TestCreateWorkflowDef_NextOnSuccess_Empty(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "wf-noref",
		ScopeType:             "project",
		NextWorkflowOnSuccess: "",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef with empty NextWorkflowOnSuccess: %v", err)
	}
	if wf == nil {
		t.Fatal("expected non-nil workflow")
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-noref")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if def.NextWorkflowOnSuccess != "" {
		t.Errorf("GetWorkflowDef NextWorkflowOnSuccess = %q, want empty", def.NextWorkflowOnSuccess)
	}
}

// TestCreateWorkflowDef_NextOnSuccess_CrossProject verifies that a cross-project reference is rejected.
func TestCreateWorkflowDef_NextOnSuccess_CrossProject(t *testing.T) {
	t.Parallel()
	pool, svc := setupWorkflowDefsTestEnv(t)

	// Insert a second project.
	now := time.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P2', '/tmp/p2', ?, ?)`,
		"proj2", now, now,
	); err != nil {
		t.Fatalf("insert proj2: %v", err)
	}

	// Create a project-scoped workflow in proj2.
	_, err := svc.CreateWorkflowDef("proj2", &types.WorkflowDefCreateRequest{
		ID:        "proj2-wf",
		ScopeType: "project",
	})
	if err != nil {
		t.Fatalf("create proj2 workflow: %v", err)
	}

	// Try to reference proj2's workflow from proj1.
	_, err = svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "src",
		ScopeType:             "project",
		NextWorkflowOnSuccess: "proj2-wf",
	})
	if err == nil {
		t.Fatal("expected error for cross-project target, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected error containing 'does not exist', got: %v", err)
	}
}

// TestUpdateWorkflowDef_NextOnSuccess_TriState verifies tri-state update semantics:
//   - nil pointer → leaves the value untouched
//   - &"target" → sets the value
//   - &"" → clears the value
func TestUpdateWorkflowDef_NextOnSuccess_TriState(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	// Create target and source workflows.
	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:        "target",
		ScopeType: "project",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	_, err = svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:        "src",
		ScopeType: "project",
	})
	if err != nil {
		t.Fatalf("create src: %v", err)
	}

	// Step 1: nil pointer — field must remain empty.
	if err := svc.UpdateWorkflowDef("proj1", "src", &types.WorkflowDefUpdateRequest{
		NextWorkflowOnSuccess: nil,
	}); err != nil {
		t.Fatalf("update with nil: %v", err)
	}
	def, err := svc.GetWorkflowDef("proj1", "src")
	if err != nil {
		t.Fatalf("get after nil update: %v", err)
	}
	if def.NextWorkflowOnSuccess != "" {
		t.Errorf("after nil update: NextWorkflowOnSuccess = %q, want empty", def.NextWorkflowOnSuccess)
	}

	// Step 2: set to "target" — field must reflect the new value.
	target := "target"
	if err := svc.UpdateWorkflowDef("proj1", "src", &types.WorkflowDefUpdateRequest{
		NextWorkflowOnSuccess: &target,
	}); err != nil {
		t.Fatalf("update to set target: %v", err)
	}
	def, err = svc.GetWorkflowDef("proj1", "src")
	if err != nil {
		t.Fatalf("get after set: %v", err)
	}
	if def.NextWorkflowOnSuccess != "target" {
		t.Errorf("after set update: NextWorkflowOnSuccess = %q, want %q", def.NextWorkflowOnSuccess, "target")
	}

	// Step 3: nil pointer again — value must remain "target" (not cleared).
	if err := svc.UpdateWorkflowDef("proj1", "src", &types.WorkflowDefUpdateRequest{
		NextWorkflowOnSuccess: nil,
	}); err != nil {
		t.Fatalf("second nil update: %v", err)
	}
	def, err = svc.GetWorkflowDef("proj1", "src")
	if err != nil {
		t.Fatalf("get after second nil update: %v", err)
	}
	if def.NextWorkflowOnSuccess != "target" {
		t.Errorf("after second nil update: NextWorkflowOnSuccess = %q, want %q", def.NextWorkflowOnSuccess, "target")
	}

	// Step 4: &"" — field must be cleared.
	empty := ""
	if err := svc.UpdateWorkflowDef("proj1", "src", &types.WorkflowDefUpdateRequest{
		NextWorkflowOnSuccess: &empty,
	}); err != nil {
		t.Fatalf("update to clear: %v", err)
	}
	def, err = svc.GetWorkflowDef("proj1", "src")
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	if def.NextWorkflowOnSuccess != "" {
		t.Errorf("after clear update: NextWorkflowOnSuccess = %q, want empty", def.NextWorkflowOnSuccess)
	}
}

// TestUpdateWorkflowDef_NextOnSuccess_SelfRef verifies self-reference rejection on update.
func TestUpdateWorkflowDef_NextOnSuccess_SelfRef(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:        "self-wf",
		ScopeType: "project",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	selfRef := "self-wf"
	err = svc.UpdateWorkflowDef("proj1", "self-wf", &types.WorkflowDefUpdateRequest{
		NextWorkflowOnSuccess: &selfRef,
	})
	if err == nil {
		t.Fatal("expected error for self-referencing update, got nil")
	}
	if !strings.Contains(err.Error(), "cannot reference itself") {
		t.Errorf("expected error containing 'cannot reference itself', got: %v", err)
	}
}

// TestUpdateWorkflowDef_NextOnSuccess_TicketScoped verifies ticket-scoped rejection on update.
func TestUpdateWorkflowDef_NextOnSuccess_TicketScoped(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:        "tgt-ticket",
		ScopeType: "ticket",
	})
	if err != nil {
		t.Fatalf("setup target: %v", err)
	}
	_, err = svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:        "src",
		ScopeType: "project",
	})
	if err != nil {
		t.Fatalf("setup src: %v", err)
	}

	ref := "tgt-ticket"
	err = svc.UpdateWorkflowDef("proj1", "src", &types.WorkflowDefUpdateRequest{
		NextWorkflowOnSuccess: &ref,
	})
	if err == nil {
		t.Fatal("expected error for ticket-scoped target in update, got nil")
	}
	if !strings.Contains(err.Error(), "not project-scoped") {
		t.Errorf("expected error containing 'not project-scoped', got: %v", err)
	}
}

// TestListWorkflowDefs_NextOnSuccess verifies that ListWorkflowDefs returns the field.
func TestListWorkflowDefs_NextOnSuccess(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:        "tgt",
		ScopeType: "project",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	_, err = svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "src",
		ScopeType:             "project",
		NextWorkflowOnSuccess: "tgt",
	})
	if err != nil {
		t.Fatalf("create src: %v", err)
	}

	defs, err := svc.ListWorkflowDefs("proj1")
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}

	src, ok := defs["src"]
	if !ok {
		t.Fatal("src not in result")
	}
	if src.NextWorkflowOnSuccess != "tgt" {
		t.Errorf("ListWorkflowDefs src.NextWorkflowOnSuccess = %q, want %q", src.NextWorkflowOnSuccess, "tgt")
	}

	tgt, ok := defs["tgt"]
	if !ok {
		t.Fatal("tgt not in result")
	}
	if tgt.NextWorkflowOnSuccess != "" {
		t.Errorf("ListWorkflowDefs tgt.NextWorkflowOnSuccess = %q, want empty", tgt.NextWorkflowOnSuccess)
	}
}
