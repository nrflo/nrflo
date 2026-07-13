package service

import (
	"strings"
	"testing"
)

func TestValidatePlanManifest_Reject(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		manifest func() PlanManifest
		wantErr  string
	}{
		{"version not 1", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Version = 2
			return m
		}, "version must be 1"},
		{"empty goal", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Goal = "   "
			return m
		}, "goal is required"},
		{"no layers", func() PlanManifest {
			return PlanManifest{Version: 1, Goal: "g", Layers: nil}
		}, "at least one layer is required"},
		{"too many layers", func() PlanManifest {
			var layers []PlanLayer
			for i := 0; i < DefaultPlanMaxLayers+1; i++ {
				layers = append(layers, PlanLayer{
					Layer: i, Policy: "all",
					Nodes: []PlanNode{{ID: "n" + string(rune('a'+i)), Template: "worker", Instructions: "do it"}},
				})
			}
			return PlanManifest{Version: 1, Goal: "g", Layers: layers}
		}, "too many layers"},
		{"too many questions", func() PlanManifest {
			m := baseValidManifest("worker")
			for i := 0; i < DefaultPlanMaxQuestions+1; i++ {
				m.Questions = append(m.Questions, PlanQuestion{ID: "q" + string(rune('a'+i)), Question: "why?"})
			}
			return m
		}, "too many questions"},
		{"sparse layers (0 and 2, missing 1)", func() PlanManifest {
			return PlanManifest{
				Version: 1, Goal: "g",
				Layers: []PlanLayer{
					{Layer: 0, Policy: "all", Nodes: []PlanNode{{ID: "a", Template: "worker", Instructions: "x"}}},
					{Layer: 2, Policy: "all", Nodes: []PlanNode{{ID: "b", Template: "worker", Instructions: "y"}}},
				},
			}
		}, "must be dense and 0-indexed"},
		{"empty layer (0 nodes)", func() PlanManifest {
			return PlanManifest{
				Version: 1, Goal: "g",
				Layers: []PlanLayer{
					{Layer: 0, Policy: "all", Nodes: nil},
				},
			}
		}, "must have at least one node"},
		{"invalid layer policy", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[0].Policy = "quorum:99"
			return m
		}, "invalid policy"},
		{"final layer has two nodes", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[1].Nodes = append(m.Layers[1].Nodes, PlanNode{ID: "fix2", Template: "worker", Instructions: "also fix"})
			m.Layers[1].Policy = "all"
			return m
		}, "final layer (1) must have exactly one node"},
		{"node id underscore-prefixed fails both regex and prefix checks", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[0].Nodes[0].ID = "_foo"
			m.Layers[1].Nodes[0].Instructions = "fix using #{NODE_FINDINGS:_foo}"
			return m
		}, "must not start with '_'"},
		{"node id uppercase fails regex", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[0].Nodes[0].ID = "Investigate"
			m.Layers[1].Nodes[0].Instructions = "fix using #{NODE_FINDINGS:Investigate}"
			return m
		}, "must match ^[a-z0-9]"},
		{"duplicate node id across layers", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[1].Nodes[0].ID = "investigate"
			m.Layers[1].Nodes[0].Instructions = "no self ref needed"
			return m
		}, `duplicate node id "investigate"`},
		{"instructions empty", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[0].Nodes[0].Instructions = "   "
			return m
		}, "instructions are required"},
		{"instructions over MaxInstructionBytes cap", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[0].Nodes[0].Instructions = strings.Repeat("a", DefaultPlanMaxInstructionBytes+1)
			return m
		}, "instructions exceed"},
		{"template empty", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[0].Nodes[0].Template = "  "
			return m
		}, "template is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool, projectID, workflowID := setupPlanValidateEnv(t)
			err := ValidatePlanManifest(pool, projectID, workflowID, tc.manifest())
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// The "too many nodes" cap is independent of the "too many layers" cap: stay
// within MaxLayers while exceeding MaxNodes across those layers.
func TestValidatePlanManifest_Reject_TooManyNodes(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)

	perLayer := 10
	m := PlanManifest{Version: 1, Goal: "big fanout"}
	for l := 0; l < DefaultPlanMaxLayers-1; l++ {
		var nodes []PlanNode
		for n := 0; n < perLayer; n++ {
			nodes = append(nodes, PlanNode{
				ID:           string(rune('a'+l)) + "node" + string(rune('a'+n)),
				Template:     "worker",
				Instructions: "do work",
			})
		}
		m.Layers = append(m.Layers, PlanLayer{Layer: l, Policy: "all", Nodes: nodes})
	}
	// final layer: exactly one node, dense index.
	m.Layers = append(m.Layers, PlanLayer{
		Layer: DefaultPlanMaxLayers - 1, Policy: "any",
		Nodes: []PlanNode{{ID: "final", Template: "worker", Instructions: "wrap up"}},
	})

	err := ValidatePlanManifest(pool, projectID, workflowID, m)
	if err == nil {
		t.Fatal("expected too-many-nodes error, got nil")
	}
	if !strings.Contains(err.Error(), "too many nodes") {
		t.Fatalf("expected error containing %q, got: %v", "too many nodes", err)
	}
	if strings.Contains(err.Error(), "too many layers") {
		t.Fatalf("did not expect a layers-cap violation in this scenario, got: %v", err)
	}
}
