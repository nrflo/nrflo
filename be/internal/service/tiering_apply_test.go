package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

func setupTieringApplyTestEnv(t *testing.T) (*TieringService, *db.Pool) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tiering_apply_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	clk := clock.NewTest(time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC))
	return NewTieringService(pool, clk, NewModelService(pool, clk)), pool
}

// TestApplyForProject_Idempotent applies a mix of eligible defs with
// ConfirmAll, then applies again with the same confirmation: the first pass
// must write model/effort/template, the second must report every def
// "unchanged" and re-write nothing (updated_at stable).
func TestApplyForProject_Idempotent(t *testing.T) {
	t.Parallel()
	svc, pool := setupTieringApplyTestEnv(t)
	seedProjectAndWorkflow(t, pool, "idem", "feature", "ticket")
	seedTieringDef(t, pool, tieringDefSeed{projectID: "idem", workflowID: "feature", defID: "implementor", model: "opus-4-8"})
	seedTieringDef(t, pool, tieringDefSeed{projectID: "idem", workflowID: "feature", defID: "doc-updater", model: "sonnet-5"})

	confirmation := types.TieringApplyConfirmation{ProjectID: "idem", ConfirmAll: true}

	first, err := svc.ApplyForProject(confirmation)
	if err != nil {
		t.Fatalf("first ApplyForProject: %v", err)
	}
	implOutcome := findApplyOutcome(t, first, "feature", "implementor")
	if implOutcome.Outcome != "applied" {
		t.Errorf("first pass implementor outcome = %q, want applied", implOutcome.Outcome)
	}
	docOutcome := findApplyOutcome(t, first, "feature", "doc-updater")
	if docOutcome.Outcome != "applied" {
		t.Errorf("first pass doc-updater outcome = %q, want applied", docOutcome.Outcome)
	}

	model, effort, template, updatedAt1 := getAgentDefFields(t, pool, "idem", "feature", "implementor")
	if model != "sonnet-5" || effort != "medium" || template != "tier-t1-executor" {
		t.Errorf("implementor after apply = (%q, %q, %q), want (sonnet-5, medium, tier-t1-executor)", model, effort, template)
	}

	second, err := svc.ApplyForProject(confirmation)
	if err != nil {
		t.Fatalf("second ApplyForProject: %v", err)
	}
	implOutcome2 := findApplyOutcome(t, second, "feature", "implementor")
	if implOutcome2.Outcome != "unchanged" {
		t.Errorf("second pass implementor outcome = %q, want unchanged", implOutcome2.Outcome)
	}
	docOutcome2 := findApplyOutcome(t, second, "feature", "doc-updater")
	if docOutcome2.Outcome != "unchanged" {
		t.Errorf("second pass doc-updater outcome = %q, want unchanged", docOutcome2.Outcome)
	}

	_, _, _, updatedAt2 := getAgentDefFields(t, pool, "idem", "feature", "implementor")
	if updatedAt2 != updatedAt1 {
		t.Errorf("updated_at changed on unchanged apply: %q -> %q", updatedAt1, updatedAt2)
	}
}

// TestApplyForProject_SkipsCustomizedConsultantHotfixNonStatic asserts the
// four skip categories are neither written to the DB nor reported as applied.
func TestApplyForProject_SkipsCustomizedConsultantHotfixNonStatic(t *testing.T) {
	t.Parallel()
	svc, pool := setupTieringApplyTestEnv(t)
	seedProjectAndWorkflow(t, pool, "skip", "refactor", "ticket")
	seedProjectAndWorkflow(t, pool, "skip", "feature", "ticket")
	seedProjectAndWorkflow(t, pool, "skip", "hotfix", "ticket")

	seedTieringDef(t, pool, tieringDefSeed{projectID: "skip", workflowID: "refactor", defID: "implementor", model: "fable-5"})
	seedTieringDef(t, pool, tieringDefSeed{projectID: "skip", workflowID: "feature", defID: "qa-verifier", model: "opus-4-8", consultant: true})
	seedTieringDef(t, pool, tieringDefSeed{projectID: "skip", workflowID: "hotfix", defID: "implementor", model: "opus-4-8"})
	seedTieringDef(t, pool, tieringDefSeed{projectID: "skip", workflowID: "feature", defID: "implement-fanout", model: "opus-4-8", nodeRole: "planner"})

	result, err := svc.ApplyForProject(types.TieringApplyConfirmation{ProjectID: "skip", ConfirmAll: true})
	if err != nil {
		t.Fatalf("ApplyForProject: %v", err)
	}

	cases := []struct {
		workflowID, defID, wantOutcome, origModel string
	}{
		{"refactor", "implementor", "skipped-customized", "fable-5"},
		{"feature", "qa-verifier", "skipped-consultant", "opus-4-8"},
		{"hotfix", "implementor", "skipped-hotfix", "opus-4-8"},
		{"feature", "implement-fanout", "skipped-non-static", "opus-4-8"},
	}
	for _, c := range cases {
		outcome := findApplyOutcome(t, result, c.workflowID, c.defID)
		if outcome.Outcome != c.wantOutcome {
			t.Errorf("%s/%s outcome = %q, want %q", c.workflowID, c.defID, outcome.Outcome, c.wantOutcome)
		}
		model, _, _, _ := getAgentDefFields(t, pool, "skip", c.workflowID, c.defID)
		if model != c.origModel {
			t.Errorf("%s/%s model = %q, want unchanged %q", c.workflowID, c.defID, model, c.origModel)
		}
	}
	for _, o := range result.Applied {
		t.Errorf("Applied must be empty for an all-skip project, got %+v", o)
	}
}

// TestApplyForProject_UnconfirmedDefSkipped asserts a def not named in
// DefKeys (and ConfirmAll unset) is left untouched.
func TestApplyForProject_UnconfirmedDefSkipped(t *testing.T) {
	t.Parallel()
	svc, pool := setupTieringApplyTestEnv(t)
	seedProjectAndWorkflow(t, pool, "unconf", "feature", "ticket")
	seedTieringDef(t, pool, tieringDefSeed{projectID: "unconf", workflowID: "feature", defID: "implementor", model: "opus-4-8"})
	seedTieringDef(t, pool, tieringDefSeed{projectID: "unconf", workflowID: "feature", defID: "doc-updater", model: "sonnet-5"})

	result, err := svc.ApplyForProject(types.TieringApplyConfirmation{
		ProjectID: "unconf",
		DefKeys:   []types.TieringDefKey{{WorkflowID: "feature", DefID: "implementor"}},
	})
	if err != nil {
		t.Fatalf("ApplyForProject: %v", err)
	}

	implOutcome := findApplyOutcome(t, result, "feature", "implementor")
	if implOutcome.Outcome != "applied" {
		t.Errorf("confirmed implementor outcome = %q, want applied", implOutcome.Outcome)
	}
	docOutcome := findApplyOutcome(t, result, "feature", "doc-updater")
	if docOutcome.Outcome != "skipped-unconfirmed" {
		t.Errorf("unconfirmed doc-updater outcome = %q, want skipped-unconfirmed", docOutcome.Outcome)
	}
	model, _, _, _ := getAgentDefFields(t, pool, "unconf", "feature", "doc-updater")
	if model != "sonnet-5" {
		t.Errorf("unconfirmed doc-updater model = %q, want unchanged sonnet-5", model)
	}
}

// TestApplyForProject_WorkerTemplateAssignment asserts implementor/test-writer/
// doc-updater get tier-t1-executor and setup-analyzer/qa-verifier get
// tier-t2-extractor written on apply.
func TestApplyForProject_WorkerTemplateAssignment(t *testing.T) {
	t.Parallel()
	svc, pool := setupTieringApplyTestEnv(t)
	seedProjectAndWorkflow(t, pool, "tmpl", "feature", "ticket")

	roles := map[string]string{
		"setup-analyzer": "sonnet-5",
		"test-writer":    "opus-4-8",
		"implementor":    "opus-4-8",
		"qa-verifier":    "opus-4-8",
		"doc-updater":    "sonnet-5",
	}
	for defID, model := range roles {
		seedTieringDef(t, pool, tieringDefSeed{projectID: "tmpl", workflowID: "feature", defID: defID, model: model})
	}

	if _, err := svc.ApplyForProject(types.TieringApplyConfirmation{ProjectID: "tmpl", ConfirmAll: true}); err != nil {
		t.Fatalf("ApplyForProject: %v", err)
	}

	wantTemplate := map[string]string{
		"implementor":    "tier-t1-executor",
		"test-writer":    "tier-t1-executor",
		"doc-updater":    "tier-t1-executor",
		"setup-analyzer": "tier-t2-extractor",
		"qa-verifier":    "tier-t2-extractor",
	}
	for defID, want := range wantTemplate {
		_, _, template, _ := getAgentDefFields(t, pool, "tmpl", "feature", defID)
		if template != want {
			t.Errorf("%s system_template_id = %q, want %q", defID, template, want)
		}
	}
}

// TestApplyForProject_MissingProjectID validates the required field.
func TestApplyForProject_MissingProjectID(t *testing.T) {
	t.Parallel()
	svc, _ := setupTieringApplyTestEnv(t)
	if _, err := svc.ApplyForProject(types.TieringApplyConfirmation{}); err == nil {
		t.Error("ApplyForProject with empty ProjectID succeeded, want error")
	}
}
