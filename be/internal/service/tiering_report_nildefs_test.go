package service

import "testing"

// Repro: a project whose defs all fail ClassifyRole (or has no defs) leaves
// TieringProjectReport.Defs nil -> JSON "defs":null -> FE project.defs.filter crashes.
func TestBuildReport_ProjectWithNoMappedDefs_DefsNonNil(t *testing.T) {
	t.Parallel()
	svc, pool, _ := setupTieringReportTestEnv(t)
	seedProjectAndWorkflow(t, pool, "empty-proj", "feature", "ticket")

	report, err := svc.BuildReport()
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	proj := findProjectReport(t, report, "empty-proj")
	if proj.Defs == nil {
		t.Fatal("Defs is nil — marshals to JSON null and crashes the FE .filter()")
	}
}
