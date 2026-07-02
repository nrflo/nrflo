package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

// seedDeepResearchReport inserts a terminal synthesize session plus its session-scoped
// "report" finding, mirroring what emit_findings stores during a real run.
func seedDeepResearchReport(t *testing.T, env *testEnv, wfiID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, prompt, config, ended_at, created_at, updated_at)
		VALUES ('dr-synth', ?, '', ?, 'synthesize', 'synthesize', 'project_completed', '', '', ?, ?, ?)`,
		env.project, wfiID, now, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := repo.NewFindingRepo(env.pool, clock.Real()).Upsert(
		"session", "dr-synth", "report", json.RawMessage(`{"summary":"the answer","findings":[],"caveats":""}`),
		repo.Denorm{ProjectID: env.project, WorkflowInstanceID: wfiID},
		repo.Actor{ID: "dr-synth", Source: "agent"}); err != nil {
		t.Fatalf("seed report finding: %v", err)
	}
}

// TestReadDeepResearchReport_ReadsSessionScopedReport is the regression for the
// scope misread: emit_findings stores the report at session scope, and the runner
// must read it back from there (not workflow_instance scope) on a terminal instance.
func TestReadDeepResearchReport_ReadsSessionScopedReport(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")
	if _, err := env.pool.Exec(`UPDATE workflow_instances SET status='project_completed' WHERE id=?`, wfiID); err != nil {
		t.Fatalf("mark instance terminal: %v", err)
	}
	seedDeepResearchReport(t, env, wfiID)

	report, err := env.orch.readDeepResearchReport(wfiID)
	if err != nil {
		t.Fatalf("readDeepResearchReport: %v", err)
	}
	if !strings.Contains(string(report), "the answer") {
		t.Errorf("report = %s, want the seeded summary", report)
	}
}

// TestReadDeepResearchReport_NoReportErrors verifies a terminal run without a
// report finding surfaces a clear error instead of an empty result.
func TestReadDeepResearchReport_NoReportErrors(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")
	if _, err := env.pool.Exec(`UPDATE workflow_instances SET status='project_completed' WHERE id=?`, wfiID); err != nil {
		t.Fatalf("mark instance terminal: %v", err)
	}

	_, err := env.orch.readDeepResearchReport(wfiID)
	if err == nil || !strings.Contains(err.Error(), "no 'report' finding") {
		t.Errorf("want no-report error, got %v", err)
	}
}

// TestReadDeepResearchReport_NonTerminalErrors verifies a non-terminal instance
// (active, waiting, failed) is rejected rather than read.
func TestReadDeepResearchReport_NonTerminalErrors(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test") // stays active

	_, err := env.orch.readDeepResearchReport(wfiID)
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Errorf("want non-terminal status error, got %v", err)
	}
}
