package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupPlanValidateEnv creates an isolated DB with a project and a workflow,
// plus one usable "worker" fanout_template agent definition (model "sonnet",
// execution_mode "cli_interactive", both seeded enabled=1 in the template DB)
// so reject-case tests can reference a valid template and isolate the
// violation under test. Returns pool, projectID, workflowID.
func setupPlanValidateEnv(t *testing.T) (*db.Pool, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "plan_validate_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	const projectID = "proj1"
	const workflowID = "wf1"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		projectID, now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	wfSvc := NewWorkflowService(pool, clock.Real())
	if _, err := wfSvc.CreateWorkflowDef(projectID, &types.WorkflowDefCreateRequest{ID: workflowID}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	insertFanoutTemplate(t, pool, projectID, workflowID, "worker", "sonnet", "cli_interactive")

	return pool, projectID, workflowID
}

// insertFanoutTemplate inserts an agent_definitions row usable as a plan
// template (node_role='fanout_template', consultant=0).
func insertFanoutTemplate(t *testing.T, pool *db.Pool, projectID, workflowID, id, model, executionMode string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, layer, consultant, node_role, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 20, 'do work', ?, '', 0, 0, 'fanout_template', ?, ?)`,
		id, projectID, workflowID, model, executionMode, now, now); err != nil {
		t.Fatalf("insert fanout_template %q: %v", id, err)
	}
}

// insertPlannerDef inserts an agent_definitions row with node_role='planner'
// (never a fanout_template — used to prove a plan node cannot reference a
// planner definition by id).
func insertPlannerDef(t *testing.T, pool *db.Pool, projectID, workflowID, id string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, layer, consultant, node_role, created_at, updated_at)
		 VALUES (?, ?, ?, 'sonnet', 20, 'plan', 'cli_interactive', '', 0, 0, 'planner', ?, ?)`,
		id, projectID, workflowID, now, now); err != nil {
		t.Fatalf("insert planner def %q: %v", id, err)
	}
}

// baseValidManifest returns a minimal, fully-valid two-layer manifest bound
// to the given template id, for reject-case tests to mutate a single field.
func baseValidManifest(template string) PlanManifest {
	return PlanManifest{
		Version: 1,
		Goal:    "accomplish the goal",
		Layers: []PlanLayer{
			{
				Layer:  0,
				Policy: "all",
				Nodes: []PlanNode{
					{ID: "investigate", Template: template, Instructions: "look into it"},
				},
			},
			{
				Layer:  1,
				Policy: "any",
				Nodes: []PlanNode{
					{ID: "fix", Template: template, Instructions: "fix using #{NODE_FINDINGS:investigate}"},
				},
			},
		},
	}
}
