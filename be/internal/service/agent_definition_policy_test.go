package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupPolicyTestEnv creates an isolated DB with a project, workflow, and two agents
// in layer 1, and a quorum:2 policy on that layer.
func setupPolicyTestEnv(t *testing.T) (*db.Pool, *AgentDefinitionService, *WorkflowLayerPolicyService) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "policy_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		"proj", now, now)

	clk := clock.Real()
	wfSvc := NewWorkflowService(pool, clk)
	wfSvc.CreateWorkflowDef("proj", &types.WorkflowDefCreateRequest{ID: "wf"})

	cliModelSvc := NewCLIModelService(pool, clk)
	agentSvc := NewAgentDefinitionService(pool, clk, cliModelSvc, nil, false)
	if _, err := agentSvc.CreateAgentDef("proj", "wf", &types.AgentDefCreateRequest{ID: "agent-a", Prompt: "do stuff", Layer: 1}); err != nil {
		t.Fatalf("create agent-a: %v", err)
	}
	if _, err := agentSvc.CreateAgentDef("proj", "wf", &types.AgentDefCreateRequest{ID: "agent-b", Prompt: "do stuff", Layer: 1}); err != nil {
		t.Fatalf("create agent-b: %v", err)
	}

	// Set quorum:2 on layer 1
	polSvc := NewWorkflowLayerPolicyService(pool, clk)
	if err := polSvc.SetLayerPolicy("proj", "wf", 1, "quorum:2"); err != nil {
		t.Fatalf("SetLayerPolicy: %v", err)
	}

	return pool, agentSvc, polSvc
}

// TestDeleteAgentDef_PolicyInvalidated verifies that deleting an agent that would
// leave fewer agents than the quorum is rejected with a clear error.
func TestDeleteAgentDef_PolicyInvalidated(t *testing.T) {
	t.Parallel()
	_, agentSvc, _ := setupPolicyTestEnv(t)

	// Delete agent-a from layer 1 — only agent-b remains but quorum:2 requires 2
	err := agentSvc.DeleteAgentDef("proj", "wf", "agent-a")
	if err == nil {
		t.Fatal("expected error when deleting agent would violate quorum:2, got nil")
	}
	if !strings.Contains(err.Error(), "quorum") {
		t.Errorf("expected quorum mention in error, got: %s", err.Error())
	}
}

// TestDeleteAgentDef_NoPolicy_Allowed verifies that without a policy, deletion is allowed.
func TestDeleteAgentDef_NoPolicy_Allowed(t *testing.T) {
	t.Parallel()
	_, agentSvc, polSvc := setupPolicyTestEnv(t)

	// Remove the policy first
	polSvc.DeleteLayerPolicy("proj", "wf", 1)

	// Now deletion should succeed
	if err := agentSvc.DeleteAgentDef("proj", "wf", "agent-a"); err != nil {
		t.Fatalf("expected deletion to succeed without policy, got: %v", err)
	}
}

// TestUpdateAgentDef_LayerMove_InvalidatesPolicy verifies that moving an agent to a
// different layer is rejected when the old layer's quorum would be violated.
func TestUpdateAgentDef_LayerMove_InvalidatesPolicy(t *testing.T) {
	t.Parallel()
	_, agentSvc, _ := setupPolicyTestEnv(t)

	// Move agent-a from layer 1 to layer 0 — layer 1 would only have agent-b (count=1 < quorum:2)
	layer0 := 0
	err := agentSvc.UpdateAgentDef("proj", "wf", "agent-a", &types.AgentDefUpdateRequest{
		Layer: &layer0,
	})
	if err == nil {
		t.Fatal("expected error when layer move would violate quorum:2, got nil")
	}
	if !strings.Contains(err.Error(), "quorum") {
		t.Errorf("expected quorum mention in error, got: %s", err.Error())
	}
}

// TestUpdateAgentDef_LayerMove_SameLayer_Allowed verifies that updating an agent's other
// fields (no layer change) is always allowed even when a policy exists.
func TestUpdateAgentDef_LayerMove_SameLayer_Allowed(t *testing.T) {
	t.Parallel()
	_, agentSvc, _ := setupPolicyTestEnv(t)

	timeout := 600
	if err := agentSvc.UpdateAgentDef("proj", "wf", "agent-a", &types.AgentDefUpdateRequest{
		Timeout: &timeout,
	}); err != nil {
		t.Fatalf("expected success updating non-layer fields, got: %v", err)
	}
}
