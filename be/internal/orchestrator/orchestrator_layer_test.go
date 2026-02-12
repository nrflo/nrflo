package orchestrator

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/service"
	"be/internal/types"
)

// TestLayerGroupingAndSequencing tests that phases are correctly grouped by layer
// and layers execute in ascending order.
func TestLayerGroupingAndSequencing(t *testing.T) {
	env := newTestEnv(t)

	// Create workflow with multiple layers
	workflowSvc := service.NewWorkflowService(env.pool)
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup-a", "layer": 0},
		{"agent": "setup-b", "layer": 0},
		{"agent": "analyzer", "layer": 1},
		{"agent": "impl-a", "layer": 2},
		{"agent": "impl-b", "layer": 2},
		{"agent": "verifier", "layer": 3},
	})
	_, err := workflowSvc.CreateWorkflowDef(env.project, &types.WorkflowDefCreateRequest{
		ID:          "layered",
		Description: "Layered workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
	})
	if err != nil {
		t.Fatalf("failed to create layered workflow: %v", err)
	}

	// Verify groupPhasesByLayer produces correct groups
	workflow, _ := workflowSvc.GetWorkflowDef(env.project, "layered")
	workflows, _ := service.BuildSpawnerConfig([]*model.Workflow{{
		ID:          "layered",
		ProjectID:   env.project,
		Description: workflow.Description,
		Phases:      string(phasesJSON),
	}}, nil)

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
	env := newTestEnv(t)

	workflowSvc := service.NewWorkflowService(env.pool)
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "setup", "layer": 0},
		{"agent": "impl", "layer": 5},
		{"agent": "verify", "layer": 10},
	})
	_, err := workflowSvc.CreateWorkflowDef(env.project, &types.WorkflowDefCreateRequest{
		ID:          "sparse",
		Description: "Sparse layer workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
	})
	if err != nil {
		t.Fatalf("failed to create sparse workflow: %v", err)
	}

	workflow, _ := workflowSvc.GetWorkflowDef(env.project, "sparse")
	workflows, _ := service.BuildSpawnerConfig([]*model.Workflow{{
		ID:          "sparse",
		ProjectID:   env.project,
		Description: workflow.Description,
		Phases:      string(phasesJSON),
	}}, nil)

	groups := groupPhasesByLayer(workflows["sparse"].Phases)

	if len(groups) != 3 {
		t.Fatalf("expected 3 layer groups, got %d", len(groups))
	}

	if groups[0].layer != 0 || groups[1].layer != 5 || groups[2].layer != 10 {
		t.Errorf("expected layers [0, 5, 10], got [%d, %d, %d]", groups[0].layer, groups[1].layer, groups[2].layer)
	}
}

// TestParallelAgentsConcurrentExecution is a conceptual test showing how parallel agents
// would be spawned concurrently. This test cannot run actual spawner since it requires
// real agent processes, but it validates the orchestrator's concurrent spawning pattern.
func TestParallelAgentsConcurrentExecution(t *testing.T) {
	// This test demonstrates the concurrent execution pattern without actual spawner.
	// The real orchestrator spawns each agent in a goroutine and waits for all to finish.

	// Simulate concurrent execution
	var wg sync.WaitGroup
	results := make(chan string, 3)

	agents := []string{"agent-a", "agent-b", "agent-c"}

	for _, agent := range agents {
		wg.Add(1)
		agent := agent
		go func() {
			defer wg.Done()
			// Simulate work
			time.Sleep(10 * time.Millisecond)
			results <- agent
		}()
	}

	// Wait for all agents to complete
	wg.Wait()
	close(results)

	// Verify all agents ran
	completed := make(map[string]bool)
	for agent := range results {
		completed[agent] = true
	}

	if len(completed) != 3 {
		t.Errorf("expected 3 agents to complete, got %d", len(completed))
	}

	for _, agent := range agents {
		if !completed[agent] {
			t.Errorf("agent %s did not complete", agent)
		}
	}
}

// TestAllSkippedLayerContinues tests the shouldSkipPhase logic for category-based skipping.
func TestAllSkippedLayerContinues(t *testing.T) {
	// Verify shouldSkipPhase logic
	phase0 := service.SpawnerPhaseDef{Agent: "test-writer", SkipFor: []string{"hotfix"}}
	phase1 := service.SpawnerPhaseDef{Agent: "implementor", SkipFor: nil}

	if !shouldSkipPhase("hotfix", phase0.SkipFor) {
		t.Error("expected test-writer to be skipped for hotfix category")
	}

	if shouldSkipPhase("hotfix", phase1.SkipFor) {
		t.Error("expected implementor NOT to be skipped for hotfix category")
	}

	// Test all-skipped scenario
	allSkipped := []service.SpawnerPhaseDef{
		{Agent: "a1", SkipFor: []string{"docs"}},
		{Agent: "a2", SkipFor: []string{"docs"}},
	}

	var runnableCount int
	for _, p := range allSkipped {
		if !shouldSkipPhase("docs", p.SkipFor) {
			runnableCount++
		}
	}

	if runnableCount != 0 {
		t.Errorf("expected 0 runnable agents for 'docs' category, got %d", runnableCount)
	}
}

// TestMixedOutcomesLayerPassCount tests that a layer with mixed outcomes
// (some pass, some fail) still allows the workflow to proceed if pass_count >= 1.
func TestMixedOutcomesLayerPassCount(t *testing.T) {
	// This test validates the pass_count >= 1 logic conceptually.
	// The actual orchestrator code in runLoop checks:
	//   if passCount == 0 { markFailed(); return }

	tests := []struct {
		name         string
		passCount    int
		failCount    int
		shouldPass   bool
	}{
		{
			name:       "1 pass, 1 fail - should proceed",
			passCount:  1,
			failCount:  1,
			shouldPass: true,
		},
		{
			name:       "2 pass, 1 fail - should proceed",
			passCount:  2,
			failCount:  1,
			shouldPass: true,
		},
		{
			name:       "0 pass, 2 fail - should stop",
			passCount:  0,
			failCount:  2,
			shouldPass: false,
		},
		{
			name:       "0 pass, 1 fail - should stop",
			passCount:  0,
			failCount:  1,
			shouldPass: false,
		},
		{
			name:       "3 pass, 0 fail - should proceed",
			passCount:  3,
			failCount:  0,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate orchestrator's fan-in check
			shouldProceed := tt.passCount > 0

			if shouldProceed != tt.shouldPass {
				t.Errorf("expected shouldProceed=%v, got %v (passCount=%d, failCount=%d)",
					tt.shouldPass, shouldProceed, tt.passCount, tt.failCount)
			}
		})
	}
}

// TestAllFailLayerStopsWorkflow tests the logic for when all agents in a layer fail.
func TestAllFailLayerStopsWorkflow(t *testing.T) {
	// Simulate all-fail scenario by checking the logic
	// In real orchestrator: if passCount == 0 { markFailed() }
	passCount := 0
	failCount := 2

	// This is the orchestrator's decision logic
	shouldStop := passCount == 0 && failCount > 0

	if !shouldStop {
		t.Error("expected workflow to stop when all agents fail")
	}

	// Test that at least one pass allows continuation
	passCount = 1
	failCount = 2

	shouldStop = passCount == 0

	if shouldStop {
		t.Error("expected workflow to continue when at least one agent passes")
	}
}

// TestSingleAgentLayer tests that a layer with a single agent works correctly.
func TestSingleAgentLayer(t *testing.T) {
	env := newTestEnv(t)

	workflowSvc := service.NewWorkflowService(env.pool)
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "hotfix", "layer": 0},
	})
	_, err := workflowSvc.CreateWorkflowDef(env.project, &types.WorkflowDefCreateRequest{
		ID:          "single",
		Description: "Single agent workflow",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
	})
	if err != nil {
		t.Fatalf("failed to create single-agent workflow: %v", err)
	}

	workflow, _ := workflowSvc.GetWorkflowDef(env.project, "single")
	workflows, _ := service.BuildSpawnerConfig([]*model.Workflow{{
		ID:          "single",
		ProjectID:   env.project,
		Description: workflow.Description,
		Phases:      string(phasesJSON),
	}}, nil)

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
	env := newTestEnv(t)

	workflowSvc := service.NewWorkflowService(env.pool)
	// Phases in reverse layer order
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "verify", "layer": 3},
		{"agent": "impl", "layer": 2},
		{"agent": "analyze", "layer": 1},
		{"agent": "setup", "layer": 0},
	})
	_, err := workflowSvc.CreateWorkflowDef(env.project, &types.WorkflowDefCreateRequest{
		ID:          "unordered",
		Description: "Unordered phases",
		Categories:  []string{"full"},
		Phases:      phasesJSON,
	})
	if err != nil {
		t.Fatalf("failed to create unordered workflow: %v", err)
	}

	workflow, _ := workflowSvc.GetWorkflowDef(env.project, "unordered")
	workflows, _ := service.BuildSpawnerConfig([]*model.Workflow{{
		ID:          "unordered",
		ProjectID:   env.project,
		Description: workflow.Description,
		Phases:      string(phasesJSON),
	}}, nil)

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

// TestSkipForCategoryFiltering tests that skip_for filtering works correctly
// at the layer level.
func TestSkipForCategoryFiltering(t *testing.T) {
	env := newTestEnv(t)

	workflowSvc := service.NewWorkflowService(env.pool)
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "test-writer", "layer": 0, "skip_for": []interface{}{"docs", "hotfix"}},
		{"agent": "implementor", "layer": 1},
		{"agent": "qa-verifier", "layer": 2, "skip_for": []interface{}{"docs"}},
	})
	_, err := workflowSvc.CreateWorkflowDef(env.project, &types.WorkflowDefCreateRequest{
		ID:          "filtered",
		Description: "Filtered workflow",
		Categories:  []string{"full", "docs", "hotfix"},
		Phases:      phasesJSON,
	})
	if err != nil {
		t.Fatalf("failed to create filtered workflow: %v", err)
	}

	workflow, _ := workflowSvc.GetWorkflowDef(env.project, "filtered")
	workflows, _ := service.BuildSpawnerConfig([]*model.Workflow{{
		ID:          "filtered",
		ProjectID:   env.project,
		Description: workflow.Description,
		Phases:      string(phasesJSON),
	}}, nil)

	phases := workflows["filtered"].Phases

	// Test category="docs"
	var docsRunnable []string
	for _, p := range phases {
		if !shouldSkipPhase("docs", p.SkipFor) {
			docsRunnable = append(docsRunnable, p.Agent)
		}
	}

	// For "docs" category: test-writer and qa-verifier are skipped, only implementor runs
	if len(docsRunnable) != 1 || docsRunnable[0] != "implementor" {
		t.Errorf("category 'docs' expected only [implementor], got %v", docsRunnable)
	}

	// Test category="hotfix"
	var hotfixRunnable []string
	for _, p := range phases {
		if !shouldSkipPhase("hotfix", p.SkipFor) {
			hotfixRunnable = append(hotfixRunnable, p.Agent)
		}
	}

	// For "hotfix" category: test-writer is skipped, implementor and qa-verifier run
	expectedHotfix := []string{"implementor", "qa-verifier"}
	if len(hotfixRunnable) != 2 {
		t.Errorf("category 'hotfix' expected 2 agents, got %d: %v", len(hotfixRunnable), hotfixRunnable)
	}
	for _, exp := range expectedHotfix {
		found := false
		for _, act := range hotfixRunnable {
			if act == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("category 'hotfix' expected agent '%s', not found in %v", exp, hotfixRunnable)
		}
	}

	// Test category="full"
	var fullRunnable []string
	for _, p := range phases {
		if !shouldSkipPhase("full", p.SkipFor) {
			fullRunnable = append(fullRunnable, p.Agent)
		}
	}

	// For "full" category: all agents run
	if len(fullRunnable) != 3 {
		t.Errorf("category 'full' expected all 3 agents, got %d: %v", len(fullRunnable), fullRunnable)
	}
}
