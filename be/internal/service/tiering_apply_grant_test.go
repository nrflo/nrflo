package service

import (
	"testing"

	"be/internal/types"
)

// TestApplyForProject_GrantsDelegationTools asserts a stock apply grants
// delegate,get_delegation to the three T0/T1 executor roles
// (setup-analyzer/test-writer/implementor) and leaves the two terminal T2
// roles (qa-verifier/doc-updater) with empty tools.
func TestApplyForProject_GrantsDelegationTools(t *testing.T) {
	t.Parallel()
	svc, pool := setupTieringApplyTestEnv(t)
	seedProjectAndWorkflow(t, pool, "grant", "feature", "ticket")

	roles := map[string]string{
		"setup-analyzer": "sonnet-5",
		"test-writer":    "opus-4-8",
		"implementor":    "opus-4-8",
		"qa-verifier":    "opus-4-8",
		"doc-updater":    "sonnet-5",
	}
	for defID, model := range roles {
		seedTieringDef(t, pool, tieringDefSeed{projectID: "grant", workflowID: "feature", defID: defID, model: model})
	}

	result, err := svc.ApplyForProject(types.TieringApplyConfirmation{ProjectID: "grant", ConfirmAll: true})
	if err != nil {
		t.Fatalf("ApplyForProject: %v", err)
	}
	for defID := range roles {
		outcome := findApplyOutcome(t, result, "feature", defID)
		if outcome.Outcome != "applied" {
			t.Errorf("%s outcome = %q, want applied", defID, outcome.Outcome)
		}
	}

	granted := []string{"setup-analyzer", "test-writer", "implementor"}
	for _, defID := range granted {
		tools := getAgentDefTools(t, pool, "grant", "feature", defID)
		if tools != "delegate,get_delegation" {
			t.Errorf("%s tools = %q, want delegate,get_delegation", defID, tools)
		}
	}

	terminal := []string{"qa-verifier", "doc-updater"}
	for _, defID := range terminal {
		tools := getAgentDefTools(t, pool, "grant", "feature", defID)
		if tools != "" {
			t.Errorf("%s tools = %q, want empty (terminal T2 role, no grant)", defID, tools)
		}
	}
}

// TestApplyForProject_HandPatchedToolsIdempotent is the ticket's required
// case: a def already carrying the delegation tools CSV AND already
// tier-driven at the recommended tier (as a stock apply leaves it) must
// re-apply as byte-identical "unchanged", never "skipped-customized", and
// never gain a duplicated tools entry.
func TestApplyForProject_HandPatchedToolsIdempotent(t *testing.T) {
	t.Parallel()
	svc, pool := setupTieringApplyTestEnv(t)
	seedProjectAndWorkflow(t, pool, "patched", "feature", "ticket")
	implTier := TierMap["implementor"].Tier
	seedTieringDef(t, pool, tieringDefSeed{
		projectID: "patched", workflowID: "feature", defID: "implementor",
		model: "", systemTemplateID: "tier-t1-executor",
		tools: "delegate,get_delegation", tier: &implTier,
	})

	_, _, _, updatedAtBefore := getAgentDefFields(t, pool, "patched", "feature", "implementor")

	result, err := svc.ApplyForProject(types.TieringApplyConfirmation{ProjectID: "patched", ConfirmAll: true})
	if err != nil {
		t.Fatalf("ApplyForProject: %v", err)
	}

	outcome := findApplyOutcome(t, result, "feature", "implementor")
	if outcome.Outcome != "unchanged" {
		t.Errorf("hand-patched implementor outcome = %q, want unchanged", outcome.Outcome)
	}

	tools := getAgentDefTools(t, pool, "patched", "feature", "implementor")
	if tools != "delegate,get_delegation" {
		t.Errorf("hand-patched implementor tools = %q, want byte-identical delegate,get_delegation (no dup)", tools)
	}

	_, _, _, updatedAtAfter := getAgentDefFields(t, pool, "patched", "feature", "implementor")
	if updatedAtAfter != updatedAtBefore {
		t.Errorf("updated_at changed on hand-patched unchanged apply: %q -> %q", updatedAtBefore, updatedAtAfter)
	}
}

// TestApplyForProject_GrantedButStaleModel asserts a def that already has
// the delegation tools CSV but a stale model still applies (model
// rewritten) without duplicating the tools CSV.
func TestApplyForProject_GrantedButStaleModel(t *testing.T) {
	t.Parallel()
	svc, pool := setupTieringApplyTestEnv(t)
	seedProjectAndWorkflow(t, pool, "stale", "feature", "ticket")
	seedTieringDef(t, pool, tieringDefSeed{
		projectID: "stale", workflowID: "feature", defID: "implementor",
		model: "opus-4-8", tools: "delegate,get_delegation",
	})

	result, err := svc.ApplyForProject(types.TieringApplyConfirmation{ProjectID: "stale", ConfirmAll: true})
	if err != nil {
		t.Fatalf("ApplyForProject: %v", err)
	}

	outcome := findApplyOutcome(t, result, "feature", "implementor")
	if outcome.Outcome != "applied" {
		t.Errorf("granted-but-stale implementor outcome = %q, want applied", outcome.Outcome)
	}

	model, _, _, _ := getAgentDefFields(t, pool, "stale", "feature", "implementor")
	if model != "" {
		t.Errorf("granted-but-stale implementor model = %q, want '' (tier-driven)", model)
	}
	tier := getAgentDefTier(t, pool, "stale", "feature", "implementor")
	if tier == nil || *tier != TierMap["implementor"].Tier {
		t.Errorf("granted-but-stale implementor tier = %v, want %d", tier, TierMap["implementor"].Tier)
	}
	tools := getAgentDefTools(t, pool, "stale", "feature", "implementor")
	if tools != "delegate,get_delegation" {
		t.Errorf("granted-but-stale implementor tools = %q, want delegate,get_delegation (no dup)", tools)
	}
}
