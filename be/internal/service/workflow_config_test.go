package service

import (
	"testing"

	"be/internal/model"
)

func TestBuildSpawnerConfig_PhasesFromAgentDefs(t *testing.T) {
	wfs := []*model.Workflow{
		{ID: "feature", ProjectID: "p1", Description: "Feature workflow", ScopeType: "ticket", CloseTicketOnComplete: true},
	}
	ads := []*model.AgentDefinition{
		{ID: "verifier", ProjectID: "p1", WorkflowID: "feature", Model: "sonnet", Timeout: 20, Layer: 2},
		{ID: "setup", ProjectID: "p1", WorkflowID: "feature", Model: "haiku", Timeout: 10, Layer: 0},
		{ID: "builder", ProjectID: "p1", WorkflowID: "feature", Model: "opus", Timeout: 30, Layer: 1},
	}

	workflows, agents := BuildSpawnerConfig(wfs, ads)

	wf, ok := workflows["feature"]
	if !ok {
		t.Fatal("feature workflow not found")
	}

	// Phases sorted by layer ASC, id ASC
	if len(wf.Phases) != 3 {
		t.Fatalf("phases count = %d, want 3", len(wf.Phases))
	}
	want := []struct {
		id    string
		layer int
	}{
		{"setup", 0}, {"builder", 1}, {"verifier", 2},
	}
	for i, w := range want {
		if wf.Phases[i].ID != w.id || wf.Phases[i].Layer != w.layer {
			t.Errorf("phases[%d] = {%s, L%d}, want {%s, L%d}", i, wf.Phases[i].ID, wf.Phases[i].Layer, w.id, w.layer)
		}
		if wf.Phases[i].Agent != w.id {
			t.Errorf("phases[%d].Agent = %s, want %s", i, wf.Phases[i].Agent, w.id)
		}
	}

	// Agent configs
	if len(agents) != 3 {
		t.Fatalf("agents count = %d, want 3", len(agents))
	}
	if agents["setup"].Model != "haiku" {
		t.Errorf("agents[setup].Model = %s, want haiku", agents["setup"].Model)
	}
	if agents["builder"].Timeout != 30 {
		t.Errorf("agents[builder].Timeout = %d, want 30", agents["builder"].Timeout)
	}
}

func TestBuildSpawnerConfig_ParallelAgentsSameLayer(t *testing.T) {
	wfs := []*model.Workflow{
		{ID: "parallel", ProjectID: "p1", ScopeType: "ticket"},
	}
	ads := []*model.AgentDefinition{
		{ID: "test-fe", ProjectID: "p1", WorkflowID: "parallel", Model: "sonnet", Timeout: 20, Layer: 1},
		{ID: "test-be", ProjectID: "p1", WorkflowID: "parallel", Model: "sonnet", Timeout: 20, Layer: 1},
		{ID: "setup", ProjectID: "p1", WorkflowID: "parallel", Model: "haiku", Timeout: 10, Layer: 0},
		{ID: "merge", ProjectID: "p1", WorkflowID: "parallel", Model: "sonnet", Timeout: 20, Layer: 2},
	}

	workflows, _ := BuildSpawnerConfig(wfs, ads)
	wf := workflows["parallel"]

	if len(wf.Phases) != 4 {
		t.Fatalf("phases count = %d, want 4", len(wf.Phases))
	}
	// L0: setup, L1: test-be, test-fe (alphabetical), L2: merge
	want := []struct {
		id    string
		layer int
	}{
		{"setup", 0}, {"test-be", 1}, {"test-fe", 1}, {"merge", 2},
	}
	for i, w := range want {
		if wf.Phases[i].ID != w.id || wf.Phases[i].Layer != w.layer {
			t.Errorf("phases[%d] = {%s, L%d}, want {%s, L%d}", i, wf.Phases[i].ID, wf.Phases[i].Layer, w.id, w.layer)
		}
	}
}

func TestBuildSpawnerConfig_EmptyAgentDefs(t *testing.T) {
	wfs := []*model.Workflow{
		{ID: "empty", ProjectID: "p1", ScopeType: "ticket"},
	}
	var ads []*model.AgentDefinition

	workflows, agents := BuildSpawnerConfig(wfs, ads)

	wf := workflows["empty"]
	if len(wf.Phases) != 0 {
		t.Errorf("expected 0 phases for empty workflow, got %d", len(wf.Phases))
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestBuildSpawnerConfig_ScopeTypeDefault(t *testing.T) {
	wfs := []*model.Workflow{
		{ID: "wf1", ProjectID: "p1", ScopeType: ""},
	}

	workflows, _ := BuildSpawnerConfig(wfs, nil)
	if workflows["wf1"].ScopeType != "ticket" {
		t.Errorf("ScopeType = %q, want 'ticket' when empty", workflows["wf1"].ScopeType)
	}
}

func TestBuildSpawnerConfig_MultipleWorkflows(t *testing.T) {
	wfs := []*model.Workflow{
		{ID: "wf-a", ProjectID: "p1", ScopeType: "ticket"},
		{ID: "wf-b", ProjectID: "p1", ScopeType: "project"},
	}
	ads := []*model.AgentDefinition{
		{ID: "agent-a", ProjectID: "p1", WorkflowID: "wf-a", Model: "sonnet", Timeout: 20, Layer: 0},
		{ID: "agent-b", ProjectID: "p1", WorkflowID: "wf-b", Model: "haiku", Timeout: 10, Layer: 0},
	}

	workflows, _ := BuildSpawnerConfig(wfs, ads)

	if len(workflows) != 2 {
		t.Fatalf("workflows count = %d, want 2", len(workflows))
	}
	if len(workflows["wf-a"].Phases) != 1 || workflows["wf-a"].Phases[0].ID != "agent-a" {
		t.Errorf("wf-a phases unexpected: %+v", workflows["wf-a"].Phases)
	}
	if len(workflows["wf-b"].Phases) != 1 || workflows["wf-b"].Phases[0].ID != "agent-b" {
		t.Errorf("wf-b phases unexpected: %+v", workflows["wf-b"].Phases)
	}
}

func TestBuildSpawnerConfig_AgentTag(t *testing.T) {
	wfs := []*model.Workflow{
		{ID: "wf1", ProjectID: "p1", ScopeType: "ticket"},
	}
	ads := []*model.AgentDefinition{
		{ID: "tagged", ProjectID: "p1", WorkflowID: "wf1", Model: "sonnet", Timeout: 20, Layer: 0, Tag: "be"},
	}

	_, agents := BuildSpawnerConfig(wfs, ads)
	if agents["tagged"].Tag != "be" {
		t.Errorf("agent tag = %q, want 'be'", agents["tagged"].Tag)
	}
}
