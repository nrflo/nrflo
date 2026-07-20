package service

import "testing"

// TestTierMap_MatchesTicket asserts TierMap's per-role recommendation matches
// the ticket's tier map exactly: setup-analyzer/qa-verifier -> sonnet-5/low
// (tier-t2-extractor), test-writer/implementor -> sonnet-5/medium
// (tier-t1-executor), doc-updater -> haiku-4-5/low (tier-t1-executor).
func TestTierMap_MatchesTicket(t *testing.T) {
	t.Parallel()
	cases := []struct {
		role                                              string
		wantOriginal, wantModel, wantEffort, wantTemplate string
		wantWorker, wantGrants                            bool
	}{
		{"setup-analyzer", "sonnet-5", "sonnet-5", "low", "tier-t2-extractor", true, true},
		{"test-writer", "opus-4-8", "sonnet-5", "medium", "tier-t1-executor", true, true},
		{"implementor", "opus-4-8", "sonnet-5", "medium", "tier-t1-executor", true, true},
		{"qa-verifier", "opus-4-8", "sonnet-5", "low", "tier-t2-extractor", true, false},
		{"doc-updater", "sonnet-5", "haiku-4-5", "low", "tier-t1-executor", true, false},
	}
	for _, c := range cases {
		t.Run(c.role, func(t *testing.T) {
			target, ok := TierMap[c.role]
			if !ok {
				t.Fatalf("TierMap missing role %q", c.role)
			}
			if target.OriginalSeedModel != c.wantOriginal {
				t.Errorf("OriginalSeedModel = %q, want %q", target.OriginalSeedModel, c.wantOriginal)
			}
			if target.RecommendedModel != c.wantModel {
				t.Errorf("RecommendedModel = %q, want %q", target.RecommendedModel, c.wantModel)
			}
			if target.RecommendedEffort != c.wantEffort {
				t.Errorf("RecommendedEffort = %q, want %q", target.RecommendedEffort, c.wantEffort)
			}
			if target.SystemTemplateID != c.wantTemplate {
				t.Errorf("SystemTemplateID = %q, want %q", target.SystemTemplateID, c.wantTemplate)
			}
			if target.IsWorker != c.wantWorker {
				t.Errorf("IsWorker = %v, want %v", target.IsWorker, c.wantWorker)
			}
			if target.GrantsDelegation != c.wantGrants {
				t.Errorf("GrantsDelegation = %v, want %v", target.GrantsDelegation, c.wantGrants)
			}
		})
	}
}

func TestClassifyRole(t *testing.T) {
	t.Parallel()
	cases := []struct {
		defID    string
		wantRole string
		wantOK   bool
	}{
		// Canonical ids.
		{"setup-analyzer", "setup-analyzer", true},
		{"test-writer", "test-writer", true},
		{"implementor", "implementor", true},
		{"qa-verifier", "qa-verifier", true},
		{"doc-updater", "doc-updater", true},
		// Synonyms.
		{"plan", "setup-analyzer", true},
		{"analyze", "setup-analyzer", true},
		{"analyzer", "setup-analyzer", true},
		{"write-tests", "test-writer", true},
		{"test-writing", "test-writer", true},
		{"implement", "implementor", true},
		{"implementation", "implementor", true},
		{"review", "qa-verifier", true},
		{"verify", "qa-verifier", true},
		{"reviewer", "qa-verifier", true},
		{"docs", "doc-updater", true},
		{"document", "doc-updater", true},
		{"documentation", "doc-updater", true},
		// Case-insensitive.
		{"IMPLEMENT", "implementor", true},
		// Unmapped.
		{"spec-normalizer", "", false},
		{"", "", false},
		{"   ", "", false},
	}
	for _, c := range cases {
		t.Run(c.defID, func(t *testing.T) {
			role, ok := ClassifyRole("feature", c.defID)
			if ok != c.wantOK {
				t.Fatalf("ClassifyRole(%q) ok = %v, want %v", c.defID, ok, c.wantOK)
			}
			if role != c.wantRole {
				t.Errorf("ClassifyRole(%q) role = %q, want %q", c.defID, role, c.wantRole)
			}
		})
	}
}

// TestClassifyRole_Precedence pins the deterministic tie-break when a def id
// matches multiple synonym patterns: roleOrder resolves it, not map iteration.
func TestClassifyRole_Precedence(t *testing.T) {
	t.Parallel()
	// "review-docs" contains both "review" (qa-verifier) and "docs"
	// (doc-updater); qa-verifier precedes doc-updater in roleOrder.
	role, ok := ClassifyRole("feature", "review-docs")
	if !ok || role != "qa-verifier" {
		t.Errorf("ClassifyRole(review-docs) = (%q, %v), want (qa-verifier, true)", role, ok)
	}
}

func TestIsHotfixImplementor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		workflowID, role string
		want             bool
	}{
		{"hotfix", "implementor", true},
		{"HOTFIX", "implementor", true},
		{" hotfix ", "implementor", true},
		{"hotfix", "qa-verifier", false},
		{"feature", "implementor", false},
		{"bugfix", "implementor", false},
	}
	for _, c := range cases {
		if got := IsHotfixImplementor(c.workflowID, c.role); got != c.want {
			t.Errorf("IsHotfixImplementor(%q, %q) = %v, want %v", c.workflowID, c.role, got, c.want)
		}
	}
}
