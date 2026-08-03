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
	"ticket_update",
	"ticket_add_dependency",
	"web_search",
	"web_fetch",
}

var wantConsoleOnly = []string{
	"workflow_run",
	"workflow_stop",
	"workflow_retry_failed",
	"workflow_get",
	"workflow_wait",
	"workflow_list",
	"project_list",
	"project_status",
	"ticket_list",
	"ticket_get",
	"ticket_current",
	"artifact_list",
	"artifact_get",
	"delegate",
	"get_delegation",
	"merge_delegation",
	"dynamic_workflow",
	"get_subworkflow",
	"revise_plan",
	"approve_plan",
	"consult",
}

// wantExcluded lists session-bound / lifecycle tools that must never appear
// in the console profile.
var wantExcluded = []string{
	"agent_finished", "agent_fail", "agent_continue", "agent_callback", "agent_context_update",
	"findings_add", "findings_add_bulk", "findings_append", "findings_append_bulk",
	"findings_get", "findings_delete", "emit_findings",
	"workflow_skip", "chain_next_instructions", "chain_next_ticket",
	"run_subworkflow", "read_document", "artifact_add",
}

func TestBuildRegistry_ResolvesAllowlist(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
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

// TestBuildRegistry_NilCatalogue_KeepsFullSet verifies the pre-profile
// behavior is unchanged: nil catalogue returns the same set as an explicit
// empty allowlist.
func TestBuildRegistry_NilCatalogue_KeepsFullSet(t *testing.T) {
	env := newConsoleTestEnv(t)
	regNil, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry(nil): %v", err)
	}
	regEmpty, err := BuildRegistry(env.deps, []string{})
	if err != nil {
		t.Fatalf("BuildRegistry(empty): %v", err)
	}
	if len(regNil) != len(regEmpty) {
		t.Fatalf("len(regNil) = %d, len(regEmpty) = %d, want equal", len(regNil), len(regEmpty))
	}
	wantCount := len(wantReusedBuiltins) + len(wantConsoleOnly)
	if len(regNil) != wantCount {
		t.Errorf("len(regNil) = %d, want %d (unrestricted full console set)", len(regNil), wantCount)
	}
}

// TestBuildRegistry_T0DeciderCatalogue_ExactSet verifies the t0-decider
// profile's catalogue filters BuildRegistry down to exactly its allowlist —
// no more, no less — and that fs/bash tools are structurally absent (not
// merely refused at invoke time): they were never composed into reg at all,
// so BuildRegistry(catalogue) can't accidentally leak them back in.
func TestBuildRegistry_T0DeciderCatalogue_ExactSet(t *testing.T) {
	env := newConsoleTestEnv(t)
	profile, err := ProfileByName("t0-decider")
	if err != nil {
		t.Fatalf("ProfileByName: %v", err)
	}
	reg, err := BuildRegistry(env.deps, profile.Catalogue)
	if err != nil {
		t.Fatalf("BuildRegistry(t0-decider catalogue): %v", err)
	}
	if len(reg) != len(profile.Catalogue) {
		t.Fatalf("len(reg) = %d, want %d (exactly the catalogue)", len(reg), len(profile.Catalogue))
	}
	for _, name := range profile.Catalogue {
		if _, ok := reg[name]; !ok {
			t.Errorf("registry missing catalogued tool %q", name)
		}
	}
	for _, banned := range []string{"read_file", "edit_file", "write_file", "bash", "glob", "grep", "web_fetch"} {
		if _, ok := reg[banned]; ok {
			t.Errorf("t0-decider registry unexpectedly contains %q (structurally, this registry composes no fs/bash handler at all)", banned)
		}
	}
	// Every non-catalogued tool from the full set (e.g. project_list) must
	// also be absent — the filter is exact, not additive.
	for _, name := range wantConsoleOnly {
		inCatalogue := false
		for _, c := range profile.Catalogue {
			if c == name {
				inCatalogue = true
			}
		}
		if !inCatalogue {
			if _, ok := reg[name]; ok {
				t.Errorf("t0-decider registry unexpectedly contains non-catalogued tool %q", name)
			}
		}
	}
}

// TestBuildRegistry_T0BareCatalogue_ExactSet mirrors
// TestBuildRegistry_T0DeciderCatalogue_ExactSet for the t0-bare profile: its
// 15-tool catalogue filters BuildRegistry down to exactly that set, and
// fs/bash/findings/artifact/consult/web tools are structurally absent.
func TestBuildRegistry_T0BareCatalogue_ExactSet(t *testing.T) {
	env := newConsoleTestEnv(t)
	profile, err := ProfileByName("t0-bare")
	if err != nil {
		t.Fatalf("ProfileByName: %v", err)
	}
	reg, err := BuildRegistry(env.deps, profile.Catalogue)
	if err != nil {
		t.Fatalf("BuildRegistry(t0-bare catalogue): %v", err)
	}
	if len(reg) != len(profile.Catalogue) {
		t.Fatalf("len(reg) = %d, want %d (exactly the catalogue)", len(reg), len(profile.Catalogue))
	}
	if len(profile.Catalogue) != 15 {
		t.Fatalf("len(profile.Catalogue) = %d, want 15", len(profile.Catalogue))
	}
	for _, name := range profile.Catalogue {
		if _, ok := reg[name]; !ok {
			t.Errorf("registry missing catalogued tool %q", name)
		}
	}
	for _, banned := range []string{
		"read_file", "edit_file", "write_file", "bash", "glob", "grep", "web_fetch",
		"web_search", "consult", "project_findings_add", "ticket_create", "artifact_list",
	} {
		if _, ok := reg[banned]; ok {
			t.Errorf("t0-bare registry unexpectedly contains %q", banned)
		}
	}
}

// TestBuildRegistry_CatalogueNamesUnknownTool_Errors verifies a catalogue
// entry this registry does not compose is a hard error, not a silent drop.
func TestBuildRegistry_CatalogueNamesUnknownTool_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	_, err := BuildRegistry(env.deps, []string{"no_such_tool"})
	if err == nil {
		t.Fatal("BuildRegistry with unknown catalogue entry: want error, got nil")
	}
}

func TestSpecs_SortedByName_ValidObjectSchemas(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
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
