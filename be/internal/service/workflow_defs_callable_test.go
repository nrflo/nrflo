package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// Callable defs must be project-scoped (sub-workflows run without a ticket)
// and non-purging — enforced at marking time, not just at invocation.
func TestCallableAsSubworkflow_RequiresProjectScope(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)
	callable := true

	// Create with default (ticket) scope → rejected.
	_, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID:                    "wf-callable-ticket",
		CallableAsSubworkflow: &callable,
	})
	if err == nil || !strings.Contains(err.Error(), "scope_type=project") {
		t.Fatalf("want project-scope error on create, got %v", err)
	}

	// Project scope → accepted.
	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID:                    "wf-callable-proj",
		ScopeType:             "project",
		CallableAsSubworkflow: &callable,
	}); err != nil {
		t.Fatalf("project-scoped callable create: %v", err)
	}

	// Flipping an already-callable def back to ticket scope → rejected.
	ticket := "ticket"
	err = env.workflowSvc.UpdateWorkflowDef(env.projectID, "wf-callable-proj", &types.WorkflowDefUpdateRequest{
		ScopeType: &ticket,
	})
	if err == nil || !strings.Contains(err.Error(), "scope_type=project") {
		t.Fatalf("want project-scope error on scope flip, got %v", err)
	}

	// Marking an existing ticket-scoped def callable → rejected.
	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID: "wf-plain-ticket",
	}); err != nil {
		t.Fatalf("plain ticket create: %v", err)
	}
	err = env.workflowSvc.UpdateWorkflowDef(env.projectID, "wf-plain-ticket", &types.WorkflowDefUpdateRequest{
		CallableAsSubworkflow: &callable,
	})
	if err == nil || !strings.Contains(err.Error(), "scope_type=project") {
		t.Fatalf("want project-scope error on callable flip, got %v", err)
	}
}
