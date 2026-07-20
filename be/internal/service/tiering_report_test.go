package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

func setupTieringReportTestEnv(t *testing.T) (*TieringService, *db.Pool, time.Time) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tiering_report_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(now)
	svc := NewTieringService(pool, clk, NewModelService(pool, clk))
	return svc, pool, now
}

// TestBuildReport_ProjectAlpha covers: stock implementor (opus-4-8 -> sonnet-5,
// delta<0), doc-updater stock with no usage (na fallback), hotfix implementor
// (skip=hotfix, no delta), and an unmapped def excluded entirely.
func TestBuildReport_ProjectAlpha(t *testing.T) {
	t.Parallel()
	svc, pool, now := setupTieringReportTestEnv(t)

	seedProjectAndWorkflow(t, pool, "alpha", "feature", "ticket")
	seedProjectAndWorkflow(t, pool, "alpha", "hotfix", "ticket")
	seedProjectAndWorkflow(t, pool, "alpha", "__spec_import__", "project")

	seedTieringDef(t, pool, tieringDefSeed{projectID: "alpha", workflowID: "feature", defID: "implementor", model: "opus-4-8"})
	seedTieringCostSession(t, pool, "alpha", "feature", "implementor", 100, now.AddDate(0, 0, -5))

	seedTieringDef(t, pool, tieringDefSeed{projectID: "alpha", workflowID: "feature", defID: "doc-updater", model: "sonnet-5"})

	seedTieringDef(t, pool, tieringDefSeed{projectID: "alpha", workflowID: "hotfix", defID: "implementor", model: "opus-4-8"})

	seedTieringDef(t, pool, tieringDefSeed{projectID: "alpha", workflowID: "__spec_import__", defID: "spec-normalizer", model: "haiku-4-5"})

	report, err := svc.BuildReport()
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	proj := findProjectReport(t, report, "alpha")

	implementor := findDefRow(t, proj.Defs, "feature", "implementor")
	if implementor.RecommendedModel != "sonnet-5" || implementor.Customized || implementor.SkipReason != "" {
		t.Errorf("stock implementor row = %+v", implementor)
	}
	if implementor.EstMonthlyDelta == nil || *implementor.EstMonthlyDelta >= 0 {
		t.Errorf("stock implementor EstMonthlyDelta = %v, want non-nil and negative", implementor.EstMonthlyDelta)
	}

	docUpdater := findDefRow(t, proj.Defs, "feature", "doc-updater")
	if docUpdater.RecommendedModel != "haiku-4-5" || docUpdater.Customized || docUpdater.SkipReason != "" {
		t.Errorf("doc-updater row = %+v", docUpdater)
	}
	if docUpdater.EstMonthlyDelta != nil {
		t.Errorf("doc-updater EstMonthlyDelta = %v, want nil (no usage -> na)", *docUpdater.EstMonthlyDelta)
	}

	hotfixImpl := findDefRow(t, proj.Defs, "hotfix", "implementor")
	if hotfixImpl.SkipReason != "hotfix" {
		t.Errorf("hotfix implementor SkipReason = %q, want hotfix", hotfixImpl.SkipReason)
	}
	if hotfixImpl.EstMonthlyDelta != nil {
		t.Errorf("hotfix implementor EstMonthlyDelta = %v, want nil (skip reasons other than customized never compute delta)", *hotfixImpl.EstMonthlyDelta)
	}

	for _, d := range proj.Defs {
		if d.DefID == "spec-normalizer" {
			t.Errorf("unmapped def spec-normalizer must be excluded from report, got row %+v", d)
		}
	}
}

// TestBuildReport_ProjectBeta covers: customized model (skip=customized, but
// delta still computed), consultant=true (skip=consultant, no delta), and a
// non-static node_role def (skip=non-static, no delta).
func TestBuildReport_ProjectBeta(t *testing.T) {
	t.Parallel()
	svc, pool, now := setupTieringReportTestEnv(t)

	seedProjectAndWorkflow(t, pool, "beta", "refactor", "ticket")
	seedProjectAndWorkflow(t, pool, "beta", "feature", "ticket")

	seedTieringDef(t, pool, tieringDefSeed{projectID: "beta", workflowID: "refactor", defID: "implementor", model: "fable-5"})
	seedTieringCostSession(t, pool, "beta", "refactor", "implementor", 50, now.AddDate(0, 0, -3))

	seedTieringDef(t, pool, tieringDefSeed{projectID: "beta", workflowID: "feature", defID: "qa-verifier", model: "opus-4-8", consultant: true})
	seedTieringCostSession(t, pool, "beta", "feature", "qa-verifier", 40, now.AddDate(0, 0, -3))

	seedTieringDef(t, pool, tieringDefSeed{projectID: "beta", workflowID: "feature", defID: "implement-fanout", model: "opus-4-8", nodeRole: "planner"})

	report, err := svc.BuildReport()
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	proj := findProjectReport(t, report, "beta")

	customized := findDefRow(t, proj.Defs, "refactor", "implementor")
	if !customized.Customized || customized.SkipReason != "customized" {
		t.Errorf("customized implementor row = %+v", customized)
	}
	if customized.EstMonthlyDelta == nil {
		t.Error("customized implementor EstMonthlyDelta = nil, want computed (report shows would-be savings)")
	}

	consultant := findDefRow(t, proj.Defs, "feature", "qa-verifier")
	if consultant.SkipReason != "consultant" {
		t.Errorf("consultant def SkipReason = %q, want consultant", consultant.SkipReason)
	}
	if consultant.EstMonthlyDelta != nil {
		t.Errorf("consultant def EstMonthlyDelta = %v, want nil", *consultant.EstMonthlyDelta)
	}

	nonStatic := findDefRow(t, proj.Defs, "feature", "implement-fanout")
	if nonStatic.SkipReason != "non-static" {
		t.Errorf("non-static def SkipReason = %q, want non-static", nonStatic.SkipReason)
	}
	if nonStatic.EstMonthlyDelta != nil {
		t.Errorf("non-static def EstMonthlyDelta = %v, want nil", *nonStatic.EstMonthlyDelta)
	}
}

// TestBuildReport_WorkerTemplateAssignment asserts implementor/test-writer/
// doc-updater map to tier-t1-executor and setup-analyzer/qa-verifier map to
// tier-t2-extractor in the report's recommended template.
func TestBuildReport_WorkerTemplateAssignment(t *testing.T) {
	t.Parallel()
	svc, pool, _ := setupTieringReportTestEnv(t)
	seedProjectAndWorkflow(t, pool, "gamma", "feature", "ticket")

	roles := map[string]string{
		"setup-analyzer": "sonnet-5",
		"test-writer":    "opus-4-8",
		"implementor":    "opus-4-8",
		"qa-verifier":    "opus-4-8",
		"doc-updater":    "sonnet-5",
	}
	for defID, model := range roles {
		seedTieringDef(t, pool, tieringDefSeed{projectID: "gamma", workflowID: "feature", defID: defID, model: model})
	}

	report, err := svc.BuildReport()
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	proj := findProjectReport(t, report, "gamma")

	wantTemplate := map[string]string{
		"implementor":    "tier-t1-executor",
		"test-writer":    "tier-t1-executor",
		"doc-updater":    "tier-t1-executor",
		"setup-analyzer": "tier-t2-extractor",
		"qa-verifier":    "tier-t2-extractor",
	}
	for defID, want := range wantTemplate {
		row := findDefRow(t, proj.Defs, "feature", defID)
		if row.RecommendedTemplate != want {
			t.Errorf("%s RecommendedTemplate = %q, want %q", defID, row.RecommendedTemplate, want)
		}
	}

	if !strings.Contains(report.Markdown, "# Tiering Report") || !strings.Contains(report.Markdown, "gamma") {
		t.Errorf("Markdown missing expected content: %s", report.Markdown)
	}
}
