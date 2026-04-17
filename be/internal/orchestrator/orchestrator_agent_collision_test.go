package orchestrator

import (
	"context"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/service"
)

// TestBuildSpawnerConfigAgentCollision documents the cross-workflow agent ID collision
// that occurred when Start()/retryFailed() loaded ALL project workflows into a flat
// agent map (last-write-wins). It also verifies that single-workflow loading is
// collision-free, serving as a regression test for the fix.
func TestBuildSpawnerConfigAgentCollision(t *testing.T) {
	featureWf := &model.Workflow{
		ID:        "feature",
		ProjectID: "proj",
	}
	bugfixWf := &model.Workflow{
		ID:        "bugfix",
		ProjectID: "proj",
	}

	featureDef := &model.AgentDefinition{
		ID:         "implementor",
		ProjectID:  "proj",
		WorkflowID: "feature",
		Model:      "opus_4_7",
		Timeout:    3600,
		Layer:      0,
	}
	bugfixDef := &model.AgentDefinition{
		ID:         "implementor",
		ProjectID:  "proj",
		WorkflowID: "bugfix",
		Model:      "sonnet",
		Timeout:    1200,
		Layer:      0,
	}

	// Mixed input (both workflows): documents the pre-fix last-write-wins collision.
	t.Run("mixed workflow input overwrites earlier def", func(t *testing.T) {
		_, agents := service.BuildSpawnerConfig(
			[]*model.Workflow{featureWf, bugfixWf},
			[]*model.AgentDefinition{featureDef, bugfixDef},
		)
		// bugfixDef is last → "sonnet" wins; featureDef's "opus_4_7" is silently lost.
		if got := agents["implementor"].Model; got != "sonnet" {
			t.Errorf("expected last-written model %q, got %q", "sonnet", got)
		}
	})

	// Reversed order shows the collision is input-order dependent.
	t.Run("reversed order: feature def overwrites bugfix def", func(t *testing.T) {
		_, agents := service.BuildSpawnerConfig(
			[]*model.Workflow{bugfixWf, featureWf},
			[]*model.AgentDefinition{bugfixDef, featureDef},
		)
		// featureDef is last → "opus_4_7" wins.
		if got := agents["implementor"].Model; got != "opus_4_7" {
			t.Errorf("expected last-written model %q, got %q", "opus_4_7", got)
		}
	})

	// Regression guard: single-workflow input always returns the correct model.
	t.Run("single workflow feature returns correct model", func(t *testing.T) {
		_, agents := service.BuildSpawnerConfig(
			[]*model.Workflow{featureWf},
			[]*model.AgentDefinition{featureDef},
		)
		if got := agents["implementor"].Model; got != "opus_4_7" {
			t.Errorf("implementor.Model = %q, want %q", got, "opus_4_7")
		}
		if got := agents["implementor"].Timeout; got != 3600 {
			t.Errorf("implementor.Timeout = %d, want 3600", got)
		}
	})

	t.Run("single workflow bugfix returns correct model", func(t *testing.T) {
		_, agents := service.BuildSpawnerConfig(
			[]*model.Workflow{bugfixWf},
			[]*model.AgentDefinition{bugfixDef},
		)
		if got := agents["implementor"].Model; got != "sonnet" {
			t.Errorf("implementor.Model = %q, want %q", got, "sonnet")
		}
		if got := agents["implementor"].Timeout; got != 1200 {
			t.Errorf("implementor.Timeout = %d, want 1200", got)
		}
	})
}

// TestBuildSpawnerConfigMultiAgentSingleWorkflow verifies all agents in a single
// workflow are loaded with correct config and no collision when IDs are distinct.
func TestBuildSpawnerConfigMultiAgentSingleWorkflow(t *testing.T) {
	wf := &model.Workflow{ID: "feature", ProjectID: "proj"}

	defs := []*model.AgentDefinition{
		{ID: "implementor", ProjectID: "proj", WorkflowID: "feature", Model: "opus_4_7", Timeout: 3600, Layer: 0},
		{ID: "verifier", ProjectID: "proj", WorkflowID: "feature", Model: "sonnet", Timeout: 1800, Tag: "qa", Layer: 1},
		{ID: "doc-updater", ProjectID: "proj", WorkflowID: "feature", Model: "haiku", Timeout: 900, Layer: 2},
	}

	_, agents := service.BuildSpawnerConfig([]*model.Workflow{wf}, defs)

	cases := []struct {
		id      string
		model   string
		timeout int
		tag     string
	}{
		{"implementor", "opus_4_7", 3600, ""},
		{"verifier", "sonnet", 1800, "qa"},
		{"doc-updater", "haiku", 900, ""},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			cfg, ok := agents[tc.id]
			if !ok {
				t.Fatalf("agent %q not found in config", tc.id)
			}
			if cfg.Model != tc.model {
				t.Errorf("Model = %q, want %q", cfg.Model, tc.model)
			}
			if cfg.Timeout != tc.timeout {
				t.Errorf("Timeout = %d, want %d", cfg.Timeout, tc.timeout)
			}
			if cfg.Tag != tc.tag {
				t.Errorf("Tag = %q, want %q", cfg.Tag, tc.tag)
			}
		})
	}
}

// TestBuildSpawnerConfigThreeWorkflowsCollision verifies that with N workflows sharing
// the same agent ID, only the last-written config survives — and that loading each
// workflow in isolation always returns the correct model.
func TestBuildSpawnerConfigThreeWorkflowsCollision(t *testing.T) {
	workflows := []*model.Workflow{
		{ID: "wf-a", ProjectID: "proj"},
		{ID: "wf-b", ProjectID: "proj"},
		{ID: "wf-c", ProjectID: "proj"},
	}
	defs := []*model.AgentDefinition{
		{ID: "implementor", ProjectID: "proj", WorkflowID: "wf-a", Model: "opus_4_7", Layer: 0},
		{ID: "implementor", ProjectID: "proj", WorkflowID: "wf-b", Model: "sonnet", Layer: 0},
		{ID: "implementor", ProjectID: "proj", WorkflowID: "wf-c", Model: "haiku", Layer: 0},
	}

	// Mixed: only the last entry survives.
	t.Run("mixed three workflows: last entry wins", func(t *testing.T) {
		_, agents := service.BuildSpawnerConfig(workflows, defs)
		if got := agents["implementor"].Model; got != "haiku" {
			t.Errorf("expected last-written model %q, got %q", "haiku", got)
		}
	})

	// Each individual workflow returns its own model.
	singles := []struct{ wf *model.Workflow; def *model.AgentDefinition; want string }{
		{workflows[0], defs[0], "opus_4_7"},
		{workflows[1], defs[1], "sonnet"},
		{workflows[2], defs[2], "haiku"},
	}
	for _, tc := range singles {
		tc := tc
		t.Run("single "+tc.wf.ID+" returns "+tc.want, func(t *testing.T) {
			_, agents := service.BuildSpawnerConfig(
				[]*model.Workflow{tc.wf},
				[]*model.AgentDefinition{tc.def},
			)
			if got := agents["implementor"].Model; got != tc.want {
				t.Errorf("implementor.Model = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStartReturnsErrorForUnknownWorkflow verifies that Start() returns a descriptive
// error when wfRepo.Get fails because the workflow name is not in the DB.
// This covers the new error path introduced by the fix (Get vs List).
func TestStartReturnsErrorForUnknownWorkflow(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "COLL-1", "Collision ticket")

	_, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "COLL-1",
		WorkflowName: "nonexistent-workflow",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent workflow, got nil")
	}

	wantPrefix := "workflow definition 'nonexistent-workflow' not found"
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Errorf("error = %q, want prefix %q", err.Error(), wantPrefix)
	}
}
