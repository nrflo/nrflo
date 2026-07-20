package service

import (
	"testing"
	"time"

	"be/internal/db"
	"be/internal/types"
)

// tieringDefSeed is the set of fields tiering tests need on an
// agent_definitions row; zero values pick sane defaults (node_role='static',
// consultant=false, no reasoning_effort/system_template_id override).
type tieringDefSeed struct {
	projectID, workflowID, defID, model string
	effort, nodeRole, systemTemplateID  string
	consultant                          bool
}

// seedTieringDef inserts one agent_definitions row for tiering tests. Caller
// must have already seeded the project/workflow (seedProjectAndWorkflow).
func seedTieringDef(t *testing.T, pool *db.Pool, s tieringDefSeed) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	nodeRole := s.nodeRole
	if nodeRole == "" {
		nodeRole = "static"
	}
	var effort interface{}
	if s.effort != "" {
		effort = s.effort
	}
	consultant := 0
	if s.consultant {
		consultant = 1
	}
	_, err := pool.Exec(`
		INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, layer, reasoning_effort, node_role, consultant, system_template_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, 20, '', 0, ?, ?, ?, ?, ?, ?)`,
		s.defID, s.projectID, s.workflowID, s.model, effort, nodeRole, consultant, s.systemTemplateID, now, now,
	)
	if err != nil {
		t.Fatalf("seedTieringDef(%s/%s/%s): %v", s.projectID, s.workflowID, s.defID, err)
	}
}

// seedTieringCostSession inserts a completed workflow_agent agent_session
// with a cost_estimate, keyed by (projectID, defID) — the fields
// estimateMonthlyDelta reads. Creates its own throwaway workflow_instance.
func seedTieringCostSession(t *testing.T, pool *db.Pool, projectID, workflowID, defID string, costEstimate float64, endedAt time.Time) {
	t.Helper()
	ts := endedAt.UTC().Format(time.RFC3339Nano)
	wfiID := "wfi-" + projectID + "-" + workflowID + "-" + defID
	_, err := pool.Exec(`
		INSERT OR IGNORE INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES (?, ?, '', ?, 'active', 'project', ?, ?)`,
		wfiID, projectID, workflowID, ts, ts,
	)
	if err != nil {
		t.Fatalf("seed workflow_instance for cost session: %v", err)
	}

	sessID := "sess-" + projectID + "-" + workflowID + "-" + defID + "-" + ts
	_, err = pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, status, cost_estimate, ended_at, created_at, updated_at)
		VALUES (?, ?, '', ?, ?, ?, ?, 'completed', ?, ?, ?, ?)`,
		sessID, projectID, wfiID, defID, defID, defID, costEstimate, ts, ts, ts,
	)
	if err != nil {
		t.Fatalf("seed cost session for %s/%s: %v", projectID, defID, err)
	}
}

// findProjectReport locates one project's report slice by project id.
func findProjectReport(t *testing.T, report *types.TieringReport, projectID string) types.TieringProjectReport {
	t.Helper()
	for _, p := range report.Projects {
		if p.ProjectID == projectID {
			return p
		}
	}
	t.Fatalf("no report entry for project %q", projectID)
	return types.TieringProjectReport{}
}

// findDefRow locates a report row by workflow+def id, failing the test if absent.
func findDefRow(t *testing.T, defs []types.TieringDefRow, workflowID, defID string) types.TieringDefRow {
	t.Helper()
	for _, d := range defs {
		if d.WorkflowID == workflowID && d.DefID == defID {
			return d
		}
	}
	t.Fatalf("no report row for %s/%s", workflowID, defID)
	return types.TieringDefRow{}
}

// findApplyOutcome locates a def in either Applied or Skipped by workflow+def id.
func findApplyOutcome(t *testing.T, result *types.TieringApplyResult, workflowID, defID string) types.TieringApplyOutcome {
	t.Helper()
	for _, o := range result.Applied {
		if o.WorkflowID == workflowID && o.DefID == defID {
			return o
		}
	}
	for _, o := range result.Skipped {
		if o.WorkflowID == workflowID && o.DefID == defID {
			return o
		}
	}
	t.Fatalf("no apply outcome for %s/%s", workflowID, defID)
	return types.TieringApplyOutcome{}
}

// getAgentDefFields reads back model/reasoning_effort/system_template_id/updated_at for one def row.
func getAgentDefFields(t *testing.T, pool *db.Pool, projectID, workflowID, defID string) (model, effort, template, updatedAt string) {
	t.Helper()
	var effortNS *string
	if err := pool.QueryRow(`
		SELECT model, reasoning_effort, system_template_id, updated_at
		FROM agent_definitions WHERE project_id = ? AND workflow_id = ? AND id = ?`,
		projectID, workflowID, defID,
	).Scan(&model, &effortNS, &template, &updatedAt); err != nil {
		t.Fatalf("getAgentDefFields(%s/%s/%s): %v", projectID, workflowID, defID, err)
	}
	if effortNS != nil {
		effort = *effortNS
	}
	return
}
