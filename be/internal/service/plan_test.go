package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// Fixed identifiers shared by every plan_*_test.go file in this package: one
// project + workflow + fanout_template agent definition (model 'sonnet',
// cli_interactive) that a minimal valid plan manifest can reference.
const (
	planTestProjectID  = "plan-proj"
	planTestWorkflowID = "plan-wf"
	planTestTemplateID = "plan-template"
)

// setupPlanTestEnv builds an isolated DB with the project/workflow/template
// fixtures above plus one active workflow_instance, and returns the pool and
// that instance's id. Additional instances (e.g. for the TTL sweep test) can
// be added with insertPlanTestInstance.
func setupPlanTestEnv(t *testing.T) (*db.Pool, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "plan_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Plan Test', '/tmp', ?, ?)`,
		planTestProjectID, now, now)
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'ticket', ?, ?)`,
		planTestWorkflowID, planTestProjectID, now, now)
	mustExec(t, pool, `INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, model, execution_mode, node_role, consultant, created_at, updated_at)
		 VALUES (?, ?, ?, 'do work', 0, 'sonnet', 'cli_interactive', 'fanout_template', 0, ?, ?)`,
		planTestTemplateID, planTestProjectID, planTestWorkflowID, now, now)

	instanceID := "plan-wfi-1"
	insertPlanTestInstance(t, pool, instanceID)
	return pool, instanceID
}

// insertPlanTestInstance adds another workflow_instances row under the shared
// project/workflow fixture, for tests that need more than one plan.
func insertPlanTestInstance(t *testing.T, pool *db.Pool, instanceID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		 VALUES (?, ?, '', ?, 'ticket', 'active', 0, ?, ?)`,
		instanceID, planTestProjectID, planTestWorkflowID, now, now)
}

// validPlanManifestJSON returns a minimal single-layer manifest that
// references planTestTemplateID and validates cleanly.
func validPlanManifestJSON(goal, instructions string) json.RawMessage {
	m := PlanManifest{
		Version: 1,
		Goal:    goal,
		Layers: []PlanLayer{
			{Layer: 0, Policy: "all", Nodes: []PlanNode{
				{ID: "step1", Template: planTestTemplateID, Instructions: instructions},
			}},
		},
	}
	b, _ := json.Marshal(m)
	return b
}

// validPlanManifestWithQuestions is like validPlanManifestJSON but carries a
// non-empty open Questions list (open questions must never block approval).
func validPlanManifestWithQuestions(goal string) json.RawMessage {
	m := PlanManifest{
		Version: 1,
		Goal:    goal,
		Layers: []PlanLayer{
			{Layer: 0, Policy: "all", Nodes: []PlanNode{
				{ID: "step1", Template: planTestTemplateID, Instructions: "do the thing"},
			}},
		},
		Questions: []PlanQuestion{{ID: "q1", Question: "which approach should we take?"}},
	}
	b, _ := json.Marshal(m)
	return b
}

// fakePlannerRunner is a test double for PlanService.PlannerRunner. On
// RunPlanner it records the call, writes a session row and a
// WorkflowPlanFindingKey finding for sessionID directly via FindingRepo.Upsert
// (bypassing FindingsService.Emit's schema validation so the test controls
// the raw manifest precisely), and returns sessionID.
type fakePlannerRunner struct {
	pool      *db.Pool
	clk       clock.Clock
	projectID string
	sessionID string
	manifest  json.RawMessage // nil => a default valid manifest keyed off in.Goal
	runErr    error

	calls     int
	lastInput PlannerInput
}

func newFakePlannerRunner(pool *db.Pool, clk clock.Clock, projectID, sessionID string) *fakePlannerRunner {
	return &fakePlannerRunner{pool: pool, clk: clk, projectID: projectID, sessionID: sessionID}
}

func (f *fakePlannerRunner) RunPlanner(ctx context.Context, instanceID string, in PlannerInput) (string, error) {
	f.calls++
	f.lastInput = in
	if f.runErr != nil {
		return "", f.runErr
	}

	now := f.clk.Now().UTC().Format(time.RFC3339Nano)
	if _, err := f.pool.Exec(
		`INSERT OR IGNORE INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, created_at, updated_at)
		 VALUES (?, ?, '', ?, 'plan', 'planner', 'completed', ?, ?)`,
		f.sessionID, f.projectID, instanceID, now, now,
	); err != nil {
		return "", err
	}

	manifest := f.manifest
	if manifest == nil {
		manifest = validPlanManifestJSON(in.Goal, "do the thing")
	}
	fr := repo.NewFindingRepo(f.pool, f.clk)
	if err := fr.Upsert("session", f.sessionID, WorkflowPlanFindingKey, manifest,
		repo.Denorm{ProjectID: f.projectID, WorkflowInstanceID: instanceID},
		repo.Actor{ID: f.sessionID, Source: "agent"}); err != nil {
		return "", err
	}
	return f.sessionID, nil
}
