package service

import "strings"

// TierTarget is the recommended re-tier target for one workflow-phase role,
// plus the original seed model used to detect per-def customization.
type TierTarget struct {
	Role              string
	OriginalSeedModel string // pre-retier seed default (post-000138/000168 model-ref rewrites)
	RecommendedModel  string
	RecommendedEffort string // "" = do not force an override (inherit the model's default)
	SystemTemplateID  string // resolves to a type='injectable' default_templates row
	IsWorker          bool
}

// TierMap is the canonical role -> recommendation table. It is the single
// source of truth consumed by the tiering report, the tiering apply flow,
// and new-project seeding (project_seed.go).
var TierMap = map[string]TierTarget{
	"setup-analyzer": {
		Role: "setup-analyzer", OriginalSeedModel: "sonnet-5",
		RecommendedModel: "sonnet-5", RecommendedEffort: "low",
		SystemTemplateID: "tier-t2-extractor", IsWorker: true,
	},
	"test-writer": {
		Role: "test-writer", OriginalSeedModel: "opus-4-8",
		RecommendedModel: "sonnet-5", RecommendedEffort: "medium",
		SystemTemplateID: "tier-t1-executor", IsWorker: true,
	},
	"implementor": {
		Role: "implementor", OriginalSeedModel: "opus-4-8",
		RecommendedModel: "sonnet-5", RecommendedEffort: "medium",
		SystemTemplateID: "tier-t1-executor", IsWorker: true,
	},
	"qa-verifier": {
		Role: "qa-verifier", OriginalSeedModel: "opus-4-8",
		RecommendedModel: "sonnet-5", RecommendedEffort: "low",
		SystemTemplateID: "tier-t2-extractor", IsWorker: true,
	},
	"doc-updater": {
		Role: "doc-updater", OriginalSeedModel: "sonnet-5",
		RecommendedModel: "haiku-4-5", RecommendedEffort: "low",
		SystemTemplateID: "tier-t1-executor", IsWorker: true,
	},
}

// roleSynonyms maps additional def-id name patterns (seen across older
// project seeds and hand-authored workflows) onto the canonical role keys in
// TierMap. Canonical ids match themselves via the substring check below and
// do not need an entry here.
//
//	setup-analyzer -> plan, analyze, analyzer
//	test-writer    -> write-tests, test-writing
//	implementor    -> implement, implementation
//	qa-verifier    -> review, verify, reviewer
//	doc-updater    -> docs, document, documentation
var roleSynonyms = map[string][]string{
	"setup-analyzer": {"setup-analyzer", "plan", "analyze", "analyzer"},
	"test-writer":    {"test-writer", "write-tests", "test-writing"},
	"implementor":    {"implementor", "implement", "implementation"},
	"qa-verifier":    {"qa-verifier", "review", "verify", "reviewer"},
	"doc-updater":    {"doc-updater", "docs", "document", "documentation"},
}

// roleOrder fixes classification precedence so overlapping synonyms (e.g. a
// def id containing both "review" and "doc") resolve deterministically.
var roleOrder = []string{"setup-analyzer", "test-writer", "implementor", "qa-verifier", "doc-updater"}

// ClassifyRole maps an agent_definitions row to a TierMap role by def-id name
// pattern (see roleSynonyms). workflowID is accepted for symmetry with
// callers that need it for workflow-level eligibility rules (e.g. the hotfix
// exemption below) but does not itself affect classification: role
// classification is purely a def-id name match.
func ClassifyRole(workflowID, defID string) (role string, ok bool) {
	_ = workflowID
	d := strings.ToLower(strings.TrimSpace(defID))
	if d == "" {
		return "", false
	}
	for _, r := range roleOrder {
		for _, pattern := range roleSynonyms[r] {
			if strings.Contains(d, pattern) {
				return r, true
			}
		}
	}
	return "", false
}

// isTierCustomized reports whether a def's current model indicates a
// hand-customization that apply must skip. A model still at the original seed
// value is stock; a model already at the recommended value is already-tiered
// (so a repeat apply is idempotent "unchanged", not a "customized" skip);
// anything else is a genuine customization flagged for manual review. Shared
// by the report and apply flows so both agree on what "customized" means.
func isTierCustomized(model string, target TierTarget) bool {
	return !strings.EqualFold(model, target.OriginalSeedModel) &&
		!strings.EqualFold(model, target.RecommendedModel)
}

// IsHotfixImplementor reports whether (workflowID, role) is the hotfix
// workflow's implementor. Per ticket ("urgency over cost"), the hotfix
// implementor is deliberately excluded from re-tiering: seeding still gives
// it the tier map's implementor model, but apply/report never touch it and
// never force its reasoning effort.
func IsHotfixImplementor(workflowID, role string) bool {
	return strings.EqualFold(strings.TrimSpace(workflowID), "hotfix") && role == "implementor"
}
