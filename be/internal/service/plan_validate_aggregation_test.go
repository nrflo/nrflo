package service

import (
	"strings"
	"testing"
)

// ValidatePlanManifest must aggregate every violation into one error rather
// than failing fast on the first problem found, so a planner agent can fix
// everything in a single retry.
func TestValidatePlanManifest_AggregatesMultipleViolations(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)

	m := PlanManifest{
		Version: 2,  // violation 1: wrong version
		Goal:    "", // violation 2: empty goal
		Layers: []PlanLayer{
			{
				Layer:  0,
				Policy: "all",
				Nodes: []PlanNode{
					{ID: "_bad id", Template: "", Instructions: ""}, // violations 3-6 below
				},
			},
		},
	}

	err := ValidatePlanManifest(pool, projectID, workflowID, m)
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	msg := err.Error()

	wantSubstrings := []string{
		"version must be 1",
		"goal is required",
		"must not start with '_'", // node id underscore-prefix
		"must match ^[a-z0-9]",    // node id regex mismatch ("_bad id" has a space too)
		"template is required",
		"instructions are required",
		// the final (only) layer has a node, so no "must have at least one
		// node" here, but it does need exactly one node to be the
		// result-carrying node — it already has one, so that's not a
		// violation in this case. The count above (6 distinct problems) is
		// enough to prove aggregation, not fail-fast.
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(msg, want) {
			t.Errorf("expected aggregated error to contain %q; full error:\n%s", want, msg)
		}
	}

	// Fail-fast would stop at the first problem (e.g. "version must be 1")
	// and never reach the rest — assert the error carries multiple lines.
	if lines := strings.Split(msg, "\n"); len(lines) < 3 {
		t.Fatalf("expected multiple aggregated problem lines, got %d line(s):\n%s", len(lines), msg)
	}
}

// A second, independently-constructed scenario exercising a different
// combination of simultaneous violations (cap + reference + template rules)
// to prove aggregation isn't specific to the structural checks alone.
func TestValidatePlanManifest_AggregatesCapAndRefAndTemplateViolations(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)

	m := PlanManifest{
		Version: 1,
		Goal:    "many problems at once",
		Layers: []PlanLayer{
			{
				Layer:  0,
				Policy: "all",
				Nodes: []PlanNode{
					{ID: "a", Template: "does-not-exist", Instructions: "reference #{NODE_FINDINGS:a}"}, // same-layer self-ref + unknown template
				},
			},
			{
				Layer:  1,
				Policy: "all",
				Nodes: []PlanNode{
					{ID: "b", Template: "worker", Instructions: "reference #{NODE_FINDINGS:nonexistent}"}, // unknown ref
					{ID: "c", Template: "worker", Instructions: "second node in final layer"},             // final layer has 2 nodes
				},
			},
		},
	}

	err := ValidatePlanManifest(pool, projectID, workflowID, m)
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	msg := err.Error()

	wantSubstrings := []string{
		"must have exactly one node",
		"unknown template",
		"is not a node declared anywhere in this plan",
		"references must target a strictly earlier layer",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(msg, want) {
			t.Errorf("expected aggregated error to contain %q; full error:\n%s", want, msg)
		}
	}
}
