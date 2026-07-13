package service

import (
	"strings"
	"testing"
)

func TestValidatePlanManifest_Accept(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		manifest func() PlanManifest
	}{
		{"minimal two-layer manifest", func() PlanManifest {
			return baseValidManifest("worker")
		}},
		{"single final-only layer", func() PlanManifest {
			return PlanManifest{
				Version: 1,
				Goal:    "one shot",
				Layers: []PlanLayer{
					{Layer: 0, Policy: "all", Nodes: []PlanNode{
						{ID: "solo", Template: "worker", Instructions: "do the whole thing"},
					}},
				},
			}
		}},
		{"questions populated never block", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Questions = []PlanQuestion{
				{ID: "q1", Question: "should we touch the auth module?"},
				{ID: "q2", Question: "any perf budget?"},
			}
			return m
		}},
		{"quorum policy on non-final layer", func() PlanManifest {
			m := PlanManifest{
				Version: 1,
				Goal:    "fan out then converge",
				Layers: []PlanLayer{
					{Layer: 0, Policy: "quorum:2", Nodes: []PlanNode{
						{ID: "a", Template: "worker", Instructions: "one"},
						{ID: "b", Template: "worker", Instructions: "two"},
						{ID: "c", Template: "worker", Instructions: "three"},
					}},
					{Layer: 1, Policy: "any", Nodes: []PlanNode{
						{ID: "converge", Template: "worker", Instructions: "combine #{NODE_FINDINGS:a} and #{NODE_FINDINGS:b}"},
					}},
				},
			}
			return m
		}},
		{"percent policy", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[0].Policy = "percent:80"
			return m
		}},
		{"instructions exactly at MaxInstructionBytes cap", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[0].Nodes[0].Instructions = strings.Repeat("a", DefaultPlanMaxInstructionBytes)
			return m
		}},
		{"node id at max length (64 chars)", func() PlanManifest {
			m := baseValidManifest("worker")
			longID := "a" + strings.Repeat("b", 63) // 64 chars total
			m.Layers[0].Nodes[0].ID = longID
			m.Layers[1].Nodes[0].Instructions = "fix using #{NODE_FINDINGS:" + longID + "}"
			return m
		}},
		{"node id with digits, dash and underscore", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[0].Nodes[0].ID = "step-1_a"
			m.Layers[1].Nodes[0].Instructions = "fix using #{NODE_FINDINGS:step-1_a}"
			return m
		}},
		{"keyed NODE_FINDINGS reference form", func() PlanManifest {
			m := baseValidManifest("worker")
			m.Layers[1].Nodes[0].Instructions = "fix using #{NODE_FINDINGS:investigate:root_cause,severity}"
			return m
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool, projectID, workflowID := setupPlanValidateEnv(t)
			if err := ValidatePlanManifest(pool, projectID, workflowID, tc.manifest()); err != nil {
				t.Fatalf("expected success, got error: %v", err)
			}
		})
	}
}
