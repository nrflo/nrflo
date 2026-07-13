package service

import (
	"testing"

	"be/internal/types"
)

// TestSetLayerPolicy_QuorumDenominatorExcludesNonStaticRoles verifies that
// SetLayerPolicy's quorum-vs-count validation rides on
// AgentDefinitionRepo.ListExecutable, which excludes planner/fanout_template
// defs the same way it already excludes consultants. A layer with 2 static
// agents plus 1 planner-role agent must be treated as a 2-agent layer, not 3.
func TestSetLayerPolicy_QuorumDenominatorExcludesNonStaticRoles(t *testing.T) {
	t.Parallel()
	svc, agentSvc, projectID, workflowID := setupLayerPolicySvc(t)

	for _, id := range []string{"agent-a", "agent-b"} {
		if _, err := agentSvc.CreateAgentDef(projectID, workflowID, &types.AgentDefCreateRequest{
			ID: id, Prompt: "p", Layer: 0,
		}); err != nil {
			t.Fatalf("CreateAgentDef(%q): %v", id, err)
		}
	}
	if _, err := agentSvc.CreateAgentDef(projectID, workflowID, &types.AgentDefCreateRequest{
		ID: "planner-agent", Prompt: "plan", Layer: 0, NodeRole: "planner", Tools: "emit_findings",
	}); err != nil {
		t.Fatalf("CreateAgentDef(planner-agent): %v", err)
	}

	// quorum:3 would be valid if the planner rode along in the denominator —
	// it must be rejected because ListExecutable only counts the 2 static defs.
	if err := svc.SetLayerPolicy(projectID, workflowID, 0, "quorum:3"); err == nil {
		t.Error("SetLayerPolicy(\"quorum:3\") expected error (planner def must not count), got nil")
	}

	// quorum:2 is exactly the static agent count and must succeed.
	if err := svc.SetLayerPolicy(projectID, workflowID, 0, "quorum:2"); err != nil {
		t.Errorf("SetLayerPolicy(\"quorum:2\") unexpected error: %v", err)
	}
}

// TestSetLayerPolicy_QuorumDenominatorExcludesFanoutTemplate mirrors the
// planner case for node_role=fanout_template.
func TestSetLayerPolicy_QuorumDenominatorExcludesFanoutTemplate(t *testing.T) {
	t.Parallel()
	svc, agentSvc, projectID, workflowID := setupLayerPolicySvc(t)

	if _, err := agentSvc.CreateAgentDef(projectID, workflowID, &types.AgentDefCreateRequest{
		ID: "solo-agent", Prompt: "p", Layer: 1,
	}); err != nil {
		t.Fatalf("CreateAgentDef(solo-agent): %v", err)
	}
	if _, err := agentSvc.CreateAgentDef(projectID, workflowID, &types.AgentDefCreateRequest{
		ID: "fanout-template", Prompt: "template", Layer: 1, NodeRole: "fanout_template", Description: "A template.",
	}); err != nil {
		t.Fatalf("CreateAgentDef(fanout-template): %v", err)
	}

	if err := svc.SetLayerPolicy(projectID, workflowID, 1, "quorum:2"); err == nil {
		t.Error("SetLayerPolicy(\"quorum:2\") expected error (fanout_template def must not count), got nil")
	}
	if err := svc.SetLayerPolicy(projectID, workflowID, 1, "quorum:1"); err != nil {
		t.Errorf("SetLayerPolicy(\"quorum:1\") unexpected error: %v", err)
	}
}
