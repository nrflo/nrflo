package integration

import (
	"strings"
	"testing"

	"be/internal/types"
)

// TestAgentDefRejectNegativeLayer tests that service rejects agent definitions with negative layer
func TestAgentDefRejectNegativeLayer(t *testing.T) {
	env := NewTestEnv(t)

	// Create a workflow first
	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "layer-neg-test",
		Description: "Test workflow",
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	agentDefSvc := env.getAgentDefService(t)
	_, err = agentDefSvc.CreateAgentDef(env.ProjectID, "layer-neg-test", &types.AgentDefCreateRequest{
		ID:     "analyzer",
		Prompt: "test",
		Layer:  -1,
	})

	if err == nil {
		t.Error("expected error for negative layer, got nil")
	} else if !strings.Contains(err.Error(), "layer must be >= 0") {
		t.Errorf("expected error to contain 'layer must be >= 0', got: %s", err.Error())
	}
}

// TestAgentDefValidLayerConfig tests that valid layer configurations are accepted via agent definitions
func TestAgentDefValidLayerConfig(t *testing.T) {
	env := NewTestEnv(t)

	tests := []struct {
		name   string
		agents []types.AgentDefCreateRequest
	}{
		{
			name: "simple sequential layers",
			agents: []types.AgentDefCreateRequest{
				{ID: "setup", Prompt: "s", Layer: 0},
				{ID: "impl", Prompt: "i", Layer: 1},
				{ID: "verify", Prompt: "v", Layer: 2},
			},
		},
		{
			name: "parallel agents in first layer only",
			agents: []types.AgentDefCreateRequest{
				{ID: "setup-a", Prompt: "a", Layer: 0},
				{ID: "setup-b", Prompt: "b", Layer: 0},
				{ID: "impl", Prompt: "i", Layer: 1},
			},
		},
		{
			name: "non-contiguous layer numbers",
			agents: []types.AgentDefCreateRequest{
				{ID: "setup", Prompt: "s", Layer: 0},
				{ID: "impl", Prompt: "i", Layer: 5},
				{ID: "verify", Prompt: "v", Layer: 10},
			},
		},
		{
			name: "single agent workflow",
			agents: []types.AgentDefCreateRequest{
				{ID: "hotfix", Prompt: "h", Layer: 0},
			},
		},
		{
			name: "parallel to parallel [A,B]->[C,D]",
			agents: []types.AgentDefCreateRequest{
				{ID: "setup-a", Prompt: "a", Layer: 0},
				{ID: "setup-b", Prompt: "b", Layer: 0},
				{ID: "verify-a", Prompt: "c", Layer: 1},
				{ID: "verify-b", Prompt: "d", Layer: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wfID := "valid-" + strings.ReplaceAll(tt.name, " ", "-")
			_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
				ID: wfID,
			})
			if err != nil {
				t.Fatalf("failed to create workflow: %v", err)
			}

			agentDefSvc := env.getAgentDefService(t)
			for _, ad := range tt.agents {
				if _, err := agentDefSvc.CreateAgentDef(env.ProjectID, wfID, &ad); err != nil {
					t.Errorf("expected no error for valid config, got: %v", err)
				}
			}

			// Verify workflow can be retrieved with phases derived from agent defs
			retrieved, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, wfID)
			if err != nil {
				t.Errorf("failed to retrieve created workflow: %v", err)
			}

			if len(retrieved.Phases) != len(tt.agents) {
				t.Errorf("expected %d phases, got %d", len(tt.agents), len(retrieved.Phases))
			}

			for _, phase := range retrieved.Phases {
				if phase.Layer < 0 {
					t.Errorf("phase has invalid layer value: %+v", phase)
				}
			}
		})
	}
}

// TestAgentDefParallelToParallelAllowed explicitly validates that [A,B]->[C,D]
// topologies (parallel agents followed by parallel agents) are accepted and produce
// the correct phase count.
func TestAgentDefParallelToParallelAllowed(t *testing.T) {
	env := NewTestEnv(t)

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID: "p2p-wf",
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	agentDefSvc := env.getAgentDefService(t)
	agents := []types.AgentDefCreateRequest{
		{ID: "setup-a", Prompt: "a", Layer: 0},
		{ID: "setup-b", Prompt: "b", Layer: 0},
		{ID: "verify-a", Prompt: "c", Layer: 1},
		{ID: "verify-b", Prompt: "d", Layer: 1},
	}
	for _, a := range agents {
		if _, err := agentDefSvc.CreateAgentDef(env.ProjectID, "p2p-wf", &a); err != nil {
			t.Fatalf("parallel-to-parallel topology must be accepted, agent %s: %v", a.ID, err)
		}
	}

	wf, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, "p2p-wf")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if len(wf.Phases) != 4 {
		t.Errorf("expected 4 phases, got %d", len(wf.Phases))
	}
	// Verify L0 agents come before L1 agents
	for i := 0; i < 2; i++ {
		if wf.Phases[i].Layer != 0 {
			t.Errorf("phases[%d].Layer = %d, want 0", i, wf.Phases[i].Layer)
		}
	}
	for i := 2; i < 4; i++ {
		if wf.Phases[i].Layer != 1 {
			t.Errorf("phases[%d].Layer = %d, want 1", i, wf.Phases[i].Layer)
		}
	}
}
