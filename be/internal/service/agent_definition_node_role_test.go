package service

import (
	"testing"

	"be/internal/types"
)

// TestCreateAgentDef_NodeRole_AcceptsValidValues verifies static, planner, and
// fanout_template are all accepted node_role values on create.
func TestCreateAgentDef_NodeRole_AcceptsValidValues(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	for _, role := range []string{"static", "planner", "fanout_template"} {
		role := role
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			req := &types.AgentDefCreateRequest{
				ID:       "agent-" + role,
				Prompt:   "do stuff",
				NodeRole: role,
			}
			if role == "planner" {
				req.Tools = "emit_findings"
			}
			def, err := svc.CreateAgentDef("proj1", wfID, req)
			if err != nil {
				t.Fatalf("CreateAgentDef(node_role=%s): %v", role, err)
			}
			if def.NodeRole != role {
				t.Errorf("NodeRole = %q, want %q", def.NodeRole, role)
			}
		})
	}
}

// TestCreateAgentDef_NodeRole_EmptyDefaultsToStatic verifies an omitted
// node_role defaults to "static".
func TestCreateAgentDef_NodeRole_EmptyDefaultsToStatic(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-default-role",
		Prompt: "do stuff",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.NodeRole != "static" {
		t.Errorf("NodeRole = %q, want static (default)", def.NodeRole)
	}
}

// TestCreateAgentDef_NodeRole_RejectsInvalidValue verifies unrecognized
// node_role strings are rejected.
func TestCreateAgentDef_NodeRole_RejectsInvalidValue(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:       "agent-bad-role",
		Prompt:   "do stuff",
		NodeRole: "bogus",
	})
	if err == nil {
		t.Fatal("CreateAgentDef(node_role=bogus): expected error, got nil")
	}
}

// TestCreateAgentDef_NodeRole_ConsultantRequiresStatic verifies that
// consultant=true combined with a non-static node_role is rejected.
func TestCreateAgentDef_NodeRole_ConsultantRequiresStatic(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupConsultantEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "consultant-planner",
		Prompt:        "advise",
		ExecutionMode: "api",
		Consultant:    true,
		NodeRole:      "planner",
	})
	if err == nil {
		t.Fatal("CreateAgentDef(consultant=true, node_role=planner): expected error, got nil")
	}
}

// TestCreateAgentDef_NodeRole_ConsultantStaticSucceeds verifies consultant=true
// with the default/explicit static node_role is accepted (existing invariant
// unaffected by the new field).
func TestCreateAgentDef_NodeRole_ConsultantStaticSucceeds(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupConsultantEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "consultant-static",
		Prompt:        "advise",
		ExecutionMode: "api",
		Consultant:    true,
		NodeRole:      "static",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(consultant=true, node_role=static): %v", err)
	}
	if def.NodeRole != "static" {
		t.Errorf("NodeRole = %q, want static", def.NodeRole)
	}
}

// TestUpdateAgentDef_NodeRole_RoundTrips verifies an explicit PATCH changes
// the stored node_role.
func TestUpdateAgentDef_NodeRole_RoundTrips(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-patch-role",
		Prompt: "do stuff",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newRole := "fanout_template"
	if err := svc.UpdateAgentDef("proj1", wfID, "agent-patch-role", &types.AgentDefUpdateRequest{
		NodeRole: &newRole,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(node_role=fanout_template): %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "agent-patch-role")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.NodeRole != "fanout_template" {
		t.Errorf("NodeRole after update = %q, want fanout_template", def.NodeRole)
	}
}

// TestUpdateAgentDef_NodeRole_OmittedFieldIsNoOp verifies a PATCH that omits
// node_role (as the UI's explicit-payload PATCH does) leaves the stored value
// unchanged — this is the guard the validation-shape mirrors from consultant.
func TestUpdateAgentDef_NodeRole_OmittedFieldIsNoOp(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:       "agent-omit-role",
		Prompt:   "do stuff",
		NodeRole: "planner",
		Tools:    "emit_findings",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newPrompt := "revised prompt"
	if err := svc.UpdateAgentDef("proj1", wfID, "agent-omit-role", &types.AgentDefUpdateRequest{
		Prompt: &newPrompt,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(prompt only): %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "agent-omit-role")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.NodeRole != "planner" {
		t.Errorf("NodeRole after unrelated update = %q, want unchanged planner", def.NodeRole)
	}
}

// TestUpdateAgentDef_NodeRole_ConsultantInvariantReValidated verifies that
// setting consultant=true via PATCH on a def whose stored node_role is
// non-static is rejected, even though the request itself does not touch
// node_role — mirroring the execution_mode re-validation-on-update pattern.
func TestUpdateAgentDef_NodeRole_ConsultantInvariantReValidated(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupConsultantEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "planner-to-consultant",
		Prompt:        "plan",
		ExecutionMode: "api",
		NodeRole:      "planner",
		Tools:         "emit_findings",
	}); err != nil {
		t.Fatalf("create planner agent: %v", err)
	}

	consultant := true
	err := svc.UpdateAgentDef("proj1", wfID, "planner-to-consultant", &types.AgentDefUpdateRequest{
		Consultant: &consultant,
	})
	if err == nil {
		t.Fatal("UpdateAgentDef(consultant=true on node_role=planner): expected error, got nil")
	}
}
