package orchestrator

import (
	"testing"

	"be/internal/service"
	"be/internal/spawner"
)

// threeLayerGroups returns a canonical 3-layer group for plan tests.
func threeLayerGroups() []layerGroup {
	return []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 2}}},
	}
}

// fourLayerGroups returns a 4-layer group with a multi-agent layer 1.
func fourLayerGroups() []layerGroup {
	return []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "impl-a", Layer: 1}, {Agent: "impl-b", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "tester", Layer: 2}}},
		{layer: 3, phases: []service.SpawnerPhaseDef{{Agent: "deployer", Layer: 3}}},
	}
}

func TestLayerIndexOf(t *testing.T) {
	groups := threeLayerGroups()
	for _, tc := range []struct{ layer, want int }{
		{0, 0}, {1, 1}, {2, 2}, {3, -1}, {-1, -1},
	} {
		got := layerIndexOf(tc.layer, groups)
		if got != tc.want {
			t.Errorf("layerIndexOf(%d) = %d, want %d", tc.layer, got, tc.want)
		}
	}
}

func TestAgentLayerOf(t *testing.T) {
	groups := threeLayerGroups()
	tests := []struct{ agent string; wantLayer int; wantOK bool }{
		{"analyzer", 0, true},
		{"builder", 1, true},
		{"verifier", 2, true},
		{"unknown", 0, false},
	}
	for _, tc := range tests {
		layer, ok := agentLayerOf(tc.agent, groups)
		if ok != tc.wantOK || layer != tc.wantLayer {
			t.Errorf("agentLayerOf(%q) = (%d,%v), want (%d,%v)", tc.agent, layer, ok, tc.wantLayer, tc.wantOK)
		}
	}
}

func TestValidateCallbackRequest(t *testing.T) {
	groups := threeLayerGroups()
	tests := []struct {
		name        string
		req         spawner.CallbackError
		originator  int
		wantErr     bool
	}{
		{"level_valid", spawner.CallbackError{Level: 1}, 2, false},
		{"level_zero", spawner.CallbackError{Level: 0}, 2, false},
		{"level_equals_originator", spawner.CallbackError{Level: 2}, 2, false},
		{"level_exceeds_originator", spawner.CallbackError{Level: 3}, 2, true},
		{"level_not_found", spawner.CallbackError{Level: 5}, 2, true},
		{"agent_valid", spawner.CallbackError{Mode: "agent", TargetAgent: "builder"}, 2, false},
		{"agent_not_found", spawner.CallbackError{Mode: "agent", TargetAgent: "unknown"}, 2, true},
		{"agent_exceeds_originator", spawner.CallbackError{Mode: "agent", TargetAgent: "verifier"}, 1, true},
		{"chain_valid", spawner.CallbackError{Mode: "chain", Chain: []string{"analyzer", "builder"}}, 2, false},
		{"chain_empty", spawner.CallbackError{Mode: "chain", Chain: []string{}}, 2, true},
		{"chain_out_of_order", spawner.CallbackError{Mode: "chain", Chain: []string{"builder", "analyzer"}}, 2, true},
		{"chain_agent_not_found", spawner.CallbackError{Mode: "chain", Chain: []string{"analyzer", "unknown"}}, 2, true},
		{"chain_exceeds_originator", spawner.CallbackError{Mode: "chain", Chain: []string{"analyzer", "verifier"}}, 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			err := validateCallbackRequest(&req, groups[tc.originator].layer, groups)
			if (err != nil) != tc.wantErr {
				t.Errorf("wantErr=%v but got err=%v", tc.wantErr, err)
			}
		})
	}
}

func TestDecomposeCallback_Level(t *testing.T) {
	groups := threeLayerGroups()
	req := &spawner.CallbackError{Level: 1, AgentType: "verifier", Instructions: "fix it"}
	d := decomposeCallback(req, 2, groups)
	if len(d.steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(d.steps))
	}
	if d.steps[0].layer != 1 || !d.steps[0].wholeLayer {
		t.Errorf("step[0]: got layer=%d whole=%v, want layer=1 whole=true", d.steps[0].layer, d.steps[0].wholeLayer)
	}
	if d.steps[1].layer != 2 || !d.steps[1].wholeLayer {
		t.Errorf("step[1]: got layer=%d whole=%v, want layer=2 whole=true", d.steps[1].layer, d.steps[1].wholeLayer)
	}
	if d.resumeLayer != 3 {
		t.Errorf("resumeLayer = %d, want 3", d.resumeLayer)
	}
	// resetScope must contain both agents in layers 1 and 2
	if len(d.resetScope) != 2 {
		t.Errorf("resetScope len = %d, want 2", len(d.resetScope))
	}
	// decomposeLevelCallback copies req.Instructions to all steps
	if d.steps[0].layerInstr != "fix it" {
		t.Errorf("step[0] layerInstr = %q, want 'fix it'", d.steps[0].layerInstr)
	}
	if d.steps[1].layerInstr != "fix it" {
		t.Errorf("step[1] layerInstr = %q, want 'fix it' (all steps share req.Instructions)", d.steps[1].layerInstr)
	}
}

func TestDecomposeCallback_Agent(t *testing.T) {
	groups := threeLayerGroups()
	req := &spawner.CallbackError{Mode: "agent", TargetAgent: "builder", AgentType: "verifier", Instructions: "fix builder"}
	d := decomposeCallback(req, 2, groups)
	if len(d.steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(d.steps))
	}
	// first step: per-agent step for builder at layer 1
	s0 := d.steps[0]
	if s0.layer != 1 || s0.wholeLayer {
		t.Errorf("step[0]: layer=%d whole=%v, want layer=1 whole=false", s0.layer, s0.wholeLayer)
	}
	if len(s0.agents) != 1 || s0.agents[0] != "builder" {
		t.Errorf("step[0].agents = %v, want [builder]", s0.agents)
	}
	if s0.perAgentInstr["builder"] != "fix builder" {
		t.Errorf("step[0] perAgentInstr[builder] = %q", s0.perAgentInstr["builder"])
	}
	// second step: whole-layer step for layer 2
	s1 := d.steps[1]
	if s1.layer != 2 || !s1.wholeLayer {
		t.Errorf("step[1]: layer=%d whole=%v, want layer=2 whole=true", s1.layer, s1.wholeLayer)
	}
	if d.resumeLayer != 3 {
		t.Errorf("resumeLayer = %d, want 3", d.resumeLayer)
	}
}

func TestDecomposeCallback_Chain(t *testing.T) {
	groups := threeLayerGroups()
	req := &spawner.CallbackError{Mode: "chain", Chain: []string{"analyzer", "builder"}, AgentType: "verifier", Instructions: "chain instr"}
	d := decomposeCallback(req, 2, groups)
	if len(d.steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(d.steps))
	}
	// steps sorted by layer: analyzer(0), builder(1)
	if d.steps[0].layer != 0 || d.steps[0].agents[0] != "analyzer" {
		t.Errorf("step[0]: %+v", d.steps[0])
	}
	if d.steps[0].perAgentInstr["analyzer"] != "chain instr" {
		t.Errorf("chain[0] should get instructions, got %q", d.steps[0].perAgentInstr["analyzer"])
	}
	if d.steps[1].perAgentInstr["builder"] != "" {
		t.Errorf("chain[1..] should have empty instructions, got %q", d.steps[1].perAgentInstr["builder"])
	}
	if d.resumeLayer != 2 {
		t.Errorf("resumeLayer = %d, want 2 (lastLayer+1=1+1)", d.resumeLayer)
	}
}

func TestMergeCallbackPlans_WholeLayerWins(t *testing.T) {
	// Per-agent step + whole-layer step for same layer → whole-layer wins.
	parts := []decomposedRequest{
		{
			agentID: "agent-a",
			steps:   []callbackPlanStep{{layer: 1, wholeLayer: false, agents: []string{"builder"}, perAgentInstr: map[string]string{"builder": "fix"}}},
			resetScope: []string{"builder"}, resumeLayer: 2,
		},
		{
			agentID: "agent-b",
			steps:   []callbackPlanStep{{layer: 1, wholeLayer: true, layerInstr: "full layer"}},
			resetScope: []string{"builder"}, resumeLayer: 2,
		},
	}
	plan := mergeCallbackPlans(parts)
	if len(plan.steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(plan.steps))
	}
	if !plan.steps[0].wholeLayer {
		t.Error("whole_layer should win over per-agent step")
	}
	if plan.steps[0].layerInstr != "full layer" {
		t.Errorf("layerInstr = %q, want 'full layer'", plan.steps[0].layerInstr)
	}
}

func TestMergeCallbackPlans_InstructionsDeterminism(t *testing.T) {
	// Two whole-layer steps: instructions joined sorted by agentID.
	parts := []decomposedRequest{
		{agentID: "z-agent", steps: []callbackPlanStep{{layer: 0, wholeLayer: true, layerInstr: "Z instruction"}}, resetScope: []string{"analyzer"}, resumeLayer: 1},
		{agentID: "a-agent", steps: []callbackPlanStep{{layer: 0, wholeLayer: true, layerInstr: "A instruction"}}, resetScope: []string{"analyzer"}, resumeLayer: 1},
	}
	plan := mergeCallbackPlans(parts)
	// a-agent sorts before z-agent → "A instruction\n---\nZ instruction"
	want := "A instruction\n---\nZ instruction"
	if plan.steps[0].layerInstr != want {
		t.Errorf("layerInstr = %q, want %q", plan.steps[0].layerInstr, want)
	}
}

func TestMergeCallbackPlans_DedupeResetScope(t *testing.T) {
	parts := []decomposedRequest{
		{agentID: "a", steps: nil, resetScope: []string{"builder", "analyzer"}, resumeLayer: 1},
		{agentID: "b", steps: nil, resetScope: []string{"analyzer", "verifier"}, resumeLayer: 1},
	}
	plan := mergeCallbackPlans(parts)
	if len(plan.resetScope) != 3 {
		t.Fatalf("resetScope len = %d, want 3 (deduped union)", len(plan.resetScope))
	}
	// sorted
	if plan.resetScope[0] != "analyzer" || plan.resetScope[1] != "builder" || plan.resetScope[2] != "verifier" {
		t.Errorf("resetScope = %v, want sorted deduped", plan.resetScope)
	}
}

func TestMergeCallbackPlans_MaxResumeLayer(t *testing.T) {
	parts := []decomposedRequest{
		{agentID: "a", steps: nil, resetScope: nil, resumeLayer: 2},
		{agentID: "b", steps: nil, resetScope: nil, resumeLayer: 4},
		{agentID: "c", steps: nil, resetScope: nil, resumeLayer: 1},
	}
	plan := mergeCallbackPlans(parts)
	if plan.resumeLayer != 4 {
		t.Errorf("resumeLayer = %d, want 4 (max)", plan.resumeLayer)
	}
}

func TestCumulativeAgentCount(t *testing.T) {
	groups := fourLayerGroups() // layers: 0(1 agent), 1(2 agents), 2(1 agent), 3(1 agent)
	tests := []struct {
		name  string
		plan  callbackPlan
		want  int
	}{
		{
			"whole_layer_single_agent",
			callbackPlan{steps: []callbackPlanStep{{layer: 0, wholeLayer: true}}},
			1,
		},
		{
			"whole_layer_multi_agent",
			callbackPlan{steps: []callbackPlanStep{{layer: 1, wholeLayer: true}}},
			2,
		},
		{
			"per_agent_step",
			callbackPlan{steps: []callbackPlanStep{{layer: 1, wholeLayer: false, agents: []string{"impl-a"}}}},
			1,
		},
		{
			"mixed_steps",
			callbackPlan{steps: []callbackPlanStep{
				{layer: 0, wholeLayer: false, agents: []string{"analyzer"}},
				{layer: 1, wholeLayer: true},
			}},
			3,
		},
		{
			"empty_plan",
			callbackPlan{steps: nil},
			0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cumulativeAgentCount(tc.plan, groups)
			if got != tc.want {
				t.Errorf("cumulativeAgentCount = %d, want %d", got, tc.want)
			}
		})
	}
}
