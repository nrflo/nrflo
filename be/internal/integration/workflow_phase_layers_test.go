package integration

import (
	"testing"
	"time"

	"be/internal/model"
	"be/internal/types"
)

// createWorkflowWithAgents creates a workflow definition and agent definitions with specified layers.
func createWorkflowWithAgents(t *testing.T, env *TestEnv, wfID string, agents []types.AgentDefCreateRequest) {
	t.Helper()
	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          wfID,
		Description: "Test workflow with layers",
	})
	if err != nil {
		t.Fatalf("failed to create workflow def %q: %v", wfID, err)
	}
	agentDefSvc := env.getAgentDefService(t)
	for _, ad := range agents {
		if _, err := agentDefSvc.CreateAgentDef(env.ProjectID, wfID, &ad); err != nil {
			t.Fatalf("failed to create agent def %s: %v", ad.ID, err)
		}
	}
}

// TestBuildV4State_PhaseLayers_SequentialLayers verifies that sequential phases each get
// their own distinct layer value in the phase_layers response field.
func TestBuildV4State_PhaseLayers_SequentialLayers(t *testing.T) {
	env := NewTestEnv(t)

	// Create a workflow with three sequential layers (0, 1, 2)
	createWorkflowWithAgents(t, env, "sequential", []types.AgentDefCreateRequest{
		{ID: "setup", Prompt: "s", Layer: 0},
		{ID: "build", Prompt: "b", Layer: 1},
		{ID: "verify", Prompt: "v", Layer: 2},
	})

	// Init and get status
	env.CreateTicket(t, "PL-1", "Phase layers sequential test")
	_, err := env.WorkflowSvc.Init(env.ProjectID, "PL-1", &types.WorkflowInitRequest{Workflow: "sequential"})
	if err != nil {
		t.Fatalf("failed to init workflow: %v", err)
	}

	raw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "PL-1", &types.WorkflowGetRequest{Workflow: "sequential"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	status := normalizeJSON(t, raw)

	// phase_layers must be present
	rawLayers, ok := status["phase_layers"]
	if !ok {
		t.Fatalf("expected phase_layers in response, got none. keys: %v", keys(status))
	}

	layers, ok := rawLayers.(map[string]interface{})
	if !ok {
		t.Fatalf("expected phase_layers to be a map, got %T: %v", rawLayers, rawLayers)
	}

	// Each phase should map to its own layer
	cases := []struct {
		phase string
		want  float64
	}{
		{"setup", 0},
		{"build", 1},
		{"verify", 2},
	}
	for _, tc := range cases {
		got, exists := layers[tc.phase]
		if !exists {
			t.Errorf("phase_layers[%q] missing, got map: %v", tc.phase, layers)
			continue
		}
		if got != tc.want {
			t.Errorf("phase_layers[%q] = %v, want %v", tc.phase, got, tc.want)
		}
	}
}

// TestBuildV4State_PhaseLayers_ParallelPhases verifies that phases in the same layer
// share the same layer value (this is what enables side-by-side ELK rendering).
func TestBuildV4State_PhaseLayers_ParallelPhases(t *testing.T) {
	env := NewTestEnv(t)

	// Create a workflow with two agents at layer 1 (parallel)
	createWorkflowWithAgents(t, env, "parallel", []types.AgentDefCreateRequest{
		{ID: "setup", Prompt: "s", Layer: 0},
		{ID: "test-be", Prompt: "t", Layer: 1},
		{ID: "test-fe", Prompt: "t", Layer: 1},
		{ID: "merge", Prompt: "m", Layer: 2},
	})

	env.CreateTicket(t, "PL-2", "Phase layers parallel test")
	_, err := env.WorkflowSvc.Init(env.ProjectID, "PL-2", &types.WorkflowInitRequest{Workflow: "parallel"})
	if err != nil {
		t.Fatalf("failed to init workflow: %v", err)
	}

	raw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "PL-2", &types.WorkflowGetRequest{Workflow: "parallel"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	status := normalizeJSON(t, raw)

	layers, ok := status["phase_layers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected phase_layers map in response, got %T", status["phase_layers"])
	}

	// Both parallel agents must share layer 1
	testBELayer, _ := layers["test-be"].(float64)
	testFELayer, _ := layers["test-fe"].(float64)
	if testBELayer != testFELayer {
		t.Errorf("parallel phases test-be (layer=%v) and test-fe (layer=%v) should share the same layer value",
			testBELayer, testFELayer)
	}
	if testBELayer != 1 {
		t.Errorf("test-be layer = %v, want 1", testBELayer)
	}

	// Surrounding sequential layers should differ
	setupLayer, _ := layers["setup"].(float64)
	mergeLayer, _ := layers["merge"].(float64)
	if setupLayer != 0 {
		t.Errorf("setup layer = %v, want 0", setupLayer)
	}
	if mergeLayer != 2 {
		t.Errorf("merge layer = %v, want 2", mergeLayer)
	}
}

// TestBuildV4State_PhaseLayers_WorkflowDefMissing verifies that when the workflow definition
// does not exist, phase_layers is absent from the response (frontend falls back to idx).
// This simulates the case where a workflow def is referenced by an instance but not in DB.
func TestBuildV4State_PhaseLayers_WorkflowDefMissing(t *testing.T) {
	env := NewTestEnv(t)

	// Build a WorkflowInstance that points to a non-existent workflow ID.
	// Use GetStatusByInstance directly — it doesn't require a DB row for the instance.
	wi := &model.WorkflowInstance{
		ID:         "test-instance-no-def",
		ProjectID:  env.ProjectID,
		WorkflowID: "nonexistent-workflow-def",
		Status:     model.WorkflowInstanceActive,
		ScopeType:  "ticket",
		Findings:   "{}",
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	raw, err := env.WorkflowSvc.GetStatusByInstance(wi)
	if err != nil {
		t.Fatalf("GetStatusByInstance should not fail when workflow def is missing: %v", err)
	}
	status := normalizeJSON(t, raw)

	// phase_layers must be absent (not nil, not empty map — simply not present)
	if _, exists := status["phase_layers"]; exists {
		t.Errorf("expected phase_layers to be absent when workflow def is missing, but it was present: %v",
			status["phase_layers"])
	}

	// The rest of the response must still be valid
	if v, _ := status["version"].(float64); v != 4 {
		t.Errorf("version = %v, want 4", v)
	}
	if _, ok := status["phase_order"]; !ok {
		t.Errorf("phase_order missing from response")
	}
	if _, ok := status["phases"]; !ok {
		t.Errorf("phases missing from response")
	}
}

// TestBuildV4State_PhaseLayers_ViaGetStatusByInstance verifies phase_layers is also
// returned when using GetStatusByInstance (the project workflow path).
func TestBuildV4State_PhaseLayers_ViaGetStatusByInstance(t *testing.T) {
	env := NewTestEnv(t)

	// Create a project-scoped workflow with parallel phases
	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "proj-wf",
		Description: "Project workflow",
		ScopeType:   "project",
	})
	if err != nil {
		t.Fatalf("failed to create project workflow def: %v", err)
	}
	agentDefSvc := env.getAgentDefService(t)
	for _, ad := range []types.AgentDefCreateRequest{
		{ID: "writer", Prompt: "w", Layer: 0},
		{ID: "reviewer", Prompt: "r", Layer: 1},
	} {
		if _, createErr := agentDefSvc.CreateAgentDef(env.ProjectID, "proj-wf", &ad); createErr != nil {
			t.Fatalf("failed to create agent def %s: %v", ad.ID, createErr)
		}
	}

	wi, err := env.WorkflowSvc.InitProjectWorkflow(env.ProjectID, &types.ProjectWorkflowRunRequest{
		Workflow: "proj-wf",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	raw, err := env.WorkflowSvc.GetStatusByInstance(wi)
	if err != nil {
		t.Fatalf("failed to get status by instance: %v", err)
	}
	status := normalizeJSON(t, raw)

	layers, ok := status["phase_layers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected phase_layers in response via GetStatusByInstance, got %T: %v",
			status["phase_layers"], status["phase_layers"])
	}

	if layers["writer"] != float64(0) {
		t.Errorf("writer layer = %v, want 0", layers["writer"])
	}
	if layers["reviewer"] != float64(1) {
		t.Errorf("reviewer layer = %v, want 1", layers["reviewer"])
	}
}

// TestBuildV4State_PhaseLayers_AllPhasesPresent verifies that all phases in the workflow
// definition are included in phase_layers (no missing entries).
func TestBuildV4State_PhaseLayers_AllPhasesPresent(t *testing.T) {
	env := NewTestEnv(t)

	// The default "test" workflow has "analyzer" (layer 0) and "builder" (layer 1)
	env.CreateTicket(t, "PL-5", "All phases present test")
	env.InitWorkflow(t, "PL-5")

	raw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "PL-5", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	status := normalizeJSON(t, raw)

	layers, ok := status["phase_layers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected phase_layers map, got %T", status["phase_layers"])
	}

	// phase_layers count must match phase_order count
	phaseOrder, _ := status["phase_order"].([]interface{})
	if len(layers) != len(phaseOrder) {
		t.Errorf("phase_layers has %d entries, want %d (matching phase_order %v)",
			len(layers), len(phaseOrder), phaseOrder)
	}

	// Every phase in phase_order must have a layer entry
	for _, name := range phaseOrder {
		phaseName, _ := name.(string)
		if _, exists := layers[phaseName]; !exists {
			t.Errorf("phase_layers missing entry for phase %q", phaseName)
		}
	}
}

// keys returns the keys of a map (for error messages).
func keys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
