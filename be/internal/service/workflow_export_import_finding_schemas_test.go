package service

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/types"
)

// TestExportImport_FindingSchemasRoundTrip verifies that a workflow's
// finding_schemas survive an export -> import cycle. Regression guard: export
// previously omitted finding_schemas from its SELECT and import never passed
// them to CreateWorkflowDef, so bundled workflows arrived schema-less.
func TestExportImport_FindingSchemasRoundTrip(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID:          "wf-schemas",
		Description: "workflow with finding schemas",
		FindingSchemas: []types.FindingSchema{
			{
				Key:     "claims",
				Schema:  json.RawMessage(`{"type":"array"}`),
				Example: json.RawMessage(`[]`),
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-schemas", &types.AgentDefCreateRequest{
		ID:     "agent-a",
		Layer:  0,
		Prompt: "do the thing",
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-schemas"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(bundle.Workflows) != 1 {
		t.Fatalf("bundle.Workflows len = %d, want 1", len(bundle.Workflows))
	}
	// Export must carry the schemas on the embedded workflow model.
	if got := string(bundle.Workflows[0].Workflow.GetFindingSchemas()); !strings.Contains(got, "claims") {
		t.Fatalf("exported finding_schemas = %q, want it to contain key \"claims\"", got)
	}

	proj2 := env.seedProject2(t)
	if _, err := env.exportSvc.Import(proj2, &types.ImportRequest{Bundle: *bundle, Action: "overwrite"}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	def, err := env.workflowSvc.GetWorkflowDef(proj2, "wf-schemas")
	if err != nil {
		t.Fatalf("GetWorkflowDef(proj2): %v", err)
	}
	if len(def.FindingSchemas) != 1 {
		t.Fatalf("imported FindingSchemas len = %d, want 1", len(def.FindingSchemas))
	}
	if def.FindingSchemas[0].Key != "claims" {
		t.Errorf("imported FindingSchemas[0].Key = %q, want \"claims\"", def.FindingSchemas[0].Key)
	}
}
