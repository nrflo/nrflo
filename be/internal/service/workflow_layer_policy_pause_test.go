package service

import (
	"testing"

	"be/internal/types"
)

// TestSetLayerPauseAfter_PersistsToggle verifies SetLayerPauseAfter persists pause_after.
func TestSetLayerPauseAfter_PersistsToggle(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	if err := svc.SetLayerPauseAfter(projectID, workflowID, 0, true); err != nil {
		t.Fatalf("SetLayerPauseAfter(true): %v", err)
	}

	got, err := svc.GetLayerPauseAfter(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPauseAfter: %v", err)
	}
	if !got[0] {
		t.Errorf("layer 0 PauseAfter = false, want true")
	}
}

// TestSetLayerPauseAfter_Toggle_FalseToTrue verifies toggling from false to true.
func TestSetLayerPauseAfter_Toggle_FalseToTrue(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	if err := svc.SetLayerPauseAfter(projectID, workflowID, 1, false); err != nil {
		t.Fatalf("SetLayerPauseAfter(false): %v", err)
	}
	if err := svc.SetLayerPauseAfter(projectID, workflowID, 1, true); err != nil {
		t.Fatalf("SetLayerPauseAfter(true): %v", err)
	}

	got, err := svc.GetLayerPauseAfter(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPauseAfter: %v", err)
	}
	if !got[1] {
		t.Errorf("layer 1 PauseAfter = false, want true after toggle")
	}
}

// TestSetLayerPauseAfter_PreservesPassPolicy verifies SetLayerPauseAfter does not clobber pass_policy.
func TestSetLayerPauseAfter_PreservesPassPolicy(t *testing.T) {
	t.Parallel()
	svc, agentSvc, projectID, workflowID := setupLayerPolicySvc(t)

	// Seed two agents in layer 0 to allow quorum:2.
	for _, id := range []string{"ag-a", "ag-b"} {
		if _, err := agentSvc.CreateAgentDef(projectID, workflowID, &types.AgentDefCreateRequest{
			ID: id, Prompt: "p", Layer: 0,
		}); err != nil {
			t.Fatalf("CreateAgentDef(%q): %v", id, err)
		}
	}

	if err := svc.SetLayerPolicy(projectID, workflowID, 0, "quorum:2"); err != nil {
		t.Fatalf("SetLayerPolicy: %v", err)
	}

	// Now toggle pause_after — pass_policy must remain "quorum:2".
	if err := svc.SetLayerPauseAfter(projectID, workflowID, 0, true); err != nil {
		t.Fatalf("SetLayerPauseAfter(true): %v", err)
	}

	policies, err := svc.GetLayerPolicies(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPolicies: %v", err)
	}
	if got := policies[0]; got != "quorum:2" {
		t.Errorf("PassPolicy after SetLayerPauseAfter = %q, want \"quorum:2\"", got)
	}
	pauseMap, err := svc.GetLayerPauseAfter(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPauseAfter: %v", err)
	}
	if !pauseMap[0] {
		t.Errorf("PauseAfter after SetLayerPauseAfter = false, want true")
	}
}

// TestSetLayerPolicy_PreservesPauseAfter verifies SetLayerPolicy does not clobber pause_after.
func TestSetLayerPolicy_PreservesPauseAfter(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	if err := svc.SetLayerPauseAfter(projectID, workflowID, 2, true); err != nil {
		t.Fatalf("SetLayerPauseAfter(true): %v", err)
	}

	// Change pass_policy — pause_after must remain true.
	if err := svc.SetLayerPolicy(projectID, workflowID, 2, "all"); err != nil {
		t.Fatalf("SetLayerPolicy: %v", err)
	}

	pauseMap, err := svc.GetLayerPauseAfter(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPauseAfter: %v", err)
	}
	if !pauseMap[2] {
		t.Errorf("PauseAfter after SetLayerPolicy = false, want true (preserved)")
	}

	policies, err := svc.GetLayerPolicies(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPolicies: %v", err)
	}
	if got := policies[2]; got != "all" {
		t.Errorf("PassPolicy = %q, want \"all\"", got)
	}
}

// TestSetLayerPauseAfter_FreshLayer_DefaultsPassPolicyToAny verifies that SetLayerPauseAfter
// on a layer with no existing policy defaults pass_policy to "any".
func TestSetLayerPauseAfter_FreshLayer_DefaultsPassPolicyToAny(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	if err := svc.SetLayerPauseAfter(projectID, workflowID, 5, true); err != nil {
		t.Fatalf("SetLayerPauseAfter(fresh layer): %v", err)
	}

	policies, err := svc.GetLayerPolicies(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPolicies: %v", err)
	}
	if got := policies[5]; got != "any" {
		t.Errorf("default PassPolicy for fresh layer = %q, want \"any\"", got)
	}
}

// TestGetLayerPauseAfter_Empty verifies GetLayerPauseAfter returns empty map for fresh workflow.
func TestGetLayerPauseAfter_Empty(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	got, err := svc.GetLayerPauseAfter(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPauseAfter: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetLayerPauseAfter() = %v, want empty map", got)
	}
}

// TestGetLayerPauseAfter_MultiLayer verifies GetLayerPauseAfter for multiple layers.
func TestGetLayerPauseAfter_MultiLayer(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	if err := svc.SetLayerPauseAfter(projectID, workflowID, 0, true); err != nil {
		t.Fatalf("SetLayerPauseAfter(layer=0, true): %v", err)
	}
	if err := svc.SetLayerPauseAfter(projectID, workflowID, 1, false); err != nil {
		t.Fatalf("SetLayerPauseAfter(layer=1, false): %v", err)
	}
	if err := svc.SetLayerPauseAfter(projectID, workflowID, 2, true); err != nil {
		t.Fatalf("SetLayerPauseAfter(layer=2, true): %v", err)
	}

	got, err := svc.GetLayerPauseAfter(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPauseAfter: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if !got[0] {
		t.Errorf("layer 0 = false, want true")
	}
	if got[1] {
		t.Errorf("layer 1 = true, want false")
	}
	if !got[2] {
		t.Errorf("layer 2 = false, want true")
	}
}
