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

// TestAgentDefFanInValidation tests that service enforces fan-in rules via agent definitions
func TestAgentDefFanInValidation(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("valid fan-in: 2 agents in layer 0, 1 in layer 1", func(t *testing.T) {
		_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
			ID: "fanin-valid",
		})
		if err != nil {
			t.Fatalf("failed to create workflow: %v", err)
		}
		agentDefSvc := env.getAgentDefService(t)
		for _, ad := range []types.AgentDefCreateRequest{
			{ID: "analyzer-a", Prompt: "a", Layer: 0},
			{ID: "analyzer-b", Prompt: "b", Layer: 0},
			{ID: "builder", Prompt: "c", Layer: 1},
		} {
			if _, err := agentDefSvc.CreateAgentDef(env.ProjectID, "fanin-valid", &ad); err != nil {
				t.Fatalf("failed to create agent def %s: %v", ad.ID, err)
			}
		}
	})

	t.Run("invalid fan-in: 2 agents in layer 0, 2 in layer 1", func(t *testing.T) {
		_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
			ID: "fanin-invalid",
		})
		if err != nil {
			t.Fatalf("failed to create workflow: %v", err)
		}
		agentDefSvc := env.getAgentDefService(t)
		// First create two agents in layer 0
		for _, ad := range []types.AgentDefCreateRequest{
			{ID: "analyzer-a", Prompt: "a", Layer: 0},
			{ID: "analyzer-b", Prompt: "b", Layer: 0},
		} {
			if _, err := agentDefSvc.CreateAgentDef(env.ProjectID, "fanin-invalid", &ad); err != nil {
				t.Fatalf("failed to create agent def %s: %v", ad.ID, err)
			}
		}
		// Adding a second agent in layer 1 should fail (first one should succeed)
		if _, err := agentDefSvc.CreateAgentDef(env.ProjectID, "fanin-invalid", &types.AgentDefCreateRequest{
			ID: "builder-a", Prompt: "c", Layer: 1,
		}); err != nil {
			t.Fatalf("first agent in layer 1 should succeed: %v", err)
		}
		_, err = agentDefSvc.CreateAgentDef(env.ProjectID, "fanin-invalid", &types.AgentDefCreateRequest{
			ID: "builder-b", Prompt: "d", Layer: 1,
		})
		if err == nil {
			t.Error("expected fan-in error, got nil")
		} else if !strings.Contains(err.Error(), "fan-in violation") {
			t.Errorf("expected error to contain 'fan-in violation', got: %s", err.Error())
		}
	})
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

			for _, phase := range retrieved.Phases {
				if phase.Layer < 0 {
					t.Errorf("phase has invalid layer value: %+v", phase)
				}
			}
		})
	}
}
