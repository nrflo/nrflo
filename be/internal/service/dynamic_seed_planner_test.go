package service

import (
	"strings"
	"testing"
)

// TestDynPlannerPrompt_CarriesTierPolicyDirectives guards the soft,
// prompt-level complement to the server-side EnforcePremiumWorkerCap
// guardrail: the planner must be steered to default cheap, fan out cheap,
// verify at sonnet-low, and reserve premium for final adjudication only,
// under the server-enforced cap.
func TestDynPlannerPrompt_CarriesTierPolicyDirectives(t *testing.T) {
	t.Parallel()

	if !strings.Contains(dynPlannerPrompt, "## Tier Policy") {
		t.Fatal("dynPlannerPrompt is missing the ## Tier Policy section")
	}

	wantSubstrings := []string{
		"Default every worker node to the cheap tier",
		"Fan-out/per-file/per-item sweep nodes are ALWAYS cheap tier",
		"Reserve at most one synthesis/judge node at mid tier",
		"Premium tier (opus/fable) is reserved for final adjudication only",
		"Verification nodes belong at sonnet-low",
		PremiumWorkerCapKey,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(dynPlannerPrompt, want) {
			t.Errorf("dynPlannerPrompt does not contain expected tier-policy directive: %q", want)
		}
	}

	// The Tier Policy section must land before the manifest schema contract,
	// matching the anchor migration 000175 REPLACEs into existing installs.
	tierIdx := strings.Index(dynPlannerPrompt, "## Tier Policy")
	schemaIdx := strings.Index(dynPlannerPrompt, "## Manifest Schema (version 1)")
	if tierIdx == -1 || schemaIdx == -1 || tierIdx >= schemaIdx {
		t.Errorf("expected ## Tier Policy (idx %d) to precede ## Manifest Schema (idx %d)", tierIdx, schemaIdx)
	}
}
