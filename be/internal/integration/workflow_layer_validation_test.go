package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/types"
)

// TestWorkflowRejectParallelField tests that service rejects workflow definitions with parallel field
func TestWorkflowRejectParallelField(t *testing.T) {
	env := NewTestEnv(t)

	tests := []struct {
		name        string
		phasesData  []map[string]interface{}
		expectError string
	}{
		{
			name: "parallel field in phase object",
			phasesData: []map[string]interface{}{
				{"agent": "test-agent", "layer": 0, "parallel": map[string]interface{}{"models": []interface{}{"opus", "sonnet"}}},
			},
			expectError: "parallel",
		},
		{
			name: "parallel field without layer",
			phasesData: []map[string]interface{}{
				{"agent": "test-agent", "parallel": map[string]interface{}{"models": []interface{}{"opus"}}},
			},
			expectError: "parallel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phasesJSON, _ := json.Marshal(tt.phasesData)

			_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
				ID:          "test-parallel-" + strings.ReplaceAll(tt.name, " ", "-"),
				Description: "Test workflow",
				Phases:      phasesJSON,
			})

			if err == nil {
				t.Error("expected error for parallel field, got nil")
			} else if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error to contain '%s', got: %s", tt.expectError, err.Error())
			}

			// Verify migration hint is present
			if !strings.Contains(err.Error(), "layer-based") && !strings.Contains(err.Error(), "Migrate") {
				t.Errorf("expected migration hint in error message, got: %s", err.Error())
			}
		})
	}
}

// TestWorkflowRejectStringPhaseEntry tests that service rejects string-only phase entries
func TestWorkflowRejectStringPhaseEntry(t *testing.T) {
	env := NewTestEnv(t)

	tests := []struct {
		name        string
		phasesJSON  string
		expectError string
	}{
		{
			name:        "single string entry",
			phasesJSON:  `["analyzer"]`,
			expectError: "no longer supported",
		},
		{
			name:        "mixed string and object entries",
			phasesJSON:  `["analyzer", {"agent": "builder", "layer": 1}]`,
			expectError: "no longer supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
				ID:          "test-string-" + strings.ReplaceAll(tt.name, " ", "-"),
				Description: "Test workflow",
				Phases:      json.RawMessage(tt.phasesJSON),
			})

			if err == nil {
				t.Error("expected error for string phase entry, got nil")
			} else if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error to contain '%s', got: %s", tt.expectError, err.Error())
			}

			// Verify migration hint to object format
			if !strings.Contains(err.Error(), "object format") && !strings.Contains(err.Error(), `"agent"`) {
				t.Errorf("expected object format migration hint in error message, got: %s", err.Error())
			}
		})
	}
}

// TestWorkflowRequireLayerField tests that service rejects phases missing layer field
func TestWorkflowRequireLayerField(t *testing.T) {
	env := NewTestEnv(t)

	tests := []struct {
		name        string
		phasesData  []map[string]interface{}
		expectError string
	}{
		{
			name: "layer field with negative value",
			phasesData: []map[string]interface{}{
				{"agent": "analyzer", "layer": -1},
			},
			expectError: "layer must be >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phasesJSON, _ := json.Marshal(tt.phasesData)

			_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
				ID:          "test-layer-" + strings.ReplaceAll(tt.name, " ", "-"),
				Description: "Test workflow",
				Phases:      phasesJSON,
			})

			if err == nil {
				t.Error("expected error for missing/invalid layer, got nil")
			} else if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error to contain '%s', got: %s", tt.expectError, err.Error())
			}
		})
	}
}

// TestWorkflowFanInValidation tests that service enforces fan-in rules
func TestWorkflowFanInValidation(t *testing.T) {
	env := NewTestEnv(t)

	tests := []struct {
		name        string
		phasesData  []map[string]interface{}
		expectError bool
		errorText   string
	}{
		{
			name: "valid fan-in: 2 agents in layer 0, 1 in layer 1",
			phasesData: []map[string]interface{}{
				{"agent": "analyzer-a", "layer": 0},
				{"agent": "analyzer-b", "layer": 0},
				{"agent": "builder", "layer": 1},
			},
			expectError: false,
		},
		{
			name: "invalid fan-in: 2 agents in layer 0, 2 in layer 1",
			phasesData: []map[string]interface{}{
				{"agent": "analyzer-a", "layer": 0},
				{"agent": "analyzer-b", "layer": 0},
				{"agent": "builder-a", "layer": 1},
				{"agent": "builder-b", "layer": 1},
			},
			expectError: true,
			errorText:   "fan-in violation",
		},
		{
			name: "valid: single agent per layer",
			phasesData: []map[string]interface{}{
				{"agent": "analyzer", "layer": 0},
				{"agent": "builder", "layer": 1},
				{"agent": "tester", "layer": 2},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phasesJSON, _ := json.Marshal(tt.phasesData)

			_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
				ID:          "test-fanin-" + strings.ReplaceAll(tt.name, " ", "-"),
				Description: "Test workflow",
				Phases:      phasesJSON,
			})

			if tt.expectError {
				if err == nil {
					t.Error("expected fan-in error, got nil")
				} else if !strings.Contains(err.Error(), tt.errorText) {
					t.Errorf("expected error to contain '%s', got: %s", tt.errorText, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestWorkflowUpdateValidation tests that workflow update also validates layer config
func TestWorkflowUpdateValidation(t *testing.T) {
	env := NewTestEnv(t)

	// First create a valid workflow
	validPhasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "analyzer", "layer": 0},
		{"agent": "builder", "layer": 1},
	})

	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "update-test",
		Description: "Test workflow",
		Phases:      validPhasesJSON,
	})
	if err != nil {
		t.Fatalf("failed to create initial workflow: %v", err)
	}

	// Try to update with invalid phases (parallel field)
	invalidPhasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "analyzer", "layer": 0, "parallel": map[string]interface{}{"models": []interface{}{"opus"}}},
	})

	desc := "Updated workflow"
	phases := json.RawMessage(invalidPhasesJSON)

	err = env.WorkflowSvc.UpdateWorkflowDef(env.ProjectID, "update-test", &types.WorkflowDefUpdateRequest{
		Description: &desc,
		Phases:      &phases,
	})

	if err == nil {
		t.Error("expected error for update with parallel field, got nil")
	} else if !strings.Contains(err.Error(), "parallel") {
		t.Errorf("expected error to mention 'parallel', got: %s", err.Error())
	}
}

// TestWorkflowValidLayerConfig tests that valid layer configurations are accepted
func TestWorkflowValidLayerConfig(t *testing.T) {
	env := NewTestEnv(t)

	tests := []struct {
		name       string
		phasesData []map[string]interface{}
	}{
		{
			name: "simple sequential layers",
			phasesData: []map[string]interface{}{
				{"agent": "setup", "layer": 0},
				{"agent": "impl", "layer": 1},
				{"agent": "verify", "layer": 2},
			},
		},
		{
			name: "parallel agents in first layer only",
			phasesData: []map[string]interface{}{
				{"agent": "setup-a", "layer": 0},
				{"agent": "setup-b", "layer": 0},
				{"agent": "impl", "layer": 1},
			},
		},
		{
			name: "non-contiguous layer numbers",
			phasesData: []map[string]interface{}{
				{"agent": "setup", "layer": 0},
				{"agent": "impl", "layer": 5},
				{"agent": "verify", "layer": 10},
			},
		},
		{
			name: "single agent workflow",
			phasesData: []map[string]interface{}{
				{"agent": "hotfix", "layer": 0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phasesJSON, _ := json.Marshal(tt.phasesData)

			wf, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
				ID:          "valid-" + strings.ReplaceAll(tt.name, " ", "-"),
				Description: "Test workflow",
				Phases:      phasesJSON,
			})

			if err != nil {
				t.Errorf("expected no error for valid config, got: %v", err)
			}

			if wf != nil && wf.ID == "" {
				t.Error("expected non-empty workflow ID")
			}

			// Verify workflow can be retrieved
			retrieved, err := env.WorkflowSvc.GetWorkflowDef(env.ProjectID, wf.ID)
			if err != nil {
				t.Errorf("failed to retrieve created workflow: %v", err)
			}

			// Verify phases have layer field
			for _, phase := range retrieved.Phases {
				if phase.Layer < 0 {
					t.Errorf("phase has invalid layer value: %+v", phase)
				}
			}
		})
	}
}
