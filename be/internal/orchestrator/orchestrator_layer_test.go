package orchestrator

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/service"
)

// TestLayerGroupingAndSequencing tests that phases are correctly grouped by layer
// and layers execute in ascending order.
func TestLayerGroupingAndSequencing(t *testing.T) {
	wf := &model.Workflow{ID: "layered", ProjectID: "proj"}
	defs := []*model.AgentDefinition{
		{ID: "setup-a", WorkflowID: "layered", Layer: 0},
		{ID: "setup-b", WorkflowID: "layered", Layer: 0},
		{ID: "analyzer", WorkflowID: "layered", Layer: 1},
		{ID: "impl-a", WorkflowID: "layered", Layer: 2},
		{ID: "impl-b", WorkflowID: "layered", Layer: 2},
		{ID: "verifier", WorkflowID: "layered", Layer: 3},
	}
	workflows, _ := service.BuildSpawnerConfig(nil, clock.Real(), []*model.Workflow{wf}, defs)

	groups := groupPhasesByLayer(workflows["layered"].Phases)

	if len(groups) != 4 {
		t.Fatalf("expected 4 layer groups, got %d", len(groups))
	}

	// Layer 0: 2 agents
	if groups[0].layer != 0 || len(groups[0].phases) != 2 {
		t.Errorf("layer 0: expected 2 agents, got %d (layer=%d)", len(groups[0].phases), groups[0].layer)
	}

	// Layer 1: 1 agent
	if groups[1].layer != 1 || len(groups[1].phases) != 1 {
		t.Errorf("layer 1: expected 1 agent, got %d (layer=%d)", len(groups[1].phases), groups[1].layer)
	}

	// Layer 2: 2 agents
	if groups[2].layer != 2 || len(groups[2].phases) != 2 {
		t.Errorf("layer 2: expected 2 agents, got %d (layer=%d)", len(groups[2].phases), groups[2].layer)
	}

	// Layer 3: 1 agent
	if groups[3].layer != 3 || len(groups[3].phases) != 1 {
		t.Errorf("layer 3: expected 1 agent, got %d (layer=%d)", len(groups[3].phases), groups[3].layer)
	}

	// Verify agents in each group
	if groups[0].phases[0].Agent != "setup-a" && groups[0].phases[0].Agent != "setup-b" {
		t.Errorf("layer 0 agents incorrect: %v", groups[0].phases)
	}
	if groups[1].phases[0].Agent != "analyzer" {
		t.Errorf("layer 1 agent incorrect: %v", groups[1].phases)
	}
	if groups[2].phases[0].Agent != "impl-a" && groups[2].phases[0].Agent != "impl-b" {
		t.Errorf("layer 2 agents incorrect: %v", groups[2].phases)
	}
	if groups[3].phases[0].Agent != "verifier" {
		t.Errorf("layer 3 agent incorrect: %v", groups[3].phases)
	}
}

// TestNonContiguousLayers tests that non-contiguous layer numbers are handled correctly
func TestNonContiguousLayers(t *testing.T) {
	wf := &model.Workflow{ID: "sparse", ProjectID: "proj"}
	defs := []*model.AgentDefinition{
		{ID: "setup", WorkflowID: "sparse", Layer: 0},
		{ID: "impl", WorkflowID: "sparse", Layer: 5},
		{ID: "verify", WorkflowID: "sparse", Layer: 10},
	}
	workflows, _ := service.BuildSpawnerConfig(nil, clock.Real(), []*model.Workflow{wf}, defs)

	groups := groupPhasesByLayer(workflows["sparse"].Phases)

	if len(groups) != 3 {
		t.Fatalf("expected 3 layer groups, got %d", len(groups))
	}

	if groups[0].layer != 0 || groups[1].layer != 5 || groups[2].layer != 10 {
		t.Errorf("expected layers [0, 5, 10], got [%d, %d, %d]", groups[0].layer, groups[1].layer, groups[2].layer)
	}
}

// TestSingleAgentLayer tests that a layer with a single agent works correctly.
func TestSingleAgentLayer(t *testing.T) {
	wf := &model.Workflow{ID: "single", ProjectID: "proj"}
	defs := []*model.AgentDefinition{
		{ID: "hotfix", WorkflowID: "single", Layer: 0},
	}
	workflows, _ := service.BuildSpawnerConfig(nil, clock.Real(), []*model.Workflow{wf}, defs)

	groups := groupPhasesByLayer(workflows["single"].Phases)

	if len(groups) != 1 {
		t.Fatalf("expected 1 layer group, got %d", len(groups))
	}

	if len(groups[0].phases) != 1 || groups[0].phases[0].Agent != "hotfix" {
		t.Errorf("expected single 'hotfix' agent in layer 0, got: %v", groups[0].phases)
	}
}

// TestLayerOrderPreserved tests that layer groups are returned in ascending order
// regardless of the order phases appear in the definition.
func TestLayerOrderPreserved(t *testing.T) {
	// Agent defs in reverse layer order to verify sorting
	wf := &model.Workflow{ID: "unordered", ProjectID: "proj"}
	defs := []*model.AgentDefinition{
		{ID: "verify", WorkflowID: "unordered", Layer: 3},
		{ID: "impl", WorkflowID: "unordered", Layer: 2},
		{ID: "analyze", WorkflowID: "unordered", Layer: 1},
		{ID: "setup", WorkflowID: "unordered", Layer: 0},
	}
	workflows, _ := service.BuildSpawnerConfig(nil, clock.Real(), []*model.Workflow{wf}, defs)

	groups := groupPhasesByLayer(workflows["unordered"].Phases)

	// Verify layers are in ascending order
	for i := 0; i < len(groups); i++ {
		if groups[i].layer != i {
			t.Errorf("expected layer %d at index %d, got layer %d", i, i, groups[i].layer)
		}
	}

	// Verify agent order matches layer order
	expectedAgents := []string{"setup", "analyze", "impl", "verify"}
	for i, group := range groups {
		if group.phases[0].Agent != expectedAgents[i] {
			t.Errorf("expected agent '%s' in layer %d, got '%s'", expectedAgents[i], i, group.phases[0].Agent)
		}
	}
}
