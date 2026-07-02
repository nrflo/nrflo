package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

// seedChildInstance marks the instance as a sub-run at the given depth/status
// and seeds a terminal session plus a session-scoped result finding under key,
// mirroring what emit_findings stores during a real run.
func seedChildInstance(t *testing.T, env *testEnv, wfiID, status string, depth int, key string) {
	t.Helper()
	if _, err := env.pool.Exec(
		`UPDATE workflow_instances SET status=?, launch_depth=? WHERE id=?`, status, depth, wfiID); err != nil {
		t.Fatalf("mark instance: %v", err)
	}
	if key == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, prompt, config, ended_at, created_at, updated_at)
		VALUES ('sw-synth', ?, '', ?, 'synthesize', 'synthesize', 'project_completed', '', '', ?, ?, ?)`,
		env.project, wfiID, now, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := repo.NewFindingRepo(env.pool, clock.Real()).Upsert(
		"session", "sw-synth", key, json.RawMessage(`{"summary":"the answer"}`),
		repo.Denorm{ProjectID: env.project, WorkflowInstanceID: wfiID},
		repo.Actor{ID: "sw-synth", Source: "agent"}); err != nil {
		t.Fatalf("seed result finding: %v", err)
	}
}

// GetSubworkflow must read the result from session scope (the scope
// emit_findings actually writes) — regression heir of the deep-research
// workflow_instance-scope misread.
func TestGetSubworkflow_ReadsSessionScopedResult(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, wfiID, "project_completed", 1, "report")

	status, result, _, err := env.orch.GetSubworkflow(context.Background(), env.project, wfiID, "report")
	if err != nil {
		t.Fatalf("GetSubworkflow: %v", err)
	}
	if status != "completed" || !strings.Contains(string(result), "the answer") {
		t.Errorf("got (%q, %s), want completed with seeded result", status, result)
	}
}

func TestGetSubworkflow_StatusMapping(t *testing.T) {
	env := newTestEnv(t)

	active := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, active, "active", 1, "")
	if status, _, _, err := env.orch.GetSubworkflow(context.Background(), env.project, active, ""); err != nil || status != "running" {
		t.Errorf("active: got (%q, %v), want running", status, err)
	}

	failed := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, failed, "failed", 1, "")
	fr := repo.NewFindingRepo(env.pool, clock.Real())
	_ = fr.Upsert("workflow_instance", failed, "_failure_reason", json.RawMessage(`{"reason":"boom"}`),
		repo.Denorm{ProjectID: env.project, WorkflowInstanceID: failed}, repo.Actor{Source: "orchestrator"})
	status, _, reason, err := env.orch.GetSubworkflow(context.Background(), env.project, failed, "")
	if err != nil || status != "failed" || reason != "boom" {
		t.Errorf("failed: got (%q, %q, %v), want (failed, boom)", status, reason, err)
	}
}

// Top-level runs (launch_depth=0) and foreign projects are not pollable.
func TestGetSubworkflow_RejectsNonSubRuns(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test") // launch_depth stays 0

	if _, _, _, err := env.orch.GetSubworkflow(context.Background(), env.project, wfiID, ""); err == nil {
		t.Error("want error for launch_depth=0 instance")
	}

	seedChildInstance(t, env, wfiID, "active", 1, "")
	if _, _, _, err := env.orch.GetSubworkflow(context.Background(), "other-project", wfiID, ""); err == nil {
		t.Error("want error for foreign project")
	}
}

// StartSubworkflow guard order: non-callable defs are rejected before any
// budget/concurrency reservation; an inactive parent is rejected after them.
func TestStartSubworkflow_Guards(t *testing.T) {
	env := newTestEnv(t)
	parentID := env.initProjectWorkflow(t, "test")

	// Not callable.
	_, err := env.orch.StartSubworkflow(context.Background(), parentID, env.project, "test", "go")
	if err == nil || !strings.Contains(err.Error(), "not callable_as_subworkflow") {
		t.Fatalf("want not-callable error, got %v", err)
	}

	// Callable but purging.
	if _, err := env.pool.Exec(
		`UPDATE workflows SET callable_as_subworkflow=1, purge_on_completion=1 WHERE LOWER(project_id)=LOWER(?) AND LOWER(id)='test'`,
		env.project); err != nil {
		t.Fatal(err)
	}
	_, err = env.orch.StartSubworkflow(context.Background(), parentID, env.project, "test", "go")
	if err == nil || !strings.Contains(err.Error(), "purge_on_completion") {
		t.Fatalf("want purge error, got %v", err)
	}

	// Callable, non-purging, but depth cap exceeded.
	if _, err := env.pool.Exec(
		`UPDATE workflows SET purge_on_completion=0 WHERE LOWER(project_id)=LOWER(?) AND LOWER(id)='test'`, env.project); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(
		`UPDATE workflow_instances SET launch_depth=? WHERE id=?`, 3, parentID); err != nil {
		t.Fatal(err)
	}
	_, err = env.orch.StartSubworkflow(context.Background(), parentID, env.project, "test", "go")
	if err == nil || !strings.Contains(err.Error(), "subworkflow_max_depth") {
		t.Fatalf("want depth-cap error, got %v", err)
	}

	// Depth ok but parent run not active (no o.runs entry).
	if _, err := env.pool.Exec(
		`UPDATE workflow_instances SET launch_depth=0 WHERE id=?`, parentID); err != nil {
		t.Fatal(err)
	}
	_, err = env.orch.StartSubworkflow(context.Background(), parentID, env.project, "test", "go")
	if err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("want parent-not-active error, got %v", err)
	}
}
