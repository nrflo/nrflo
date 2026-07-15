package console

import (
	"encoding/json"
	"testing"
)

// wantReusedBuiltins mirrors reusedBuiltins() — kept as an independent literal
// so the test fails loudly (not silently) if the allowlist drifts.
var wantReusedBuiltins = []string{
	"project_findings_add",
	"project_findings_add_bulk",
	"project_findings_append",
	"project_findings_append_bulk",
	"project_findings_get",
	"project_findings_delete",
	"workflow_continue",
	"workflow_fail",
	"ticket_create",
	"ticket_add_dependency",
	"web_search",
	"web_fetch",
}

var wantConsoleOnly = []string{
	"workflow_run",
	"workflow_stop",
	"workflow_retry_failed",
	"workflow_get",
	"workflow_list",
	"project_list",
	"project_status",
	"ticket_list",
	"ticket_get",
	"artifact_list",
	"artifact_get",
	"deep_research",
}

// wantExcluded lists session-bound / lifecycle tools that must never appear
// in the console profile.
var wantExcluded = []string{
	"agent_finished", "agent_fail", "agent_continue", "agent_callback", "agent_context_update",
	"findings_add", "findings_add_bulk", "findings_append", "findings_append_bulk",
	"findings_get", "findings_delete", "emit_findings",
	"workflow_skip", "chain_next_instructions", "chain_next_ticket",
	"run_subworkflow", "get_subworkflow", "dynamic_workflow", "revise_plan", "approve_plan",
	"consult", "read_document", "artifact_add",
}

func TestBuildRegistry_ResolvesAllowlist(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	wantCount := len(wantReusedBuiltins) + len(wantConsoleOnly)
	if len(reg) != wantCount {
		t.Errorf("len(reg) = %d, want %d", len(reg), wantCount)
	}
	for _, name := range append(append([]string{}, wantReusedBuiltins...), wantConsoleOnly...) {
		if _, ok := reg[name]; !ok {
			t.Errorf("registry missing expected tool %q", name)
		}
	}
	for _, name := range wantExcluded {
		if _, ok := reg[name]; ok {
			t.Errorf("registry unexpectedly contains excluded tool %q", name)
		}
	}
}

func TestSpecs_SortedByName_ValidObjectSchemas(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	specs := Specs(reg)
	if len(specs) != len(reg) {
		t.Fatalf("len(specs) = %d, want %d", len(specs), len(reg))
	}
	for i := 1; i < len(specs); i++ {
		if specs[i-1].Name >= specs[i].Name {
			t.Errorf("specs not sorted: %q >= %q at index %d", specs[i-1].Name, specs[i].Name, i)
		}
	}
	for _, sp := range specs {
		if sp.Name == "" {
			t.Errorf("spec has empty name")
		}
		if len(sp.InputSchema) == 0 {
			t.Errorf("%s: empty InputSchema", sp.Name)
			continue
		}
		var schema map[string]interface{}
		if err := json.Unmarshal(sp.InputSchema, &schema); err != nil {
			t.Errorf("%s: InputSchema does not unmarshal: %v", sp.Name, err)
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("%s: InputSchema type = %v, want \"object\"", sp.Name, schema["type"])
		}
	}
}
