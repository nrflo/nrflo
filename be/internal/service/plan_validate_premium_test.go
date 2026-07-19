package service

import (
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestPlanModelTierClass_ClassificationTable pins the SINGLE tier classifier's
// name-class family match: opus/fable -> premium, haiku -> cheap, everything
// else (sonnet, gpt-*) -> mid.
func TestPlanModelTierClass_ClassificationTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id   string
		want ModelTier
	}{
		{"fable-5", ModelTierPremium},
		{"opus-4-8", ModelTierPremium},
		{"opus-4-8-1m", ModelTierPremium},
		{"haiku-4-5", ModelTierCheap},
		{"sonnet-5", ModelTierMid},
		{"gpt-5.6-sol", ModelTierMid},
		{"gpt-5.6-terra", ModelTierMid},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			got := PlanModelTierClass(&model.Model{ID: tc.id})
			if got != tc.want {
				t.Errorf("PlanModelTierClass(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

// TestLoadDynwfMaxPremiumWorkers covers the config cascade (project > global >
// default) plus the acceptance-critical difference from SubworkflowCap: 0 must
// be honored as a real cap, not floored back to the default.
func TestLoadDynwfMaxPremiumWorkers(t *testing.T) {
	t.Parallel()
	pool, projectID, _ := setupPlanValidateEnv(t)

	if got := LoadDynwfMaxPremiumWorkers(pool, projectID); got != DefaultDynwfMaxPremiumWorkers {
		t.Errorf("unset key: got %d, want default %d", got, DefaultDynwfMaxPremiumWorkers)
	}

	if err := pool.SetConfig(PremiumWorkerCapKey, "3"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if got := LoadDynwfMaxPremiumWorkers(pool, projectID); got != 3 {
		t.Errorf("global override: got %d, want 3", got)
	}

	if err := pool.SetProjectConfig(projectID, PremiumWorkerCapKey, "1"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}
	if got := LoadDynwfMaxPremiumWorkers(pool, projectID); got != 1 {
		t.Errorf("project override over global: got %d, want 1", got)
	}

	if err := pool.SetProjectConfig(projectID, PremiumWorkerCapKey, "0"); err != nil {
		t.Fatalf("SetProjectConfig(0): %v", err)
	}
	if got := LoadDynwfMaxPremiumWorkers(pool, projectID); got != 0 {
		t.Errorf("project override = 0: got %d, want 0 (must not floor to default, unlike SubworkflowCap)", got)
	}

	if err := pool.SetProjectConfig(projectID, PremiumWorkerCapKey, "not-a-number"); err != nil {
		t.Fatalf("SetProjectConfig(garbage): %v", err)
	}
	if got := LoadDynwfMaxPremiumWorkers(pool, projectID); got != DefaultDynwfMaxPremiumWorkers {
		t.Errorf("unparsable project value: got %d, want default %d", got, DefaultDynwfMaxPremiumWorkers)
	}

	if err := pool.SetProjectConfig(projectID, PremiumWorkerCapKey, "-1"); err != nil {
		t.Fatalf("SetProjectConfig(-1): %v", err)
	}
	if got := LoadDynwfMaxPremiumWorkers(pool, projectID); got != DefaultDynwfMaxPremiumWorkers {
		t.Errorf("negative project value: got %d, want default %d", got, DefaultDynwfMaxPremiumWorkers)
	}
}

// premiumHeavyManifest returns a 10-node, 3-dense-layer manifest with every
// node bound to premiumTemplate — layer 0 has 5 nodes, layer 1 has 4, and the
// final layer 2 has exactly 1 (the result-carrying node), mirroring the
// planner's own "final layer must have exactly one node" contract so the
// downgrade-retention assertion (last cap nodes kept) is meaningful.
func premiumHeavyManifest(premiumTemplate string) PlanManifest {
	mkNodes := func(prefix string, n int) []PlanNode {
		nodes := make([]PlanNode, n)
		for i := 0; i < n; i++ {
			nodes[i] = PlanNode{ID: prefix + string(rune('a'+i)), Template: premiumTemplate, Instructions: "do work"}
		}
		return nodes
	}
	return PlanManifest{
		Version: 1,
		Goal:    "premium heavy",
		Layers: []PlanLayer{
			{Layer: 0, Policy: "all", Nodes: mkNodes("l0", 5)},
			{Layer: 1, Policy: "all", Nodes: mkNodes("l1", 4)},
			{Layer: 2, Policy: "any", Nodes: mkNodes("l2", 1)},
		},
	}
}

// TestEnforcePremiumWorkerCap_CheapOnlyManifest_UnchangedForBothCanRevise is
// acceptance #1: a plan that never exceeds the premium cap is accepted
// unmodified regardless of canRevise.
func TestEnforcePremiumWorkerCap_CheapOnlyManifest_UnchangedForBothCanRevise(t *testing.T) {
	t.Parallel()
	for _, canRevise := range []bool{true, false} {
		t.Run(map[bool]string{true: "canRevise=true", false: "canRevise=false"}[canRevise], func(t *testing.T) {
			t.Parallel()
			pool, projectID, workflowID := setupPlanValidateEnv(t)
			m := baseValidManifest("worker") // sonnet-5, mid tier -> never premium

			got, warning, err := EnforcePremiumWorkerCap(pool, clock.Real(), projectID, workflowID, m, canRevise)
			if err != nil {
				t.Fatalf("EnforcePremiumWorkerCap: %v", err)
			}
			if warning != "" {
				t.Errorf("warning = %q, want empty", warning)
			}
			for li, layer := range got.Layers {
				for ni, node := range layer.Nodes {
					if node.Template != "worker" {
						t.Errorf("Layers[%d].Nodes[%d].Template = %q, want unchanged %q", li, ni, node.Template, "worker")
					}
				}
			}
		})
	}
}

// TestEnforcePremiumWorkerCap_OverCap_CanRevise_RejectsNamingOffenders covers
// the interactive-approval reject path: canRevise=true over cap must error
// naming the offending node ids and the configured cap.
func TestEnforcePremiumWorkerCap_OverCap_CanRevise_RejectsNamingOffenders(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)
	insertFanoutTemplate(t, pool, projectID, workflowID, "opus-worker", "opus-4-8", "cli_interactive")
	m := premiumHeavyManifest("opus-worker")

	_, warning, err := EnforcePremiumWorkerCap(pool, clock.Real(), projectID, workflowID, m, true)
	if err == nil {
		t.Fatal("expected a reject error for a 10-premium-node manifest over the default cap of 2, got nil")
	}
	if warning != "" {
		t.Errorf("warning = %q, want empty on the reject path", warning)
	}
	if !strings.Contains(err.Error(), PremiumWorkerCapKey) {
		t.Errorf("error %q does not mention %q", err.Error(), PremiumWorkerCapKey)
	}
	for _, id := range []string{"l0a", "l0b", "l0c", "l0d", "l0e", "l1a", "l1b", "l1c", "l1d", "l2a"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("error %q does not name offending node %q", err.Error(), id)
		}
	}
}

// TestEnforcePremiumWorkerCap_OverCap_CanReviseFalse_DowngradesRetainingLastNodes
// covers the mode=auto downgrade path: the earliest excess premium nodes are
// rebound to the cheapest non-premium template, and the LAST maxPremium
// premium refs are retained — the final (result-carrying) node keeps its
// tier.
func TestEnforcePremiumWorkerCap_OverCap_CanReviseFalse_DowngradesRetainingLastNodes(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)
	insertFanoutTemplate(t, pool, projectID, workflowID, "opus-worker", "opus-4-8", "cli_interactive")
	m := premiumHeavyManifest("opus-worker")

	got, warning, err := EnforcePremiumWorkerCap(pool, clock.Real(), projectID, workflowID, m, false)
	if err != nil {
		t.Fatalf("EnforcePremiumWorkerCap: %v", err)
	}
	if warning == "" {
		t.Fatal("expected a non-empty warning for an auto-downgraded manifest")
	}
	if !strings.Contains(warning, PremiumWorkerCapKey) {
		t.Errorf("warning %q does not mention %q", warning, PremiumWorkerCapKey)
	}

	wantTemplate := map[string]string{
		"l0a": "worker", "l0b": "worker", "l0c": "worker", "l0d": "worker", "l0e": "worker",
		"l1a": "worker", "l1b": "worker", "l1c": "worker",
		"l1d": "opus-worker", // last cap(=2) premium refs retained, in manifest order
		"l2a": "opus-worker", // ... including the final result-carrying node
	}
	for _, layer := range got.Layers {
		for _, node := range layer.Nodes {
			want, ok := wantTemplate[node.ID]
			if !ok {
				t.Fatalf("unexpected node id %q in downgraded manifest", node.ID)
			}
			if node.Template != want {
				t.Errorf("node %q Template = %q, want %q", node.ID, node.Template, want)
			}
		}
	}
	if got.Layers[2].Nodes[0].Template != "opus-worker" {
		t.Errorf("final layer node Template = %q, want opus-worker (result node must keep its tier)", got.Layers[2].Nodes[0].Template)
	}
}

// TestEnforcePremiumWorkerCap_CapOverride_ZeroDowngradesAllPremium confirms
// LoadDynwfMaxPremiumWorkers' cap=0 support flows through: with the cap set
// to 0, every premium node (including the final one) is downgraded.
func TestEnforcePremiumWorkerCap_CapOverride_ZeroDowngradesAllPremium(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)
	insertFanoutTemplate(t, pool, projectID, workflowID, "opus-worker", "opus-4-8", "cli_interactive")
	if err := pool.SetProjectConfig(projectID, PremiumWorkerCapKey, "0"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}
	m := premiumHeavyManifest("opus-worker")

	got, warning, err := EnforcePremiumWorkerCap(pool, clock.Real(), projectID, workflowID, m, false)
	if err != nil {
		t.Fatalf("EnforcePremiumWorkerCap: %v", err)
	}
	if warning == "" {
		t.Fatal("expected a non-empty warning")
	}
	for _, layer := range got.Layers {
		for _, node := range layer.Nodes {
			if node.Template != "worker" {
				t.Errorf("cap=0: node %q Template = %q, want worker (no premium node may survive)", node.ID, node.Template)
			}
		}
	}
}

// TestEnforcePremiumWorkerCap_CapOverride_OneRetainsOnlyFinalNode confirms a
// project-level cap of 1 keeps exactly the final result node premium and
// downgrades every other premium node.
func TestEnforcePremiumWorkerCap_CapOverride_OneRetainsOnlyFinalNode(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)
	insertFanoutTemplate(t, pool, projectID, workflowID, "opus-worker", "opus-4-8", "cli_interactive")
	if err := pool.SetProjectConfig(projectID, PremiumWorkerCapKey, "1"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}
	m := premiumHeavyManifest("opus-worker")

	got, _, err := EnforcePremiumWorkerCap(pool, clock.Real(), projectID, workflowID, m, false)
	if err != nil {
		t.Fatalf("EnforcePremiumWorkerCap: %v", err)
	}
	if got.Layers[2].Nodes[0].Template != "opus-worker" {
		t.Errorf("final node Template = %q, want opus-worker", got.Layers[2].Nodes[0].Template)
	}
	if got.Layers[1].Nodes[3].Template != "worker" {
		t.Errorf("last-of-layer-1 node Template = %q, want worker (cap=1 keeps only the final node)", got.Layers[1].Nodes[3].Template)
	}
}

// TestEnforcePremiumWorkerCap_NoNonPremiumTemplateAvailable_ErrorsNotPanics
// covers the downgrade path when the template library holds nothing but
// premium templates: cheapestNonPremiumTemplate must return an error, and
// EnforcePremiumWorkerCap must propagate it rather than panicking.
func TestEnforcePremiumWorkerCap_NoNonPremiumTemplateAvailable_ErrorsNotPanics(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)
	// Downgrade the only existing template ("worker") itself to premium so the
	// library holds nothing but premium-tier templates.
	mustExec(t, pool, `UPDATE agent_definitions SET model = 'opus-4-8' WHERE id = 'worker'`)
	m := premiumHeavyManifest("worker")

	_, warning, err := EnforcePremiumWorkerCap(pool, clock.Real(), projectID, workflowID, m, false)
	if err == nil {
		t.Fatal("expected an error when no non-premium template is available to downgrade to")
	}
	if warning != "" {
		t.Errorf("warning = %q, want empty on the error path", warning)
	}
}
