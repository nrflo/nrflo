package service

import (
	"testing"

	"be/internal/types"
)

// TestCreateAgentDef_PlannerToolsRequireEmitFindings exercises
// validateNodeRole's planner branch (agent_definition_validate.go) at the
// AgentDefinitionService.CreateAgentDef level: a node_role=planner def must
// carry a tools CSV that grants emit_findings, via exact match or glob.
func TestCreateAgentDef_PlannerToolsRequireEmitFindings(t *testing.T) {
	tests := []struct {
		name    string
		tools   string
		wantErr bool
	}{
		{name: "empty_tools_rejected", tools: "", wantErr: true},
		{name: "glob_all_accepted", tools: "*", wantErr: false},
		{name: "exact_emit_findings_accepted", tools: "emit_findings,agent_finished", wantErr: false},
		{name: "prefix_glob_accepted", tools: "emit_*", wantErr: false},
		{name: "missing_emit_findings_rejected", tools: "findings_add,agent_finished", wantErr: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, _, wfID := setupAgentDefAPIModeEnv(t)

			_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:       "planner-" + tc.name,
				Prompt:   "plan the work",
				NodeRole: "planner",
				Tools:    tc.tools,
			})
			if tc.wantErr && err == nil {
				t.Fatalf("CreateAgentDef with tools=%q: expected error, got nil", tc.tools)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("CreateAgentDef with tools=%q: unexpected error: %v", tc.tools, err)
			}
		})
	}
}

// TestUpdateAgentDef_PlannerToolsStrippingRejected creates a valid planner def
// (tools grants emit_findings), then PATCHes only Tools to a value that no
// longer grants it (NodeRole is not touched by the request). UpdateAgentDef
// must re-validate via revalidateConsultantAndNodeRole (which re-checks
// whenever Consultant/ExecutionMode/NodeRole/Tools is non-nil) and reject.
func TestUpdateAgentDef_PlannerToolsStrippingRejected(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:       "planner-strip",
		Prompt:   "plan the work",
		NodeRole: "planner",
		Tools:    "emit_findings,agent_finished",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newTools := "agent_finished"
	err := svc.UpdateAgentDef("proj1", wfID, "planner-strip", &types.AgentDefUpdateRequest{
		Tools: &newTools,
	})
	if err == nil {
		t.Fatal("expected UpdateAgentDef to reject stripping emit_findings from a planner def's tools, got nil error")
	}
}

// TestUpdateAgentDef_PlannerToolsOmittedFieldIsNoOp asserts
// revalidateConsultantAndNodeRole only re-checks the invariant when one of
// Consultant/ExecutionMode/NodeRole/Tools is present on the request — a PATCH
// touching only an unrelated field (Prompt) must succeed as a no-op even if
// the stored tools have (by some other means) come to lack emit_findings.
func TestUpdateAgentDef_PlannerToolsOmittedFieldIsNoOp(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:       "planner-noop",
		Prompt:   "plan the work",
		NodeRole: "planner",
		Tools:    "emit_findings",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Simulate a stale row whose tools no longer grant emit_findings, without
	// going through UpdateAgentDef (which would re-validate). svc.pool is
	// reachable directly since this test lives in package service.
	if _, err := svc.pool.Exec(
		`UPDATE agent_definitions SET tools = 'agent_finished' WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		"proj1", wfID, "planner-noop",
	); err != nil {
		t.Fatalf("simulate stale row: %v", err)
	}

	newPrompt := "revised prompt"
	if err := svc.UpdateAgentDef("proj1", wfID, "planner-noop", &types.AgentDefUpdateRequest{
		Prompt: &newPrompt,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(prompt only) on a stale planner row: expected no-op success, got: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "planner-noop")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.Prompt != newPrompt {
		t.Errorf("Prompt = %q, want %q", def.Prompt, newPrompt)
	}
}

// TestSystemAgentDef_PlannerToolsStrippingRejected is the
// SystemAgentDefinitionService (system_agent_definitions) analog: Update must
// call revalidatePlannerTools and reject stripping emit_findings from a
// planner system agent def's tools. Reuses the 'planner-system' row seeded by
// migration 000158 (role=planner, execution_mode=cli_interactive) rather than
// creating a new one — system_agent_definitions has a UNIQUE(role,
// execution_mode) index (000063_system_agent_api_mode.up.sql), so a second
// role=planner/cli_interactive row cannot be created alongside the seed.
func TestSystemAgentDef_PlannerToolsStrippingRejected(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	newTools := "agent_finished"
	err := svc.Update("planner-system", &types.SystemAgentDefUpdateRequest{
		Tools: &newTools,
	})
	if err == nil {
		t.Fatal("expected Update to reject stripping emit_findings from a planner system agent def, got nil error")
	}
}
