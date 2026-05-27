package service

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/types"
)

func TestWorkflowDef_FindingSchemasRoundTrip(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	defs := []types.FindingSchema{
		fs("security_issues",
			`{"type":"array","items":{"type":"object","properties":{"file":{"type":"string"}},"required":["file"]}}`,
			`[{"file":"a.go"}]`),
	}
	if _, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:             "wf-fs",
		ScopeType:      "ticket",
		FindingSchemas: defs,
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	got, err := svc.GetWorkflowDef("proj1", "wf-fs")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if len(got.FindingSchemas) != 1 || got.FindingSchemas[0].Key != "security_issues" {
		t.Fatalf("finding_schemas did not round-trip: %+v", got.FindingSchemas)
	}

	// Update to clear, then confirm empty.
	empty := []types.FindingSchema{}
	if err := svc.UpdateWorkflowDef("proj1", "wf-fs", &types.WorkflowDefUpdateRequest{FindingSchemas: &empty}); err != nil {
		t.Fatalf("UpdateWorkflowDef clear: %v", err)
	}
	got, err = svc.GetWorkflowDef("proj1", "wf-fs")
	if err != nil {
		t.Fatalf("GetWorkflowDef after clear: %v", err)
	}
	if len(got.FindingSchemas) != 0 {
		t.Fatalf("expected cleared finding_schemas, got %+v", got.FindingSchemas)
	}
}

func TestWorkflowDef_FindingSchemasInvalidRejected(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	bad := []types.FindingSchema{
		{Key: "k", Schema: json.RawMessage(`{"type":"array","items":{"type":"object","required":["x"]}}`), Example: json.RawMessage(`[{}]`)},
	}
	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:             "wf-bad",
		ScopeType:      "ticket",
		FindingSchemas: bad,
	})
	if err == nil || !strings.Contains(err.Error(), "does not satisfy") {
		t.Fatalf("expected example-mismatch rejection, got %v", err)
	}
}
