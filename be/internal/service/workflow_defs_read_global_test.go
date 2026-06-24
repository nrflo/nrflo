package service

import (
	"encoding/json"
	"testing"

	"be/internal/types"
)

// TestGetWorkflowDef_GlobalFallback verifies that GetWorkflowDef resolves a
// workflow defined under GlobalProjectID when queried from another project, and
// that its agent definitions (phases) + finding schemas load from the global
// project too. This is the keystone that fixes all GetWorkflowDef callers
// (progress, Init/CompletePhase, next_workflow_on_success, observer, chain).
func TestGetWorkflowDef_GlobalFallback(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	if _, err := env.pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Global', NULL, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')`,
		GlobalProjectID); err != nil {
		t.Fatalf("seed global project: %v", err)
	}
	if _, err := env.workflowSvc.CreateWorkflowDef(GlobalProjectID, &types.WorkflowDefCreateRequest{
		ID:        "gwf2",
		ScopeType: "project",
		FindingSchemas: []types.FindingSchema{
			{Key: "claims", Schema: json.RawMessage(`{"type":"array"}`), Example: json.RawMessage(`[]`)},
		},
	}); err != nil {
		t.Fatalf("create global workflow: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(GlobalProjectID, "gwf2", &types.AgentDefCreateRequest{
		ID:     "scope",
		Layer:  0,
		Prompt: "do the thing",
	}); err != nil {
		t.Fatalf("create global agent def: %v", err)
	}

	// Query from a DIFFERENT project -> falls back to global.
	def, err := env.workflowSvc.GetWorkflowDef(env.projectID, "gwf2")
	if err != nil {
		t.Fatalf("GetWorkflowDef global fallback: %v", err)
	}
	if len(def.Phases) != 1 || def.Phases[0].Agent != "scope" {
		t.Errorf("phases = %+v, want one 'scope' phase (agent defs resolved from global)", def.Phases)
	}
	if len(def.FindingSchemas) != 1 || def.FindingSchemas[0].Key != "claims" {
		t.Errorf("finding schemas = %+v, want one 'claims'", def.FindingSchemas)
	}

	// Genuinely missing workflow still errors.
	if _, err := env.workflowSvc.GetWorkflowDef(env.projectID, "does-not-exist"); err == nil {
		t.Fatal("expected error for unknown workflow")
	}
}
