package service

import (
	"encoding/json"
	"testing"

	"be/internal/types"
)

// TestLoadFindingSchemas_GlobalFallback verifies that emit-time finding-schema
// resolution falls back to the global project: a global workflow's instance
// runs under the selected project, but its finding_schemas live under
// GlobalProjectID and must still be found.
func TestLoadFindingSchemas_GlobalFallback(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	if _, err := env.pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Global', NULL, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')`,
		GlobalProjectID); err != nil {
		t.Fatalf("seed global project: %v", err)
	}
	if _, err := env.workflowSvc.CreateWorkflowDef(GlobalProjectID, &types.WorkflowDefCreateRequest{
		ID: "gwf",
		FindingSchemas: []types.FindingSchema{
			{Key: "claims", Schema: json.RawMessage(`{"type":"array"}`), Example: json.RawMessage(`[]`)},
		},
	}); err != nil {
		t.Fatalf("create global workflow: %v", err)
	}

	// projectID has no "gwf" row -> loadFindingSchemas falls back to GlobalProjectID.
	defs, err := loadFindingSchemas(env.pool, env.projectID, "gwf")
	if err != nil {
		t.Fatalf("loadFindingSchemas (global fallback): %v", err)
	}
	if len(defs) != 1 || defs[0].Key != "claims" {
		t.Fatalf("defs = %+v, want one 'claims' schema", defs)
	}
}
