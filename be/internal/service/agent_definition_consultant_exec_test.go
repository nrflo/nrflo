package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// setupConsultantExecEnv creates an isolated DB with api_mode_enabled=true and
// returns all the services needed for consultant-execution tests.
func setupConsultantExecEnv(t *testing.T) (pool *db.Pool, agentSvc *AgentDefinitionService, wfSvc *WorkflowService, polSvc *WorkflowLayerPolicyService, wfID string) {
	t.Helper()
	pool, _, wfID = setupAgentDefTestEnv(t, nil)
	clk := clock.Real()
	modelSvc := NewModelService(pool, clk)
	agentSvc = NewAgentDefinitionService(pool, clk, modelSvc, nil)
	wfSvc = NewWorkflowService(pool, clk)
	polSvc = NewWorkflowLayerPolicyService(pool, clk)
	settingsSvc := NewGlobalSettingsService(pool, clk)
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("set api_mode_enabled: %v", err)
	}
	return
}

// insertSecondWorkflow inserts an additional workflow row for multi-workflow tests.
func insertSecondWorkflow(t *testing.T, pool *db.Pool, projectID, workflowID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'ticket', ?, ?)`,
		workflowID, projectID, now, now); err != nil {
		t.Fatalf("insert workflow %s: %v", workflowID, err)
	}
}

// TestWorkflowDef_OmitsConsultantFromPhases verifies that a consultant agent
// definition does not appear in the Phases slice returned by either GetWorkflowDef
// or ListWorkflowDefs (both share the same read path), while a second consultant-free
// workflow is unaffected.
func TestWorkflowDef_OmitsConsultantFromPhases(t *testing.T) {
	t.Parallel()
	pool, agentSvc, wfSvc, _, wfID := setupConsultantExecEnv(t)

	// Add a second workflow so we can confirm isolation.
	insertSecondWorkflow(t, pool, "proj1", "wf2")

	// wf1: one real + one consultant on the same layer.
	if _, err := agentSvc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "impl", Prompt: "implement", Layer: 0,
	}); err != nil {
		t.Fatalf("create impl: %v", err)
	}
	if _, err := agentSvc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "advisor", Prompt: "advise", Layer: 0,
		ExecutionMode: "api", Consultant: true,
	}); err != nil {
		t.Fatalf("create advisor: %v", err)
	}

	// wf2: one real agent only.
	if _, err := agentSvc.CreateAgentDef("proj1", "wf2", &types.AgentDefCreateRequest{
		ID: "worker", Prompt: "work", Layer: 0,
	}); err != nil {
		t.Fatalf("create worker: %v", err)
	}

	// GetWorkflowDef path.
	wf, err := wfSvc.GetWorkflowDef("proj1", wfID)
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if len(wf.Phases) != 1 {
		t.Fatalf("GetWorkflowDef Phases count = %d, want 1 (consultant excluded)", len(wf.Phases))
	}
	if wf.Phases[0].Agent != "impl" {
		t.Errorf("GetWorkflowDef Phases[0].Agent = %q, want impl", wf.Phases[0].Agent)
	}

	// ListWorkflowDefs path.
	defs, err := wfSvc.ListWorkflowDefs("proj1")
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}
	wf1, ok := defs[wfID]
	if !ok {
		t.Fatalf("%s not found in ListWorkflowDefs result", wfID)
	}
	if len(wf1.Phases) != 1 {
		t.Errorf("%s phases = %d, want 1 (consultant excluded)", wfID, len(wf1.Phases))
	}
	if len(wf1.Phases) > 0 && wf1.Phases[0].Agent != "impl" {
		t.Errorf("%s Phases[0] = %q, want impl", wfID, wf1.Phases[0].Agent)
	}
	wf2, ok := defs["wf2"]
	if !ok {
		t.Fatal("wf2 not found in ListWorkflowDefs result")
	}
	if len(wf2.Phases) != 1 {
		t.Errorf("wf2 phases = %d, want 1", len(wf2.Phases))
	}
}

// TestDeleteAgentDef_ConsultantNotCountedInQuorumDenominator verifies that when a
// layer has 2 real agents + 1 consultant + quorum:2 policy, deleting one real agent
// is rejected (leaving 1 real < quorum:2) and the consultant does not satisfy quorum.
func TestDeleteAgentDef_ConsultantNotCountedInQuorumDenominator(t *testing.T) {
	t.Parallel()
	_, agentSvc, _, polSvc, wfID := setupConsultantExecEnv(t)

	// Create 2 real agents and 1 consultant on layer 1.
	for _, id := range []string{"real-a", "real-b"} {
		if _, err := agentSvc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
			ID: id, Prompt: "do stuff", Layer: 1,
		}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	if _, err := agentSvc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "cons-l1", Prompt: "advise", Layer: 1,
		ExecutionMode: "api", Consultant: true,
	}); err != nil {
		t.Fatalf("create consultant: %v", err)
	}

	// Set quorum:2 on layer 1 — requires 2 real agents.
	if err := polSvc.SetLayerPolicy("proj1", wfID, 1, "quorum:2"); err != nil {
		t.Fatalf("SetLayerPolicy: %v", err)
	}

	// Deleting one real agent leaves 1 real + 1 consultant.
	// Consultant must NOT count — 1 < 2 so delete must be rejected.
	err := agentSvc.DeleteAgentDef("proj1", wfID, "real-a")
	if err == nil {
		t.Fatal("DeleteAgentDef: expected error (would violate quorum:2), got nil")
	}
	if !strings.Contains(err.Error(), "quorum") {
		t.Errorf("error %q does not mention quorum", err.Error())
	}
}

// TestSetLayerPolicy_QuorumValidatedAgainstRealAgentsOnly verifies that quorum
// validation in SetLayerPolicy counts only real (non-consultant) agents, not
// consultant agents in the same layer.
func TestSetLayerPolicy_QuorumValidatedAgainstRealAgentsOnly(t *testing.T) {
	t.Parallel()
	_, agentSvc, _, polSvc, wfID := setupConsultantExecEnv(t)

	// Layer 0: 2 real agents + 1 consultant.
	for _, id := range []string{"real-x", "real-y"} {
		if _, err := agentSvc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
			ID: id, Prompt: "do stuff", Layer: 0,
		}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	if _, err := agentSvc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "cons-l0", Prompt: "advise", Layer: 0,
		ExecutionMode: "api", Consultant: true,
	}); err != nil {
		t.Fatalf("create consultant: %v", err)
	}

	// quorum:3 should be rejected (3 > 2 real agents).
	if err := polSvc.SetLayerPolicy("proj1", wfID, 0, "quorum:3"); err == nil {
		t.Error("SetLayerPolicy quorum:3 with 2 real agents: expected error, got nil")
	}

	// quorum:2 should be accepted (2 == 2 real agents).
	if err := polSvc.SetLayerPolicy("proj1", wfID, 0, "quorum:2"); err != nil {
		t.Errorf("SetLayerPolicy quorum:2 with 2 real agents: %v", err)
	}
}

// TestBuildSpawnerConfig_ConsultantExcludedViaListExecutable verifies end-to-end that
// feeding ListExecutable output into BuildSpawnerConfig produces no phase for the
// consultant. This mirrors what the orchestrator does at workflow start.
func TestBuildSpawnerConfig_ConsultantExcludedViaListExecutable(t *testing.T) {
	t.Parallel()
	pool, agentSvc, _, _, wfID := setupConsultantExecEnv(t)

	if _, err := agentSvc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "implementor", Prompt: "implement", Layer: 0,
	}); err != nil {
		t.Fatalf("create implementor: %v", err)
	}
	if _, err := agentSvc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "reviewer", Prompt: "advise", Layer: 0,
		ExecutionMode: "api", Consultant: true,
	}); err != nil {
		t.Fatalf("create reviewer consultant: %v", err)
	}

	// Simulate orchestrator: use ListExecutable to get execution defs.
	agentDefRepo := repo.NewAgentDefinitionRepo(pool, clock.Real())
	execDefs, err := agentDefRepo.ListExecutable("proj1", wfID)
	if err != nil {
		t.Fatalf("ListExecutable: %v", err)
	}

	wf := &model.Workflow{ID: wfID, ProjectID: "proj1"}
	workflows, _ := BuildSpawnerConfig([]*model.Workflow{wf}, execDefs)

	spawnerWF, ok := workflows[wfID]
	if !ok {
		t.Fatalf("workflow %s not found in BuildSpawnerConfig output", wfID)
	}
	if len(spawnerWF.Phases) != 1 {
		t.Fatalf("Phases count = %d, want 1 (consultant excluded)", len(spawnerWF.Phases))
	}
	if spawnerWF.Phases[0].Agent != "implementor" {
		t.Errorf("Phases[0].Agent = %q, want implementor", spawnerWF.Phases[0].Agent)
	}
}
