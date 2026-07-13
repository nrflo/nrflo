package service

import (
	"strings"
	"testing"
)

// TestValidatePlanManifest_NodeFindingsRefs covers the #{NODE_FINDINGS:...}
// cross-layer reference matrix: strictly-earlier-layer references are
// accepted, same-layer/later-layer/unknown-node references are rejected, and
// the keyed form #{NODE_FINDINGS:x:k1,k2} resolves identically to the base
// form (the capture group ignores the ":k1,k2" suffix).
func TestValidatePlanManifest_NodeFindingsRefs(t *testing.T) {
	t.Parallel()

	threeLayerManifest := func(refInstructions string) PlanManifest {
		return PlanManifest{
			Version: 1,
			Goal:    "multi-layer refs",
			Layers: []PlanLayer{
				{Layer: 0, Policy: "all", Nodes: []PlanNode{
					{ID: "a", Template: "worker", Instructions: "layer0 work"},
				}},
				{Layer: 1, Policy: "all", Nodes: []PlanNode{
					{ID: "b", Template: "worker", Instructions: refInstructions},
				}},
				{Layer: 2, Policy: "any", Nodes: []PlanNode{
					{ID: "c", Template: "worker", Instructions: "final layer work"},
				}},
			},
		}
	}

	cases := []struct {
		name    string
		refIns  string
		wantErr string // "" = accepted
	}{
		{
			name:    "reference to strictly earlier layer is accepted",
			refIns:  "use #{NODE_FINDINGS:a}",
			wantErr: "",
		},
		{
			name:    "keyed reference form resolves the same as the base form",
			refIns:  "use #{NODE_FINDINGS:a:root_cause,severity}",
			wantErr: "",
		},
		{
			name:    "same-layer reference is rejected",
			refIns:  "use #{NODE_FINDINGS:b}", // b references itself, same layer (1)
			wantErr: "references must target a strictly earlier layer",
		},
		{
			name:    "forward (later-layer) reference is rejected",
			refIns:  "use #{NODE_FINDINGS:c}", // c is in layer 2, later than b's layer 1
			wantErr: "references must target a strictly earlier layer",
		},
		{
			name:    "reference to unknown node id is rejected",
			refIns:  "use #{NODE_FINDINGS:ghost}",
			wantErr: "is not a node declared anywhere in this plan",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool, projectID, workflowID := setupPlanValidateEnv(t)
			err := ValidatePlanManifest(pool, projectID, workflowID, threeLayerManifest(tc.refIns))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// A node can reference multiple earlier nodes, and multiple references in
// one instructions string are all checked independently.
func TestValidatePlanManifest_MultipleRefsInOneNode(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)

	m := PlanManifest{
		Version: 1,
		Goal:    "combine two upstream findings",
		Layers: []PlanLayer{
			{Layer: 0, Policy: "all", Nodes: []PlanNode{
				{ID: "a", Template: "worker", Instructions: "investigate a"},
				{ID: "b", Template: "worker", Instructions: "investigate b"},
			}},
			{Layer: 1, Policy: "any", Nodes: []PlanNode{
				{ID: "combine", Template: "worker", Instructions: "merge #{NODE_FINDINGS:a} with #{NODE_FINDINGS:b:key1}"},
			}},
		},
	}
	if err := ValidatePlanManifest(pool, projectID, workflowID, m); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}
