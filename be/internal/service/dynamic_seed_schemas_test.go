package service

import (
	"encoding/json"
	"testing"
)

// TestDynFindingSchemasValid guards the bundled schemas: the seed inserts
// them via direct SQL (bypassing validation), so a malformed schema or
// non-conforming example would otherwise only surface as an emit_findings
// failure at runtime.
func TestDynFindingSchemasValid(t *testing.T) {
	t.Parallel()
	defs := parseFindingSchemas(dynFindingSchemas)
	if len(defs) != 8 {
		t.Fatalf("parsed %d finding schemas, want 8", len(defs))
	}
	if err := ValidateFindingSchemas(defs); err != nil {
		t.Fatalf("bundled dynamic finding schemas are invalid: %v", err)
	}
}

// TestDynFindingSchemas_CoverEveryTemplateEmitKey verifies every
// fanout_template's FindingKey has a declared schema — otherwise
// FindingsService.Emit would reject that template's emit at runtime with no
// schema to validate against.
func TestDynFindingSchemas_CoverEveryTemplateEmitKey(t *testing.T) {
	t.Parallel()
	defs := parseFindingSchemas(dynFindingSchemas)
	declared := make(map[string]bool, len(defs))
	for _, d := range defs {
		declared[d.Key] = true
	}
	for _, a := range dynAgents {
		if a.FindingKey == "" {
			continue // planner emits to the reserved _workflow_plan key, not a workflow finding_schemas entry
		}
		if !declared[a.FindingKey] {
			t.Errorf("dynAgents[%q]: emits to finding key %q, which has no declared schema", a.ID, a.FindingKey)
		}
	}
}

// TestDynFindingSchemas_ExcludesReservedPlanKey verifies the workflow's own
// finding_schemas never declares the server-owned _workflow_plan key — it is
// resolved ahead of a workflow's finding_schemas and ValidateFindingSchemas
// hard-rejects declaring it (see plan_schema_test.go).
func TestDynFindingSchemas_ExcludesReservedPlanKey(t *testing.T) {
	t.Parallel()
	defs := parseFindingSchemas(dynFindingSchemas)
	for _, d := range defs {
		if d.Key == WorkflowPlanFindingKey {
			t.Fatalf("dynFindingSchemas declares the reserved key %q", WorkflowPlanFindingKey)
		}
	}
}

// TestDynFindingSchemas_WorkflowFinalResultAcceptsPlainString verifies the
// synthesizer's completion contract: workflow_final_result must validate a
// plain non-empty string, since notify/render.go reads it with strVal and
// get_subworkflow returns it verbatim as the caller-visible result.
func TestDynFindingSchemas_WorkflowFinalResultAcceptsPlainString(t *testing.T) {
	t.Parallel()
	defs := parseFindingSchemas(dynFindingSchemas)
	var schema json.RawMessage
	for _, d := range defs {
		if d.Key == "workflow_final_result" {
			schema = d.Schema
		}
	}
	if schema == nil {
		t.Fatal("workflow_final_result schema not found")
	}
	sch, err := compileJSONSchema(string(schema))
	if err != nil {
		t.Fatalf("compile workflow_final_result schema: %v", err)
	}
	if err := sch.Validate("Final deliverable text."); err != nil {
		t.Errorf("workflow_final_result schema rejected a plain string: %v", err)
	}
	if err := sch.Validate(""); err == nil {
		t.Error("workflow_final_result schema accepted an empty string, want rejection (minLength:1)")
	}
	if err := sch.Validate(map[string]any{"foo": "bar"}); err == nil {
		t.Error("workflow_final_result schema accepted a non-string value, want rejection")
	}
}
