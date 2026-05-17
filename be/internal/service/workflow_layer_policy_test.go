package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

func setupLayerPolicySvc(t *testing.T) (*WorkflowLayerPolicyService, *AgentDefinitionService, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "lp_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	clk := clock.Real()
	now := clk.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('proj1', 'P', '/tmp', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	wfSvc := NewWorkflowService(pool, clk)
	if _, err := wfSvc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{ID: "wf1"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	cliModelSvc := NewCLIModelService(pool, clk)
	agentSvc := NewAgentDefinitionService(pool, clk, cliModelSvc, nil)
	lpSvc := NewWorkflowLayerPolicyService(pool, clk)
	return lpSvc, agentSvc, "proj1", "wf1"
}

func TestWorkflowLayerPolicyService_GetLayerPolicies_Empty(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	policies, err := svc.GetLayerPolicies(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPolicies() error: %v", err)
	}
	if len(policies) != 0 {
		t.Errorf("GetLayerPolicies() = %v, want empty map", policies)
	}
}

func TestWorkflowLayerPolicyService_SetLayerPolicy_QuorumSuccess(t *testing.T) {
	t.Parallel()
	svc, agentSvc, projectID, workflowID := setupLayerPolicySvc(t)

	for _, id := range []string{"agent-a", "agent-b"} {
		if _, err := agentSvc.CreateAgentDef(projectID, workflowID, &types.AgentDefCreateRequest{
			ID: id, Prompt: "p", Layer: 0,
		}); err != nil {
			t.Fatalf("CreateAgentDef(%q): %v", id, err)
		}
	}

	if err := svc.SetLayerPolicy(projectID, workflowID, 0, "quorum:2"); err != nil {
		t.Fatalf("SetLayerPolicy(\"quorum:2\") unexpected error: %v", err)
	}

	policies, err := svc.GetLayerPolicies(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPolicies() error: %v", err)
	}
	if got := policies[0]; got != "quorum:2" {
		t.Errorf("policies[0] = %q, want \"quorum:2\"", got)
	}
}

func TestWorkflowLayerPolicyService_SetLayerPolicy_QuorumExceedsCount(t *testing.T) {
	t.Parallel()
	svc, agentSvc, projectID, workflowID := setupLayerPolicySvc(t)

	for _, id := range []string{"agent-a", "agent-b"} {
		if _, err := agentSvc.CreateAgentDef(projectID, workflowID, &types.AgentDefCreateRequest{
			ID: id, Prompt: "p", Layer: 0,
		}); err != nil {
			t.Fatalf("CreateAgentDef(%q): %v", id, err)
		}
	}

	if err := svc.SetLayerPolicy(projectID, workflowID, 0, "quorum:3"); err == nil {
		t.Error("SetLayerPolicy(\"quorum:3\") expected error (quorum > agent count), got nil")
	}
}

func TestWorkflowLayerPolicyService_SetLayerPolicy_AnyWithZeroAgents(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	if err := svc.SetLayerPolicy(projectID, workflowID, 5, "any"); err != nil {
		t.Errorf("SetLayerPolicy(\"any\", layer=5) unexpected error: %v", err)
	}
}

func TestWorkflowLayerPolicyService_SetLayerPolicy_AllWithZeroAgents(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	if err := svc.SetLayerPolicy(projectID, workflowID, 0, "all"); err != nil {
		t.Errorf("SetLayerPolicy(\"all\") unexpected error: %v", err)
	}
}

func TestWorkflowLayerPolicyService_DeleteLayerPolicy(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	if err := svc.SetLayerPolicy(projectID, workflowID, 0, "any"); err != nil {
		t.Fatalf("SetLayerPolicy: %v", err)
	}

	if err := svc.DeleteLayerPolicy(projectID, workflowID, 0); err != nil {
		t.Fatalf("DeleteLayerPolicy: %v", err)
	}

	policies, err := svc.GetLayerPolicies(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPolicies() after delete: %v", err)
	}
	if len(policies) != 0 {
		t.Errorf("policies after delete = %v, want empty", policies)
	}
}

func TestWorkflowLayerPolicyService_MultipleLayerPolicies(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	if err := svc.SetLayerPolicy(projectID, workflowID, 0, "any"); err != nil {
		t.Fatalf("SetLayerPolicy(layer=0): %v", err)
	}
	if err := svc.SetLayerPolicy(projectID, workflowID, 1, "all"); err != nil {
		t.Fatalf("SetLayerPolicy(layer=1): %v", err)
	}

	policies, err := svc.GetLayerPolicies(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPolicies(): %v", err)
	}
	if len(policies) != 2 {
		t.Fatalf("len(policies) = %d, want 2", len(policies))
	}
	if got := policies[0]; got != "any" {
		t.Errorf("policies[0] = %q, want \"any\"", got)
	}
	if got := policies[1]; got != "all" {
		t.Errorf("policies[1] = %q, want \"all\"", got)
	}
}

func TestWorkflowLayerPolicyService_SetLayerPolicy_Upsert(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	if err := svc.SetLayerPolicy(projectID, workflowID, 0, "any"); err != nil {
		t.Fatalf("SetLayerPolicy (initial): %v", err)
	}
	if err := svc.SetLayerPolicy(projectID, workflowID, 0, "all"); err != nil {
		t.Fatalf("SetLayerPolicy (update): %v", err)
	}

	policies, err := svc.GetLayerPolicies(projectID, workflowID)
	if err != nil {
		t.Fatalf("GetLayerPolicies(): %v", err)
	}
	if got := policies[0]; got != "all" {
		t.Errorf("policies[0] after upsert = %q, want \"all\"", got)
	}
}

func TestWorkflowLayerPolicyService_SetLayerPolicy_InvalidPolicy(t *testing.T) {
	t.Parallel()
	svc, _, projectID, workflowID := setupLayerPolicySvc(t)

	cases := []struct {
		policy string
	}{
		{"quorum:0"},
		{"percent:0"},
		{"percent:101"},
		{"garbage"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.policy, func(t *testing.T) {
			t.Parallel()
			err := svc.SetLayerPolicy(projectID, workflowID, 0, tc.policy)
			if err == nil {
				t.Errorf("SetLayerPolicy(%q) expected error, got nil", tc.policy)
			}
		})
	}
}
